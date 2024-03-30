// Copyright Istio Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package model

import "istio.io/istio/pkg/cluster"

// XDSUpdater is used for direct updates of the xDS model and incremental push.
// Pilot uses multiple registries - for example each K8S cluster is a registry
// instance. Each registry is responsible for tracking a set
// of endpoints associated with mesh services, and calling the EDSUpdate on changes.
// A registry may group endpoints for a service in smaller subsets - for example by
// deployment, or to deal with very large number of endpoints for a service. We want
// to avoid passing around large objects - like full list of endpoints for a registry,
// or the full list of endpoints for a service across registries, since it limits
// scalability.
//
// Future optimizations will include grouping the endpoints by labels, gateway or region to
// reduce the time when subsetting or split-horizon is used. This design assumes pilot
// tracks all endpoints in the mesh and they fit in RAM - so limit is few M endpoints.
// It is possible to split the endpoint tracking in future.
type XDSUpdater interface {
	// EDSUpdate is called when the list of endpoints or labels in a Service is changed.
	// For each cluster and hostname, the full list of active endpoints (including empty list)
	// must be sent. The shard name is used as a key - current implementation is using the
	// registry name.
	EDSUpdate(shard ShardKey, hostname string, namespace string, entry []*IstioEndpoint)

	// EDSCacheUpdate is called when the list of endpoints or labels in a Service is changed.
	// For each cluster and hostname, the full list of active endpoints (including empty list)
	// must be sent. The shard name is used as a key - current implementation is using the
	// registry name.
	// Note: the difference with `EDSUpdate` is that it only update the cache rather than requesting a push
	EDSCacheUpdate(shard ShardKey, hostname string, namespace string, entry []*IstioEndpoint)

	// SvcUpdate is called when a service definition is updated/deleted.
	SvcUpdate(shard ShardKey, hostname string, namespace string, event Event)

	// ConfigUpdate is called to notify the XDS server of config updates and request a push.
	// The requests may be collapsed and throttled.
	ConfigUpdate(req *PushRequest)

	// ProxyUpdate is called to notify the XDS server to send a push to the specified proxy.
	// The requests may be collapsed and throttled.
	ProxyUpdate(clusterID cluster.ID, ip string)

	// RemoveShard removes all endpoints for the given shard key
	RemoveShard(shardKey ShardKey)
}
