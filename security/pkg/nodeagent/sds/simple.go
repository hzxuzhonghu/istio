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
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"istio.io/istio/pilot/pkg/model"
	"istio.io/istio/pkg/config/mesh"
)

// Server represents the XDS serving feature of Istiod (pilot).
// Unlike bootstrap/, this packet has no dependencies on K8S, CA,
// and other features. It'll be used initially in the istio-agent,
// to provide a minimal proxy while reusing the same code as istiod.
// Portions of the code will also be used in istiod - after it becomes
// stable the plan is to refactor bootstrap to use this code instead
// of directly bootstrapping XDS.
//
// The server support proxy/federation of multiple sources - last part
// or parity with MCP/Galley and MCP-over-XDS.
type SimpleServer struct {
	// DiscoveryServer is the gRPC XDS implementation
	// Env and MemRegistry are available as fields, as well as the default
	// PushContext.
	DiscoveryServer *GenericXdsServer

	// GRPCListener is the listener used for GRPC. For agent it is
	// an insecure port, bound to 127.0.0.1
	GRPCListener net.Listener
}

// Creates an basic, functional discovery server.
func newServer() *SimpleServer {
	env := &model.Environment{
		PushContext: model.NewPushContext(),
	}
	mc := mesh.DefaultMeshConfig()
	env.Watcher = mesh.NewFixedWatcher(&mc)
	env.PushContext.Mesh = env.Watcher.Mesh()
	env.Init()

	ds := NewGenericXdsServer(env)
	s := &SimpleServer{
		DiscoveryServer: ds,
	}

	return s
}

func (s *SimpleServer) StartGRPC(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	gs := grpc.NewServer()
	s.DiscoveryServer.Register(gs)
	reflection.Register(gs)
	s.GRPCListener = lis
	go func() {
		err = gs.Serve(lis)
		if err != nil {
			log.Info("Serve done ", err)
		}
	}()
	return nil
}
