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
	"sync"

	"github.com/opencontainers/go-digest"

	"github.com/golang/glog"
	"golang.org/x/net/context"

	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/containers/image/v5/docker/reference"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/kubernetes/pkg/util/mount"
)

const buildahPath = "/bin/buildah"

var mutex = &sync.Mutex{}

type nodeServer struct {
	nodeID string
}

func NewNodeServer(nodeID string) *nodeServer {
	return &nodeServer{
		nodeID: nodeID,
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

	image := req.GetVolumeContext()["image"]

	// Check the image existence

	mutex.Lock()
	defer mutex.Unlock()

	glog.V(4).Infof("Checking whether image %s is known", image)

	existingDigestString, err := runCmd(buildahPath, "images", "--format={{.Digest}}", image)
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	imageDigest, err := digest.Parse(existingDigestString)
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("Adding image %s to index if necessary", image)

	imageDigest, err = addImageToIndexIfNecessary(image, imageDigest)
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	glog.V(4).Infof("Image %s stored in index with digest: %s", image, imageDigest.String())

	glog.V(4).Infof("Checking whether image %s has already a countainer from it", imageDigest)

	annotations, err := getImageAnnotationsInIndex(imageDigest)
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	container, containerExists := annotations["csi-based-tool-provider.container"]
	if containerExists {
		glog.V(4).Infof("Container already exists for image %s : %s\n", image, container)
	} else {
		glog.V(4).Infof("Creating container from image %s", imageDigest)

		// Build image reference with digest
		imageReference, err := reference.ParseDockerRef(image)
		if err != nil {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		imageName, err := reference.WithName(imageReference.Name())
		if err != nil {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		imageNameWithDigest, err := reference.WithDigest(imageName, imageDigest)
		if err != nil {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Create a container from the image
		container, err = runCmd(buildahPath, "from", imageNameWithDigest.String())
		if err != nil {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}

		// Add the container annotation inside the index
		glog.V(4).Infof("Adding the container annotation for image %s in the index", imageDigest)

		err = addImageAnnotationsInIndex(imageDigest, map[string]string{
			"csi-based-tool-provider.container": container,
		})
		if err != nil {
			glog.V(4).Infof("Index modifiction failed => removing container created from Image %s : %s", image, container)
			_, errRemove := runCmd(buildahPath, "rm", container)
			if errRemove != nil {
				glog.Errorf("Unable to removing container created from Image %s : %s", image, container)
				glog.Error(errRemove)
				return nil, status.Error(codes.Internal, err.Error())
			}
		}
		glog.V(4).Infof("Container created from Image %s : %s", image, container)
	}

	// Mount container

	glog.V(4).Infof("Mounting container %s", container)

	containerMount, err := runCmd(buildahPath, "mount", container)
	if err != nil {
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	glog.V(4).Infof("Container %s mounted at %s", container, containerMount)

	glog.V(4).Infof("Checking volume target path before mounting it")

	notMnt, err := mount.New("").IsLikelyNotMountPoint(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			if err = os.MkdirAll(targetPath, 0750); err != nil {
				glog.Error(err)
				return nil, status.Error(codes.Internal, err.Error())
			}
			notMnt = true
		} else {
			glog.Error(err)
			return nil, status.Error(codes.Internal, err.Error())
		}
	}

	if !notMnt {
		return &csi.NodePublishVolumeResponse{}, nil
	}

	glog.V(4).Infof("Adding the mountpath annotation for volumeId %s to image %s in the index", volumeID, imageDigest)

	err = addImageAnnotationsInIndex(imageDigest, map[string]string{
		"csi-based-tool-provider.volume." + volumeID + ".mountpath": targetPath,
	})

	glog.V(4).Infof("Mounting volume target path to container mount")

	options := []string{"bind", "ro"}
	mounter := mount.New("")
	if err := mounter.Mount(containerMount, targetPath, "", options); err != nil {
		removeImageAnnotationsInIndex(imageDigest, "csi-based-tool-provider.volume."+volumeID+".mountpath")
		glog.Error(err)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func Equal(a, b []string) bool {
	if len(a) != len(b) {

		return false
	}
	for i, v := range a {
		if v != b[i] {
			return false
		}
	}
	return true
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

	mountPathAnnotationName := "csi-based-tool-provider.volume." + volumeID + ".mountpath"

	mutex.Lock()
	defer mutex.Unlock()

	glog.V(4).Infof("Searching an image with a mountpath annotation for volumeId %s in the index", volumeID)

	index, err := getIndex()
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	imageDigests, err := index.filterImagesByAnnotations(mountPathAnnotationName)

	mounter := mount.New("")

	if len(imageDigests) == 0 {
		glog.Warningf("Volume %s not associated to any image in the index", volumeID)
		glog.V(4).Infof("Still unmounting volume %s with minimal cleaning", volumeID)
		// Unmounting the image
		err = mounter.Unmount(targetPath)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	if len(imageDigests) > 1 {
		glog.Warningf("Volume %s associated to several images in the index. Unpublishing the first one...", volumeID)
	}

	imageDigest := imageDigests[0]
	annotations := index.getImageAnnotations(imageDigest)
	mountPath := annotations[mountPathAnnotationName]
	if mountPath != targetPath {
		glog.Warningf("The mountPath %s associated to volume %s does not match the request target path %s", mountPath, volumeID, targetPath)
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}
	
	glog.V(4).Infof("Volume %s had been mounted on image %s at mountpath: %s", volumeID, imageDigest, mountPath)
	
	glog.V(4).Infof("Unmounting volume %s", volumeID)

	// Unmounting the image
	err = mounter.Unmount(targetPath)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	glog.V(4).Infof("Removing mountpath annotation for volumeId %s from image %s in the index", volumeID, imageDigest.String())
	removeImageAnnotationsInIndex(imageDigest, mountPathAnnotationName)

	glog.V(4).Infof("Checking whether other volumes are mounted for image %s in the index", imageDigest.String())
	annotations, err = getImageAnnotationsInIndex(imageDigest)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	otherMounts := false
	for key := range annotations {
		if strings.HasPrefix(key, "csi-based-tool-provider.volume.") {
			otherMounts = true
			break
		}
	}
	if !otherMounts {
		glog.V(4).Infof("No other volume is mounted for image %s in the index => Remove container", imageDigest.String())
		container, containerExists := annotations["csi-based-tool-provider.container"]
		if !containerExists {
			glog.Warningf("The container annotation should exist in image %s inside the index", imageDigest.String())
			return &csi.NodeUnpublishVolumeResponse{}, nil
		}
		_, err := runCmd(buildahPath, "delete", container)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		glog.V(4).Infof("Container removed after the last volume is unmounted %s", container)
		err = removeImageFromIndex(imageDigest)
		if err != nil {
			glog.Warningf("Cannot remove image digest %s annotation from the index: %v", imageDigest, err)
		}
		glog.V(4).Infof("Removed container annotation from image %s in the index", imageDigest.String())
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
