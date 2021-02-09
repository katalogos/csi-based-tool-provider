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
	"context"
	"errors"

	"github.com/golang/glog"
)

type toolProvider struct {
	name              string
	nodeID            string
	version           string
	endpoint          string

	ids *identityServer
	ns  *nodeServer
	cs  *controllerServer
}

var (
	vendorVersion = "dev"
)

func init() {
}

func NewToolProviderDriver(driverName, nodeID, endpoint, version string) (*toolProvider, error) {
	if driverName == "" {
		return nil, errors.New("no driver name provided")
	}

	if nodeID == "" {
		return nil, errors.New("no node id provided")
	}

	if endpoint == "" {
		return nil, errors.New("no driver endpoint provided")
	}
	if version != "" {
		vendorVersion = version
	}

	glog.Infof("Driver: %v ", driverName)
	glog.Infof("Version: %s", vendorVersion)

	return &toolProvider{
		name:              driverName,
		version:           vendorVersion,
		nodeID:            nodeID,
		endpoint:          endpoint,
	}, nil
}

func (tp *toolProvider) Run() {
	store := &metadataStore{}
	created, startBackgroundTasks, cleanup := store.init()
	defer cleanup() 

	if created {
		glog.Infof("Created a new metadata store => removing all existing containers")
		runCmd(context.Background(), buildahPath, "rm", "--all")
	}

	startBackgroundTasks()

	// Create GRPC servers
	tp.ids = NewIdentityServer(tp.name, tp.version)
	tp.ns = NewNodeServer(tp.nodeID, store)
	tp.cs = NewControllerServer(tp.nodeID)

	s := NewNonBlockingGRPCServer()
	s.Start(tp.endpoint, tp.ids, tp.cs, tp.ns)
	s.Wait()
}
