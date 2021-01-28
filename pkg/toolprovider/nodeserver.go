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
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
	badger "github.com/dgraph-io/badger/v3"	
	"k8s.io/utils/keymutex"
)

const buildahPath = "/bin/buildah"

type nodeServer struct {
	nodeID string
	store *metadataStore
	mtxNodeVolumeID keymutex.KeyMutex
}

func NewNodeServer(nodeID string, store *metadataStore) *nodeServer {
	return &nodeServer{
		nodeID: nodeID,
		store: store,
		mtxNodeVolumeID: keymutex.NewHashed(0),
	}
}

type volumeLock struct {
	unlockFunc func()		
} 

func (vl *volumeLock) unlock() {
	vl.unlockFunc()
}

func (ns *nodeServer) lock(volumeID string) *volumeLock {
	ns.mtxNodeVolumeID.LockKey(volumeID)
	return &volumeLock {
		unlockFunc: func() {
			if err := ns.mtxNodeVolumeID.UnlockKey(volumeID); err != nil {
				glog.Error(err)
			}
		},
	}
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// Check arguments
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capability missing in request")
	}
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	volumeLock := ns.lock(volumeID)
	defer volumeLock.unlock()
	
	image := req.GetVolumeContext()["image"]

	// Check the image existence

	glog.V(4).Infof("Checking whether image %s is known", image)

	var err error = nil

	containerID, mountPath, err := ns.store.selectContainerForVolume(volumeID, image)
	if err == badger.ErrEmptyKey {
		glog.Error(err)
		return nil, status.Error(codes.Internal, "Image '" + image + "' was not found in the catalog and could not be mounted in the container")
	}
	if err == badger.ErrConflict {
		glog.Error(err)
		return nil, status.Error(codes.Internal, "Image '" + image + "' is being updated. Please retry later")
	}
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, "Image '" + image + "' connot be mounted into the container for an unexpected reason:" + err.Error())
	}

	// Mount container

	cleaningOnError := func() {
		if cleaningErr := ns.store.dropVolumeContainerOnMountError(containerID, volumeID); cleaningErr != nil {
			glog.Error(err)		
		}
	}

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				glog.Error(err)
				cleaningOnError()
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			glog.Error(err)
			cleaningOnError()
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		cleaningOnError()
		return &csi.NodePublishVolumeResponse{}, nil
	}

	glog.V(4).Infof("Mounting volume target path to container mount")

	options := []string{"bind", "ro"}
	mounter := mount.New("")
	if err := mounter.Mount(mountPath, targetPath, "", options); err != nil {
		glog.Error(err)
		cleaningOnError()
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {

	// Check arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	volumeLock := ns.lock(volumeID)
	defer volumeLock.unlock()

	mounter := mount.New("")
	// Unmounting the image
	err := mounter.Unmount(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	err = ns.store.dropVolumeContainerOnUnmount(volumeID)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func runCmd(execPath string, args ...string) (string, error) {
	cmd := exec.Command(execPath, args...)
	glog.V(6).Infof("Executing command: %s\n", cmd.String())
	output, execErr := cmd.CombinedOutput()
	result := strings.TrimSpace(string(output[:]))
	glog.V(6).Infof("    => Output = %s\n", result)
	return result, execErr
}

func (ns *nodeServer) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return &csi.NodeStageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (ns *nodeServer) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {

	return &csi.NodeGetInfoResponse{
		NodeId: ns.nodeID,
	}, nil
}

func (ns *nodeServer) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {

	return &csi.NodeGetCapabilitiesResponse{
		Capabilities: []*csi.NodeServiceCapability{
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
					},
				},
			},
			{
				Type: &csi.NodeServiceCapability_Rpc{
					Rpc: &csi.NodeServiceCapability_RPC{
						Type: csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
					},
				},
			},
		},
	}, nil
}

func (ns *nodeServer) NodeGetVolumeStats(ctx context.Context, in *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// NodeExpandVolume is only implemented so the driver can be used for e2e testing.
func (ns *nodeServer) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}
