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
	"io/ioutil"
	"strings"

	"github.com/opencontainers/go-digest"
	"golang.org/x/net/context"
	"k8s.io/kubernetes/pkg/util/mount"

	"github.com/containers/image/v5/docker/reference"
)

const (
	configDirectory = "/etc/toolprovider-catalog-config/"
	imagesConfigFile = "images"
)

func getCatalogImagesFromConfigMap(ctx context.Context) ([]string, error) {
	imagesBytes, err := ioutil.ReadFile(configDirectory + imagesConfigFile)
	if err != nil {
		return nil, err
	}

	return strings.Split(strings.Trim(string(imagesBytes), "\n"), "\n"), nil
}

func getImageDigestFromContainer(ctx context.Context, containerID string) (string, error) {
	// Create a container from the image
	imageDigest, err := runCmd(ctx, buildahPath, "inspect", "--format", "{{.FromImageDigest}}", containerID)
	if err != nil {
		return "", err
	}
	return imageDigest, nil
}

func createContainer(ctx context.Context, image, newDigest string) (string, error) {
	logger := contextLogger(ctx)
	
	// create a new container from image with new digest	
	logger.Infof("Creating container from image %s with digest %s", image, newDigest)

	// Build image reference with digest
	imageReference, err := reference.ParseDockerRef(image)
	if err != nil {
		return "", err
	}

	imageName, err := reference.WithName(imageReference.Name())
	if err != nil {
		return "", err
	}

	imageDigest, err := digest.Parse(newDigest)
	if err != nil {
		return "", err
	}

	imageNameWithDigest, err := reference.WithDigest(imageName, imageDigest)
	if err != nil {
		return "", err
	}

	// Create a container from the image
	containerID, err := runCmd(ctx, buildahPath, "from", imageNameWithDigest.String())
	if err != nil {
		return "", err
	}

	// create a new container from image with new digest	
	logger.Infof("Container created from image %s with digest %s: %s", image, newDigest, containerID)

	return containerID, nil			
}

func mountContainer(ctx context.Context, containerID string) (string, error) {
	logger := contextLogger(ctx)

	logger.Infof("Mounting container %s", containerID)
	containerMount, err := runCmd(ctx, buildahPath, "mount", containerID)
	if err != nil {
		return "", err
	}
	logger.Infof("Container %s mounted at %s", containerID, containerMount)
	
	return containerMount, nil			
}

func deleteContainer(ctx context.Context, containerID string) error {
	logger := contextLogger(ctx)

	logger.Infof("Deleting container %s", containerID)

	// Delete the container
	_, err := runCmd(ctx, buildahPath, "rm", containerID)
	if err != nil {
		logger.Errorf("%v", err)
		return err
	}
	logger.Infof("Container %s deleted", containerID)	
	return nil
}

func updateImages(ctx context.Context, store *metadataStore) {
	logger := contextLogger(ctx)
	images, err := getCatalogImagesFromConfigMap(ctx)
	if err != nil {
		logger.Errorf("%v", err)
		return
	}

	if errs := store.deleteImagesMissingFromCatalog(ctx, images); len(errs) > 0 {
		for _, err := range errs {
			logger.Errorf("%v", err)
		} 
	}
	
	mounter := mount.New("")
	mountPoints, err := mounter.List()
	if err != nil {
		logger.Warningf("Unexpected error when parsng the /proc/mounts file: %s", err)
		mountPoints = []mount.MountPoint{}
	}
	isPathMounted := func(mountPath string) bool {
		if mountPath == "" {
			return false
		}
		for _, mountPoint := range mountPoints {
			if mountPath == mountPoint.Path {
				return true
			}
		}
		return false
	}

	for _, image := range images {

		logger.Infof("Updating image: %s", image)
		newDigest, err := runCmd(ctx, buildahPath, "images", "--format={{.Digest}}", image)
		if err != nil {
			logger.Errorf("Image %s not available : %v", image, err)
			continue
		}
	
		if err = store.updateImage(
			ctx,
			image,
			newDigest,
			getImageDigestFromContainer,
			createContainer,
			isPathMounted,
			mountContainer,
			deleteContainer,
		); err != nil {
			logger.Errorf("%v", err)
		}
	}
}
