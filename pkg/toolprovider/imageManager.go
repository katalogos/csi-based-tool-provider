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
	"github.com/opencontainers/go-digest"

	"github.com/golang/glog"

	"github.com/containers/image/v5/docker/reference"
)

func getCatalogImagesFromConfigMap() ([]string, error) {
	return []string {
		"quay.io/dfestal/csi-tool-maven-3.6.3:latest",
		"quay.io/dfestal/csi-tool-openjdk11u-jdk_x64_linux_hotspot_11.0.9.1_1:latest",
	}, nil
}

func getImageDigestFromContainer(containerID string) (string, error) {
	// Create a container from the image
	imageDigest, err := runCmd(buildahPath, "inspect", "--format", "{{.FromImageDigest}}", containerID)
	if err != nil {
		return "", err
	}
	return imageDigest, nil
}

func createContainer(image, newDigest string) (string, string, error) {
	// create a new container from image with new digest
	
	glog.V(4).Infof("Creating container from image %s with digest %s", image, newDigest)

	// Build image reference with digest
	imageReference, err := reference.ParseDockerRef(image)
	if err != nil {
		return "", "", err
	}

	imageName, err := reference.WithName(imageReference.Name())
	if err != nil {
		return "", "", err
	}

	imageDigest, err := digest.Parse(newDigest)
	if err != nil {
		return "", "", err
	}

	imageNameWithDigest, err := reference.WithDigest(imageName, imageDigest)
	if err != nil {
		return "", "", err
	}

	// Create a container from the image
	containerID, err := runCmd(buildahPath, "from", imageNameWithDigest.String())
	if err != nil {
		return "", "", err
	}

	glog.V(4).Infof("  => Container created from Image %s : %s", image, containerID)

	glog.V(4).Infof("Mounting container %s", containerID)
	containerMount, err := runCmd(buildahPath, "mount", containerID)
	if err != nil {
		return "", "", err
	}
	glog.V(4).Infof("Container %s mounted at %s", containerID, containerMount)
	
	return containerID, containerMount, nil			
}

func deleteContainer(containerID string) error {
	// Create a container from the image
	_, err := runCmd(buildahPath, "rm", containerID)
	if err != nil {
		glog.Error(err)
	}
	return err
}

func updateImages(store *metadataStore) {
	images, err := getCatalogImagesFromConfigMap()
	if err != nil {
		glog.Error(err)
		return
	}

	if errs := store.deleteImagesMissingFromCatalog(images); len(errs) > 0 {
		for _, err := range errs {
			glog.Error(err)
		} 
	}
	
	for _, image := range images {
		newDigest, err := runCmd(buildahPath, "images", "--format={{.Digest}}", image)
		if err != nil {
			glog.Error("Image " + image + " not available !")
			glog.Error(err)
			continue
		}
	
		if err = store.updateImage(
			image,
			newDigest,
			getImageDigestFromContainer,
			createContainer,
			deleteContainer,
		); err != nil {
			glog.Error(err)
		}
	}
}
