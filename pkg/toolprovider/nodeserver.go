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
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/golang/glog"
	"github.com/katalogos/csi-based-tool-provider/pkg/common"
	"github.com/prometheus/client_golang/prometheus"
	"context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	badger "github.com/dgraph-io/badger/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/mount-utils"
	"k8s.io/utils/keymutex"
	utilpath "k8s.io/utils/path"	
)

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

func isDirEmpty(path string) (bool, error) {	
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	_, err = f.Readdirnames(1)
	if err != nil {
		if err == io.EOF {
			return true, nil
		}
		return false, err
	}
	return false, nil 
}

func (ns *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	var metricStatus string
	var metricCatalog string
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		getVolumeMountLatency.WithLabelValues(metricStatus, metricCatalog).Observe(v)
	}))
	defer func() {
		timer.ObserveDuration()
	}()
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

	logger := common.ContextLogger(ctx)
	volumeLock := ns.lock(volumeID)
	defer volumeLock.unlock()
	
	image := req.GetVolumeContext()["image"]

	// Check the image existence

	logger.Infof("Checking whether image %s is known", image)

	var err error = nil

	containerID, catalog, mountPath, err := ns.store.selectContainerForVolume(volumeID, image)
	metricCatalog = catalog
	if err == badger.ErrEmptyKey {
		logger.Errorf("%v", err)
		metricStatus = "ImageNotFoundError"
		return nil, status.Error(codes.NotFound, "Image '" + image + "' was not found in the catalog and could not be mounted in the container")
	}
	if err == badger.ErrConflict {
		logger.Errorf("%v", err)
		metricStatus = "MetadataStoreConflictError"
		return nil, status.Error(codes.Aborted, "Image '" + image + "' is being updated. Please retry later")
	}
	if err != nil {
		logger.Errorf("%v", err)
		metricStatus = "UnexpectedError"
		return nil, status.Error(codes.Unknown, "Image '" + image + "' cannot be mounted into the container for an unexpected reason:" + err.Error())
	}

	// Mount container

	cleaningOnError := func() {
		if cleaningErr := ns.store.dropVolumeContainerOnMountError(ctx, containerID, volumeID); cleaningErr != nil {
			logger.Errorf("%v", err)		
		}
	}

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {	
			logger.Infof("Creating target path for volume %s: %s", volumeID, targetPath)		
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				logger.Errorf("%v", err)
				cleaningOnError()
				metricStatus = "UnexpectedError"
				return nil, status.Error(codes.Unknown, err.Error())
			}
			notMnt = true
		} else {
			logger.Errorf("%v", err)
			cleaningOnError()
			metricStatus = "UnexpectedError"
			return nil, status.Error(codes.Unknown, err.Error())
		}
	}

	if !notMnt {
		// It IS a mount point.
		// Target path is non-bind IsDirEmpty point
		// That doesn't seem normal.
		//   => Trigger an error
		cleaningOnError()
		metricStatus = "UnexpectedError"
		return nil, status.Error(codes.FailedPrecondition, "Target path for volume " + volumeID + " is already a mount point: " + targetPath)
	}

	containerMountPathIsEmpty, err := isDirEmpty(mountPath)
	if err != nil {
		cleaningOnError()
		if os.IsNotExist(err) {
			metricStatus = "ContainerEmpty"
			return nil, status.Error(codes.Aborted, "MountPath  " + mountPath + " for container " + containerID + " does not exist")
		}
		metricStatus = "UnexpectedError"
		return nil, status.Error(codes.Unknown, "MountPath " + mountPath + " for volume " + volumeID + " cannot be checked due to error: " + err.Error())
	}

	if containerMountPathIsEmpty {
		metricStatus = "ContainerEmpty"
		return nil, status.Error(codes.Aborted, "MountPath  " + mountPath + " for container " + containerID + " is empty")
	}

	logger.Infof("Mounting volume target path to container mount")

	options := []string{"bind", "ro"}
	mounter := mount.New("")
	if err := mounter.Mount(mountPath, targetPath, "", options); err != nil {
		logger.Errorf("%v", err)
		metricStatus = "MountError"
		cleaningOnError()
		return nil, status.Error(codes.Unknown, err.Error())
	}

	metricStatus = "Success"
	return &csi.NodePublishVolumeResponse{}, nil
}

func (ns *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	logger := common.ContextLogger(ctx)
	metricStatus := "Success"
	timer := prometheus.NewTimer(prometheus.ObserverFunc(func(v float64) {
		getVolumeUnmountLatency.WithLabelValues(metricStatus).Observe(v)
	}))
	defer func() {
		timer.ObserveDuration()
	}()

	// Check arguments
	volumeID := req.GetVolumeId()
	if len(volumeID) == 0 {
		metricStatus = "InvalidArgument"
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	targetPath := req.GetTargetPath()
	if len(targetPath) == 0 {
		metricStatus = "InvalidArgument"
		return nil, status.Error(codes.InvalidArgument, "Target path missing in request")
	}

	volumeLock := ns.lock(volumeID)
	defer volumeLock.unlock()

	mounter := mount.New("")
	// Unmounting the image
	err := mounter.Unmount(targetPath)
	if err != nil {
		logger.Warningf("Unmount failed: %v", err)
		existsPath, _ := utilpath.Exists(utilpath.CheckFollowSymlink, (targetPath))
		if existsPath {
			logger.Infof("Removing mount target path: %s", targetPath)
			args := []string{"-Rf", targetPath}
			command := exec.Command("rm", args...)
			output, err := command.CombinedOutput()
			if err != nil {
				args := strings.Join(args, " ")
				logger.Warningf("Removal failed: %v\nCommand: %s\nArguments: %s\nOutput: %s\n", err, "rm", args, string(output))
			}
		}
		metricStatus = "UnmountError"
	}

	err = ns.store.dropVolumeContainerOnUnmount(ctx, volumeID)
	if err != nil {
		metricStatus = "MetadataStoreUpdateError"
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
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
