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
	"encoding/json"
	"errors"
	"strings"
	imageSpec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	indexName = "csi-based-tool-provider/index:latest"
)

type index struct {
	imageSpec.Index
}

func createIndexIfNecessary() (created bool, err error) {
	output, err := runCmd(buildahPath, "images", "--quiet", indexName)
	if err != nil {
		if ! strings.Contains(output, "No such image") {
			return false, err
		}
		output = ""
	}
	if output != "" {
		return false, nil
	}

	_, err = runCmd(buildahPath, "manifest", "create", indexName)
	if err != nil {
		return false, err
	}
	return true, nil
}

func addImageToIndexIfNecessary(image string, existingImageDigest digest.Digest) (digest.Digest, error) {
	index, err := getIndex()
	if err != nil {
		return "", err
	}

	if index.findManifest(existingImageDigest) != nil {
		return existingImageDigest, nil
	}

	output, err := runCmd(buildahPath, "manifest", "add", indexName, "docker://"+image, "--annotation", "csi-based-tool-provider.imageName="+image)
	imageDigest := ""
	if err != nil {
		return "", err
	}
	parts := strings.Split(output, ": ")
	if len(parts) > 1 {
		imageDigest = parts[1]
	} else {
		return "", errors.New("Unexpected output of 'buildah manifest add' command: " + output)
	}
	digest, err := digest.Parse(imageDigest)
	if err != nil {
		return "", err
	}
	return digest, nil
}

func getIndex() (*index, error) {
	index := &index {}
	output, err := runCmd(buildahPath, "manifest", "--registries-conf=unexisting.conf", "inspect", indexName)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(output), index)
	if err != nil {
		return nil, err
	}
	return index, nil
}

func (i *index) findManifest(imageDigest digest.Digest) *imageSpec.Descriptor {
	for _, manifest := range i.Manifests {
		if manifest.Digest == imageDigest {
			return &manifest
		}
	}
	return nil
}

func (i *index) getImageAnnotations(imageDigest digest.Digest) map[string]string {
	manifest := i.findManifest(imageDigest)
	if manifest != nil {
		return manifest.Annotations
	}
	return nil
}

func getImageAnnotationsInIndex(imageDigest digest.Digest) (map[string]string, error) {
	index, err := getIndex()
	if err != nil {
		return nil, err
	}
	manifest := index.findManifest(imageDigest)
	if manifest != nil {
		return manifest.Annotations, nil
	}
	return nil, nil
}

func setImageAnnotationsInIndex(imageDigest digest.Digest, annotations map[string]string) error  {
	args := []string{ "manifest", "annotate", indexName, string(imageDigest) }
	for key, value := range annotations {
		args = append(args, "--annotation", key + "=" + value)
	}

	_, err := runCmd(buildahPath, args...)
	if err != nil {
		return err
	}
	return nil
}

func addImageAnnotationsInIndex(imageDigest digest.Digest, annotations map[string]string) error {
	index, err := getIndex()
	if err != nil {
		return err
	}
	existingAnnotations := index.getImageAnnotations(imageDigest)
	for key, value := range annotations {
		existingAnnotations[key] = value 
	}
	return setImageAnnotationsInIndex(imageDigest, existingAnnotations)
}

func removeImageAnnotationsInIndex(imageDigest digest.Digest, annotations ...string) error {
	index, err := getIndex()
	if err != nil {
		return err
	}
	existingAnnotations := index.getImageAnnotations(imageDigest)
	for _, key := range annotations {
		delete(existingAnnotations, key) 
	}
	return setImageAnnotationsInIndex(imageDigest, existingAnnotations)
}

func removeImageFromIndex(imageDigest digest.Digest) error {
	_, err := runCmd(buildahPath, "manifest", "remove", indexName, imageDigest.String())
	return err
}

func (i *index) filterImagesByAnnotations(annotations ...string) ([]digest.Digest, error) {
	result := []digest.Digest {}	
	NextManifest:
	for _, manifest := range i.Manifests {
		for _, annotation := range annotations {
			if _, exists := manifest.Annotations[annotation]; !exists {
				continue NextManifest
			}
		}
		result = append(result, manifest.Digest)
	}
	
	return result, nil
}

func filterImagesByAnnotationsInIndex(annotations ...string) ([]digest.Digest, error) {
	index, err := getIndex()
	if err != nil {
		return nil, err
	}

	return index.filterImagesByAnnotations(annotations...)
}
