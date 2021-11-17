// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sds

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discovery "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	istiolog "istio.io/pkg/log"

	"istio.io/istio/pilot/pkg/features"
	istiogrpc "istio.io/istio/pilot/pkg/grpc"
	"istio.io/istio/pilot/pkg/model"
	v3 "istio.io/istio/pilot/pkg/xds/v3"
	"istio.io/istio/pkg/spiffe"
)

var (
	log = istiolog.RegisterScope("generic-ads", "ads debugging", 0)

	// Tracks connections, increment on each new connection.
	connectionNumber = int64(0)
)

// GenericXDSServer contains common for a xDS server.
type GenericXdsServer struct {
	// mutex used for protecting Environment.PushContext
	updateMutex sync.RWMutex
	// Env is the model environment.
	Env *model.Environment

	// Generators allow customizing the generated config, based on the client metadata.
	// Key is the generator type - will match the Generator metadata to set the per-connection
	// default generator, or the combination of Generator metadata and TypeUrl to select a
	// different generator for a type.
	// Normal istio clients use the default generator - will not be impacted by this.
	Generators map[string]model.XdsResourceGenerator

	// ProxyNeedsPush is a function that determines whether a push can be completely skipped. Individual generators
	// may also choose to not send any updates.
	ProxyNeedsPush func(proxy *model.Proxy, req *model.PushRequest) bool

	pushChannel chan *model.PushRequest

	// adsClients reflect active gRPC channels, for both ADS and EDS.
	adsClients      map[string]*Connection
	adsClientsMutex sync.RWMutex
}

// NewDiscoveryServer creates DiscoveryServer that sources data from Pilot's internal mesh data structures
func NewGenericXdsServer(env *model.Environment) *GenericXdsServer {
	out := &GenericXdsServer{
		Env:         env,
		Generators:  map[string]model.XdsResourceGenerator{},
		pushChannel: make(chan *model.PushRequest, 10),
		adsClients:  map[string]*Connection{},
	}

	return out
}

func (s *GenericXdsServer) Run(stopCh <-chan struct{}) {
	// versionNum counts versions
	var versionNum uint64 = 0
	for {
		select {
		case pushRequest := <-s.pushChannel:
			versionNum++
			versionLocal := time.Now().Format(time.RFC3339) + "/" + strconv.FormatUint(versionNum, 10)
			pushRequest.Push = &model.PushContext{PushVersion: versionLocal}
			s.sdsPushAll(versionLocal, pushRequest)
		case <-stopCh:
			return
		}
	}
}

// DiscoveryStream is a server interface for XDS.
type DiscoveryStream = discovery.AggregatedDiscoveryService_StreamAggregatedResourcesServer

// DeltaDiscoveryStream is a server interface for Delta XDS.
type DeltaDiscoveryStream = discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesServer

// DiscoveryClient is a client interface for XDS.
type DiscoveryClient = discovery.AggregatedDiscoveryService_StreamAggregatedResourcesClient

// DeltaDiscoveryClient is a client interface for Delta XDS.
type DeltaDiscoveryClient = discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesClient

// Connection holds information about connected client.
type Connection struct {
	// PeerAddr is the address of the client, from network layer.
	PeerAddr string

	// Defines associated identities for the connection
	Identities []string

	// ConID is the connection identifier, used as a key in the connection table.
	// Currently based on the node name and a counter.
	ConID string

	// proxy is the client to which this connection is established.
	proxy *model.Proxy

	// Sending on this channel results in a push.
	pushChannel chan *model.PushRequest

	// Both ADS and SDS streams implement this interface
	stream DiscoveryStream

	// initialized channel will be closed when proxy is initialized. Pushes, or anything accessing
	// the proxy, should not be started until this channel is closed.
	initialized chan struct{}

	// stop can be used to end the connection manually via debug endpoints. Only to be used for testing.
	stop chan struct{}

	// reqChan is used to receive discovery requests for this connection.
	reqChan chan *discovery.DiscoveryRequest

	// errorChan is used to process error during discovery request processing.
	errorChan chan error
}

func newConnection(peerAddr string, stream DiscoveryStream) *Connection {
	return &Connection{
		pushChannel: make(chan *model.PushRequest),
		initialized: make(chan struct{}),
		stop:        make(chan struct{}),
		reqChan:     make(chan *discovery.DiscoveryRequest, 1),
		errorChan:   make(chan error, 1),
		PeerAddr:    peerAddr,
		stream:      stream,
	}
}

func (s *GenericXdsServer) receive(con *Connection) {
	defer func() {
		close(con.errorChan)
		close(con.reqChan)
		// Close the initialized channel, if its not already closed, to prevent blocking the stream.
		select {
		case <-con.initialized:
		default:
			close(con.initialized)
		}
	}()

	firstRequest := true
	for {
		req, err := con.stream.Recv()
		if err != nil {
			if istiogrpc.IsExpectedGRPCError(err) {
				log.Infof("ADS: %q %s terminated %v", con.PeerAddr, con.ConID, err)
				return
			}
			con.errorChan <- err
			log.Errorf("ADS: %q %s terminated with error: %v", con.PeerAddr, con.ConID, err)
			return
		}
		// This should be only set for the first request. The node id may not be set - for example malicious clients.
		if firstRequest {
			// probe happens before envoy sends first xDS request
			if req.TypeUrl == v3.HealthInfoType {
				log.Warnf("ADS: %q %s send health check probe before normal xDS request", con.PeerAddr, con.ConID)
				continue
			}
			firstRequest = false
			if req.Node == nil || req.Node.Id == "" {
				con.errorChan <- status.New(codes.InvalidArgument, "missing node information").Err()
				return
			}
			if err := s.initConnection(req.Node, con); err != nil {
				con.errorChan <- err
				return
			}
			defer s.closeConnection(con)
			log.Infof("ADS: new connection for node:%s", con.ConID)
		}

		select {
		case con.reqChan <- req:
		case <-con.stream.Context().Done():
			log.Infof("ADS: %q %s terminated with stream closed", con.PeerAddr, con.ConID)
			return
		}
	}
}

// processRequest is handling one request. This is currently called from the 'main' thread, which also
// handles 'push' requests and close - the code will eventually call the 'push' code, and it needs more mutex
// protection. Original code avoided the mutexes by doing both 'push' and 'process requests' in same thread.
func (s *GenericXdsServer) processRequest(req *discovery.DiscoveryRequest, con *Connection) error {
	shouldRespond := s.shouldRespond(con, req)

	var request *model.PushRequest
	push := s.globalPushContext()
	if shouldRespond {
		// This is a request, trigger a full push for this type. Override the blocked push (if it exists),
		// as this full push is guaranteed to be a superset of what we would have pushed from the blocked push.
		request = &model.PushRequest{Full: true, Push: push}
	} else {
		return nil
	}

	request.Reason = append(request.Reason, model.ProxyRequest)
	request.Start = time.Now()
	return s.pushXds(con, push, con.Watched(req.TypeUrl), request)
}

// StreamAggregatedResources implements the ADS interface.
func (s *GenericXdsServer) StreamAggregatedResources(stream discovery.AggregatedDiscoveryService_StreamAggregatedResourcesServer) error {
	return s.Stream(stream)
}

// StreamAggregatedResources implements the ADS interface.
func (s *GenericXdsServer) DeltaAggregatedResources(stream discovery.AggregatedDiscoveryService_DeltaAggregatedResourcesServer) error {
	return fmt.Errorf("delta ads is not supported")
}

func (s *GenericXdsServer) Stream(stream DiscoveryStream) error {
	// Check if server is ready to accept clients and process new requests.
	// Currently ready means caches have been synced and hence can build
	// clusters correctly. Without this check, InitContext() call below would
	// initialize with empty config, leading to reconnected Envoys loosing
	// configuration. This is an additional safety check inaddition to adding
	// cachesSynced logic to readiness probe to handle cases where kube-proxy
	// ip tables update latencies.
	// See https://github.com/istio/istio/issues/25495.
	// if !s.IsServerReady() {
	// 	return status.Error(codes.Unavailable, "server is not ready to serve discovery information")
	// }

	ctx := stream.Context()
	peerAddr := "0.0.0.0"
	if peerInfo, ok := peer.FromContext(ctx); ok {
		peerAddr = peerInfo.Addr.String()
	}

	// ids, err := s.authenticate(ctx)
	// if err != nil {
	// 	return status.Error(codes.Unauthenticated, err.Error())
	// }
	// if ids != nil {
	// 	log.Debugf("Authenticated XDS: %v with identity %v", peerAddr, ids)
	// } else {
	// 	log.Debugf("Unauthenticated XDS: %s", peerAddr)
	// }

	con := newConnection(peerAddr, stream)
	// Do not call: defer close(con.pushChannel). The push channel will be garbage collected
	// when the connection is no longer used. Closing the channel can cause subtle race conditions
	// with push. According to the spec: "It's only necessary to close a channel when it is important
	// to tell the receiving goroutines that all data have been sent."

	// Block until either a request is received or a push is triggered.
	// We need 2 go routines because 'read' blocks in Recv().
	go s.receive(con)

	// Wait for the proxy to be fully initialized before we start serving traffic. Because
	// initialization doesn't have dependencies that will block, there is no need to add any timeout
	// here. Prior to this explicit wait, we were implicitly waiting by receive() not sending to
	// reqChannel and the connection not being enqueued for pushes to pushChannel until the
	// initialization is complete.
	<-con.initialized

	for {
		select {
		case req, ok := <-con.reqChan:
			if ok {
				if err := s.processRequest(req, con); err != nil {
					return err
				}
			} else {
				// Remote side closed connection or error processing the request.
				return <-con.errorChan
			}
		case pushReq := <-con.pushChannel:
			err := s.pushConnection(con, pushReq)
			if err != nil {
				return err
			}
		case <-con.stop:
			return nil
		}
	}
}

// shouldRespond determines whether this request needs to be responded back. It applies the ack/nack rules as per xds protocol
// using WatchedResource for previous state and discovery request for the current state.
func (s *GenericXdsServer) shouldRespond(con *Connection, request *discovery.DiscoveryRequest) bool {
	stype := v3.GetShortType(request.TypeUrl)

	// If there is an error in request that means previous response is erroneous.
	// We do not have to respond in that case. In this case request's version info
	// will be different from the version sent. But it is fragile to rely on that.
	if request.ErrorDetail != nil {
		errCode := codes.Code(request.ErrorDetail.Code)
		log.Warnf("ADS:%s: ACK ERROR %s %s:%s", stype, con.ConID, errCode.String(), request.ErrorDetail.GetMessage())
		con.proxy.Lock()
		if w, f := con.proxy.WatchedResources[request.TypeUrl]; f {
			w.NonceNacked = request.ResponseNonce
		}
		con.proxy.Unlock()
		return false
	}

	if shouldUnsubscribe(request) {
		log.Debugf("ADS:%s: UNSUBSCRIBE %s %s %s", stype, con.ConID, request.VersionInfo, request.ResponseNonce)
		con.proxy.Lock()
		delete(con.proxy.WatchedResources, request.TypeUrl)
		con.proxy.Unlock()
		return false
	}

	// This is first request - initialize typeUrl watches.
	if request.ResponseNonce == "" {
		log.Debugf("ADS:%s: INIT %s %s %s", stype, con.ConID, request.VersionInfo, request.ResponseNonce)
		con.proxy.Lock()
		con.proxy.WatchedResources[request.TypeUrl] = &model.WatchedResource{TypeUrl: request.TypeUrl, ResourceNames: request.ResourceNames, LastRequest: request}
		con.proxy.Unlock()
		return true
	}

	con.proxy.RLock()
	previousInfo := con.proxy.WatchedResources[request.TypeUrl]
	con.proxy.RUnlock()

	// This is a case of Envoy reconnecting Istiod i.e. Istiod does not have
	// information about this typeUrl, but Envoy sends response nonce - either
	// because Istiod is restarted or Envoy disconnects and reconnects.
	// We should always respond with the current resource names.
	if previousInfo == nil {
		log.Debugf("ADS:%s: RECONNECT %s %s %s", stype, con.ConID, request.VersionInfo, request.ResponseNonce)
		con.proxy.Lock()
		con.proxy.WatchedResources[request.TypeUrl] = &model.WatchedResource{TypeUrl: request.TypeUrl, ResourceNames: request.ResourceNames, LastRequest: request}
		con.proxy.Unlock()
		return true
	}

	// If there is mismatch in the nonce, that is a case of expired/stale nonce.
	// A nonce becomes stale following a newer nonce being sent to Envoy.
	if request.ResponseNonce != previousInfo.NonceSent {
		log.Debugf("ADS:%s: REQ %s Expired nonce received %s, sent %s", stype,
			con.ConID, request.ResponseNonce, previousInfo.NonceSent)
		con.proxy.Lock()
		con.proxy.WatchedResources[request.TypeUrl].NonceNacked = ""
		con.proxy.WatchedResources[request.TypeUrl].LastRequest = request
		con.proxy.Unlock()
		return false
	}

	// If it comes here, that means nonce match. This an ACK. We should record
	// the ack details and respond if there is a change in resource names.
	con.proxy.Lock()
	previousResources := con.proxy.WatchedResources[request.TypeUrl].ResourceNames
	con.proxy.WatchedResources[request.TypeUrl].VersionAcked = request.VersionInfo
	con.proxy.WatchedResources[request.TypeUrl].NonceAcked = request.ResponseNonce
	con.proxy.WatchedResources[request.TypeUrl].NonceNacked = ""
	con.proxy.WatchedResources[request.TypeUrl].ResourceNames = request.ResourceNames
	con.proxy.WatchedResources[request.TypeUrl].LastRequest = request
	con.proxy.Unlock()

	// Envoy can send two DiscoveryRequests with same version and nonce
	// when it detects a new resource. We should respond if they change.
	if listEqualUnordered(previousResources, request.ResourceNames) {
		log.Debugf("ADS:%s: ACK %s %s %s", stype, con.ConID, request.VersionInfo, request.ResponseNonce)
		return false
	}
	log.Debugf("ADS:%s: RESOURCE CHANGE previous resources: %v, new resources: %v %s %s %s", stype,
		previousResources, request.ResourceNames, con.ConID, request.VersionInfo, request.ResponseNonce)

	return true
}

// shouldUnsubscribe checks if we should unsubscribe. This is done when Envoy is
// no longer watching. For example, we remove all RDS references, we will
// unsubscribe from RDS. NOTE: This may happen as part of the initial request. If
// there are no routes needed, Envoy will send an empty request, which this
// properly handles by not adding it to the watched resource list.
func shouldUnsubscribe(request *discovery.DiscoveryRequest) bool {
	return len(request.ResourceNames) == 0 && !isWildcardTypeURL(request.TypeUrl)
}

// isWildcardTypeURL checks whether a given type is a wildcard type
// https://www.envoyproxy.io/docs/envoy/latest/api-docs/xds_protocol#how-the-client-specifies-what-resources-to-return
// If the list of resource names becomes empty, that means that the client is no
// longer interested in any resources of the specified type. For Listener and
// Cluster resource types, there is also a “wildcard” mode, which is triggered
// when the initial request on the stream for that resource type contains no
// resource names.
func isWildcardTypeURL(typeURL string) bool {
	switch typeURL {
	case v3.SecretType:
		return false
	default:
		// All of our internal types use wildcard semantics
		return true
	}
}

// listEqualUnordered checks that two lists contain all the same elements
func listEqualUnordered(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	first := make(map[string]struct{}, len(a))
	for _, c := range a {
		first[c] = struct{}{}
	}
	for _, c := range b {
		_, f := first[c]
		if !f {
			return false
		}
	}
	return true
}

// update the node associated with the connection, after receiving a packet from envoy, also adds the connection
// to the tracking map.
func (s *GenericXdsServer) initConnection(node *core.Node, con *Connection) error {
	// Setup the initial proxy metadata
	proxy, err := s.initProxyMetadata(node)
	if err != nil {
		return err
	}
	// First request so initialize connection id and start tracking it.
	con.ConID = connectionID(proxy.ID)
	con.proxy = proxy

	// Register the connection. this allows pushes to be triggered for the proxy. Note: the timing of
	// this and initializeProxy important. While registering for pushes *after* initialization is complete seems like
	// a better choice, it introduces a race condition; If we complete initialization of a new push
	// context between initializeProxy and addCon, we would not get any pushes triggered for the new
	// push context, leading the proxy to have a stale state until the next full push.
	s.addCon(con.ConID, con)
	// Register that initialization is complete. This triggers to calls that it is safe to access the
	// proxy
	defer close(con.initialized)

	// Complete full initialization of the proxy
	if err := s.initializeProxy(con); err != nil {
		s.closeConnection(con)
		return err
	}

	return nil
}

func (s *GenericXdsServer) closeConnection(con *Connection) {
	if con.ConID == "" {
		return
	}
	s.removeCon(con.ConID)
}

func checkConnectionIdentity(con *Connection) (*spiffe.Identity, error) {
	for _, rawID := range con.Identities {
		spiffeID, err := spiffe.ParseIdentity(rawID)
		if err != nil {
			continue
		}
		if con.proxy.ConfigNamespace != "" && spiffeID.Namespace != con.proxy.ConfigNamespace {
			continue
		}
		if con.proxy.Metadata.ServiceAccount != "" && spiffeID.ServiceAccount != con.proxy.Metadata.ServiceAccount {
			continue
		}
		return &spiffeID, nil
	}
	return nil, fmt.Errorf("no identities (%v) matched %v/%v", con.Identities, con.proxy.ConfigNamespace, con.proxy.Metadata.ServiceAccount)
}

func connectionID(node string) string {
	id := atomic.AddInt64(&connectionNumber, 1)
	return node + "-" + strconv.FormatInt(id, 10)
}

// initProxyMetadata initializes just the basic metadata of a proxy. This is decoupled from
// initProxyState such that we can perform authorization before attempting expensive computations to
// fully initialize the proxy.
func (s *GenericXdsServer) initProxyMetadata(node *core.Node) (*model.Proxy, error) {
	meta, err := model.ParseMetadata(node.Metadata)
	if err != nil {
		return nil, err
	}
	proxy, err := model.ParseServiceNodeWithMetadata(node.Id, meta)
	if err != nil {
		return nil, err
	}
	// Update the config namespace associated with this proxy
	proxy.ConfigNamespace = model.GetProxyConfigNamespace(proxy)
	proxy.XdsNode = node
	return proxy, nil
}

// initializeProxy completes the initialization of a proxy. It is expected to be called only after
// initProxyMetadata.
func (s *GenericXdsServer) initializeProxy(con *Connection) error {
	proxy := con.proxy
	proxy.WatchedResources = map[string]*model.WatchedResource{}
	// Based on node metadata and version, we can associate a different generator.
	if proxy.Metadata.Generator != "" {
		proxy.XdsResourceGenerator = s.Generators[proxy.Metadata.Generator]
	}

	return nil
}

// Compute and send the new configuration for a connection.
func (s *GenericXdsServer) pushConnection(con *Connection, pushRequest *model.PushRequest) error {
	if !s.ProxyNeedsPush(con.proxy, pushRequest) {
		log.Debugf("Skipping push to %v, no updates required", con.ConID)
		return nil
	}

	// Send pushes to all generators
	// Each Generator is responsible for determining if the push event requires a push
	for _, w := range orderWatchedResources(con.proxy.WatchedResources) {
		// Always send the push if flow control disabled
		if err := s.pushXds(con, pushRequest.Push, w, pushRequest); err != nil {
			return err
		}
		continue
	}
	return nil
}

// PushOrder defines the order that updates will be pushed in. Any types not listed here will be pushed in random
// order after the types listed here
var PushOrder = []string{v3.SecretType}

// KnownOrderedTypeUrls has typeUrls for which we know the order of push.
var KnownOrderedTypeUrls = map[string]struct{}{
	v3.SecretType: {},
}

// orderWatchedResources orders the resources in accordance with known push order.
func orderWatchedResources(resources map[string]*model.WatchedResource) []*model.WatchedResource {
	wr := make([]*model.WatchedResource, 0, len(resources))
	// first add all known types, in order
	for _, tp := range PushOrder {
		if w, f := resources[tp]; f {
			wr = append(wr, w)
		}
	}
	// Then add any undeclared types
	for tp, w := range resources {
		if _, f := KnownOrderedTypeUrls[tp]; !f {
			wr = append(wr, w)
		}
	}
	return wr
}

func (s *GenericXdsServer) adsClientCount() int {
	s.adsClientsMutex.RLock()
	defer s.adsClientsMutex.RUnlock()
	return len(s.adsClients)
}

// Register adds the ADS handler to the grpc server
func (s *GenericXdsServer) Register(rpcs *grpc.Server) {
	// Register v3 server
	discovery.RegisterAggregatedDiscoveryServiceServer(rpcs, s)
}

// sdsPushAll implements old style invalidation, generated when any rule or endpoint changes.
// Primary code path is from v1 discoveryService.clearCache(), which is added as a handler
// to the model ConfigStorageCache and Controller.
func (s *GenericXdsServer) sdsPushAll(version string, req *model.PushRequest) {
	if !req.Full {
		log.Infof("XDS: Incremental Pushing: ConnectedEndpoints:%d Version:%s",
			s.adsClientCount(), version)
	} else {
		// Make sure the ConfigsUpdated map exists
		if req.ConfigsUpdated == nil {
			req.ConfigsUpdated = make(map[model.ConfigKey]struct{})
		}
	}

	s.startPush(req)
}

// Send a signal to all connections, with a push event.
func (s *GenericXdsServer) startPush(req *model.PushRequest) {
	req.Start = time.Now()
	for _, c := range s.AllClients() {
		c.pushChannel <- req
	}
}

func (s *GenericXdsServer) addCon(conID string, con *Connection) {
	s.adsClientsMutex.Lock()
	defer s.adsClientsMutex.Unlock()
	s.adsClients[conID] = con
}

func (s *GenericXdsServer) removeCon(conID string) {
	s.adsClientsMutex.Lock()
	defer s.adsClientsMutex.Unlock()

	delete(s.adsClients, conID)
}

// Send with timeout if configured.
func (conn *Connection) send(res *discovery.DiscoveryResponse) error {
	sendHandler := func() error {
		return conn.stream.Send(res)
	}
	err := istiogrpc.Send(conn.stream.Context(), sendHandler)
	if err == nil {
		sz := 0
		for _, rc := range res.Resources {
			sz += len(rc.Value)
		}
		if res.Nonce != "" && !strings.HasPrefix(res.TypeUrl, v3.DebugType) {
			conn.proxy.Lock()
			if conn.proxy.WatchedResources[res.TypeUrl] == nil {
				conn.proxy.WatchedResources[res.TypeUrl] = &model.WatchedResource{TypeUrl: res.TypeUrl}
			}
			conn.proxy.WatchedResources[res.TypeUrl].NonceSent = res.Nonce
			conn.proxy.WatchedResources[res.TypeUrl].VersionSent = res.VersionInfo
			conn.proxy.WatchedResources[res.TypeUrl].LastSent = time.Now()
			conn.proxy.WatchedResources[res.TypeUrl].LastSize = sz
			conn.proxy.Unlock()
		}
	} else if status.Convert(err).Code() == codes.DeadlineExceeded {
		log.Infof("Timeout writing %s", conn.ConID)
	}
	return err
}

// nolint
// Synced checks if the type has been synced, meaning the most recent push was ACKed
func (conn *Connection) Synced(typeUrl string) (bool, bool) {
	conn.proxy.RLock()
	defer conn.proxy.RUnlock()
	acked := conn.proxy.WatchedResources[typeUrl].NonceAcked
	sent := conn.proxy.WatchedResources[typeUrl].NonceSent
	nacked := conn.proxy.WatchedResources[typeUrl].NonceNacked != ""
	sendTime := conn.proxy.WatchedResources[typeUrl].LastSent
	return nacked || acked == sent, time.Since(sendTime) > features.FlowControlTimeout
}

// nolint
func (conn *Connection) Watched(typeUrl string) *model.WatchedResource {
	conn.proxy.RLock()
	defer conn.proxy.RUnlock()
	if conn.proxy.WatchedResources != nil && conn.proxy.WatchedResources[typeUrl] != nil {
		return conn.proxy.WatchedResources[typeUrl]
	}
	return nil
}

func (conn *Connection) Stop() {
	conn.stop <- struct{}{}
}

// Returns the global push context.
func (s *GenericXdsServer) globalPushContext() *model.PushContext {
	s.updateMutex.RLock()
	defer s.updateMutex.RUnlock()
	return s.Env.PushContext
}

// Push an XDS resource for the given connection. Configuration will be generated
// based on the passed in generator. Based on the updates field, generators may
// choose to send partial or even no response if there are no changes.
func (s *GenericXdsServer) pushXds(con *Connection, push *model.PushContext,
	w *model.WatchedResource, req *model.PushRequest) error {
	if w == nil {
		return nil
	}
	gen, _ := s.Generators[w.TypeUrl]
	if gen == nil {
		log.Infof("no generator for resource %s", w.TypeUrl)
		return nil
	}

	res, logDetail, err := gen.Generate(con.proxy, push, w, req)
	if err != nil || res == nil {
		return err
	}

	resp := &discovery.DiscoveryResponse{
		TypeUrl:     w.TypeUrl,
		VersionInfo: push.PushVersion,
		Nonce:       push.PushVersion,
		Resources:   model.ResourcesToAny(res),
	}

	if err := con.send(resp); err != nil {
		log.Warnf("%s: Send failure for node:%s resources:%d %s: %v",
			v3.GetShortType(w.TypeUrl), con.proxy.ID, len(res), logDetail.AdditionalInfo, err)
	}

	log.Debugf("%s: push for node:%s resources:%d %s", v3.GetShortType(w.TypeUrl), req.PushReason(), con.proxy.ID, len(res),
		logDetail.AdditionalInfo)
	return err
}

// AllClients returns all connected clients, per Clients, but additionally includes unintialized connections
// Warning: callers must take care not to rely on the con.proxy field being set
func (s *GenericXdsServer) AllClients() []*Connection {
	s.adsClientsMutex.RLock()
	defer s.adsClientsMutex.RUnlock()
	clients := make([]*Connection, 0, len(s.adsClients))
	for _, con := range s.adsClients {
		clients = append(clients, con)
	}
	return clients
}

// ConfigUpdate implements ConfigUpdater interface, used to request pushes.
// It replaces the 'clear cache' from v1.
func (s *GenericXdsServer) ConfigUpdate(req *model.PushRequest) {
	s.pushChannel <- req
}
