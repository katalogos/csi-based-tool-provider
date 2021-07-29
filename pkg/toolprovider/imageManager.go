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

	"context"
	"k8s.io/mount-utils"

	"github.com/katalogos/csi-based-tool-provider/pkg/common"
)

type imageDescription struct {
	imageID string
	catalog string
}

func getCatalogImagesFromConfigMap(ctx context.Context, imagesConfigFile string) (map[string]imageDescription, error) {
	imagesBytes, err := ioutil.ReadFile(imagesConfigFile)
	if err != nil {
		return nil, err
	}

	result := map[string]imageDescription{}
	for _, imageLine := range strings.Split(strings.Trim(string(imagesBytes), "\n"), "\n") {
		if strings.TrimSpace(imageLine) == "" {
			continue
		}
		imageLineParts := strings.Split(imageLine, "=")
		imageName := strings.Trim(imageLineParts[0], " ")
		var imageID string
		var catalog string
		if len(imageLineParts) > 1 {
			imageDescriptionParts := strings.SplitN(imageLineParts[1], ",", 2)
			imageID = strings.Trim(imageDescriptionParts[0], " ")
			if len(imageDescriptionParts) > 1 {
				catalog = strings.Trim(imageDescriptionParts[1], " ")
			}
		}
		result[imageName] = imageDescription{
			imageID: imageID,
			catalog: catalog,
		} 
	}
	
	return result, nil
}

func getImageIDFromContainer(ctx context.Context, containerID string) (string, error) {
	// Create a container from the image
	imageID, err := common.RunCmd(ctx, common.BuildahPath, "inspect", "--format", "{{.FromImageID}}", containerID)
	if err != nil {
		return "", err
	}

	return imageID, nil
}

func createContainer(ctx context.Context, image, newImageID string) (string, error) {
	logger := common.ContextLogger(ctx)
	
	// create a new container from image with new ImageID	
	logger.Infof("Creating container from image %s with imageID %s", image, newImageID)

	// Create a container from the image
	containerName, err := common.RunCmd(ctx, common.BuildahPath, "from", "--pull-never", newImageID)
	if err != nil {
		return "", err
	}

	// Create a container from the image
	containerID, err := common.RunCmd(ctx, common.BuildahPath, "containers", "-q", "--filter", "name=" + containerName)
	if err != nil {
		return "", err
	}

	// create a new container from image with new digest	
	logger.Infof("Container created from image %s with imageID %s: %s", image, newImageID, containerID)

	return containerID, nil			
}

func mountContainer(ctx context.Context, containerID string) (string, error) {
	logger := common.ContextLogger(ctx)

	logger.Infof("Mounting container %s", containerID)
	containerMount, err := common.RunCmd(ctx, common.BuildahPath, "mount", containerID)
	if err != nil {
		return "", err
	}
	logger.Infof("Container %s mounted at %s", containerID, containerMount)
	
	return containerMount, nil			
}

func deleteContainer(ctx context.Context, containerID string) error {
	logger := common.ContextLogger(ctx)

	logger.Infof("Deleting container %s", containerID)

	// Delete the container
	_, err := common.RunCmd(ctx, common.BuildahPath, "rm", containerID)
	if err != nil {
		logger.Errorf("%v", err)
		return err
	}
	logger.Infof("Container %s deleted", containerID)	
	return nil
}

func updateImages(ctx context.Context, store *metadataStore, imagesConfigFile string) {
	logger := common.ContextLogger(ctx)
	images, err := getCatalogImagesFromConfigMap(ctx, imagesConfigFile)
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
		logger.Warningf("Unexpected error when parsing the /proc/mounts file: %s", err)
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

	for image, imageDescription := range images {
		logger.Infof("Updating image: %s", image)
		imageToSearch := imageDescription.imageID
		if imageToSearch == "" {
			imageToSearch = image
		}

		newImageID, err := common.RunCmd(ctx, common.BuildahPath, "pull", "--policy", "never", imageToSearch)
		if err != nil {
			logger.Errorf("Image %s not available : %v", image, err)
			continue
		}

		if err = store.updateImage(
			ctx,
			image,
			newImageID,
			imageDescription.catalog,
			getImageIDFromContainer,
			createContainer,
			isPathMounted,
			mountContainer,
			deleteContainer,
		); err != nil {
			logger.Errorf("%v", err)
		}
	}
}
