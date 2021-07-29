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

package catalogs

import (
	corev1 "k8s.io/api/core/v1"
)

type SoftwareCatalog struct {
	Description string `json:"description,omitempty"`
	Entries map[string]SoftwareCatalogEntry `json:"entries"`
	DontNeedImageUpdates bool `json:"dontNeedImageUpdates,omitempty"`

	// Use corev1/helpers/MatchNodeSelectorTerms to resolve this.
	// A nil value supports all nodes.
	NodeSelector *corev1.NodeSelector`json:"nodeSelector,omitempty"`
}

type SoftwareCatalogEntry struct {
	Description string `json:"description,omitempty"`
	Image string `json:"image"`
	SubPath string `json:"subPath,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
}
