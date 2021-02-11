/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package toolprovider

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
)

func NewNonBlockingGRPCServer() *nonBlockingGRPCServer {
	return &nonBlockingGRPCServer{}
}

// NonBlocking server
type nonBlockingGRPCServer struct {
	wg     sync.WaitGroup
	server *grpc.Server
}

func (s *nonBlockingGRPCServer) Start(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {

	s.wg.Add(1)

	go s.serve(endpoint, ids, cs, ns)

	return
}

func (s *nonBlockingGRPCServer) Wait() {
	s.wg.Wait()
}

func (s *nonBlockingGRPCServer) Stop() {
	s.server.GracefulStop()
}

func (s *nonBlockingGRPCServer) ForceStop() {
	s.server.Stop()
}

func (s *nonBlockingGRPCServer) serve(endpoint string, ids csi.IdentityServer, cs csi.ControllerServer, ns csi.NodeServer) {
	proto, addr, err := parseEndpoint(endpoint)
	if err != nil {
		glog.Fatal(err.Error())
	}

	if proto == "unix" {
		addr = "/" + addr
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) { //nolint: vetshadow
			glog.Fatalf("Failed to remove %s, error: %s", addr, err.Error())
		}
	}

	listener, err := net.Listen(proto, addr)
	if err != nil {
		glog.Fatalf("Failed to listen: %v", err)
	}

	opts := []grpc.ServerOption{
		grpc.UnaryInterceptor(logGRPC),
	}
	server := grpc.NewServer(opts...)
	s.server = server

	if ids != nil {
		csi.RegisterIdentityServer(server, ids)
	}
	if cs != nil {
		csi.RegisterControllerServer(server, cs)
	}
	if ns != nil {
		csi.RegisterNodeServer(server, ns)
	}

	glog.Infof("Listening for connections on address: %#v", listener.Addr())

	server.Serve(listener)

}

func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("Invalid endpoint: %v", ep)
}

var callNumber uint32
var callIDMutex sync.Mutex = sync.Mutex{}

func getCallLogPrefix() string {
	callIDMutex.Lock()
	defer callIDMutex.Unlock()	
	prefix := fmt.Sprintf("GRPC-%08X", callNumber)
	callNumber ++
	return prefix
}

func logGRPC(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	start := time.Now() 
	prefix := getCallLogPrefix()
	grpcCallLogger := buildLogger(prefix, levelGRPCCalls)
	handlerLogger := grpcCallLogger
	volumePublishingLogger := buildLogger(prefix, levelVolumePublishing)
	method := info.FullMethod
	methodParts := strings.Split(method, "/")
	methodName := methodParts[len(methodParts)-1]
	volumePublishingCall := false
	if methodName == "NodePublishVolume" || methodName == "NodeUnpublishVolume" {
		volumePublishingCall = true
	}
	grpcCallLogger.Infof("GRPC call: %s", method)
	grpcCallLogger.Infof("GRPC request: %+v", protosanitizer.StripSecrets(req), req)
	if volumePublishingCall {
		volumePublishingLogger.Infof("Starting %s: ", method)
		handlerLogger = volumePublishingLogger
	}
	resp, err := handler(context.WithValue(ctx, loggerKey, handlerLogger), req)
	duration := time.Now().Sub(start)
	if err != nil {
		grpcCallLogger.Errorf("GRPC error: %v - response: %+v", err, protosanitizer.StripSecrets(resp))
		if volumePublishingCall {
			volumePublishingLogger.Infof("Finished %s in %v with error: %v", method, duration, err)
		}
	} else {
		grpcCallLogger.Infof("GRPC response: %+v", protosanitizer.StripSecrets(resp))
		if volumePublishingCall {
			volumePublishingLogger.Infof("Finished %s in %v successfully", method, duration)
		}
	}
	return resp, err
}
