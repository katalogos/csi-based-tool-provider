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

package common

import (
	"github.com/containers/image/v5/docker/reference"
	"github.com/opencontainers/go-digest"
)

func ImageWithDigest(image, theDigest string) (string, error) {
	
	// Build image reference with digest
	imageReference, err := reference.ParseDockerRef(image)
	if err != nil {
		return "", err
	}

	imageName, err := reference.WithName(imageReference.Name())
	if err != nil {
		return "", err
	}

	imageDigest, err := digest.Parse(theDigest)
	if err != nil {
		return "", err
	}

	imageNameWithDigest, err := reference.WithDigest(imageName, imageDigest)
	if err != nil {
		return "", err
	}

	return imageNameWithDigest.String(), nil			
}
