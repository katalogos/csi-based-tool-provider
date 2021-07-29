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
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/google/renameio"
	"github.com/hashicorp/go-multierror"
	"github.com/katalogos/csi-based-tool-provider/pkg/common"
	//	corev1 "k8s.io/api/core/v1"
)

type PullRound string
const (
	PullRoundTicker PullRound = "PullRoundTicker"
	PullRoundConfigChanged PullRound = "PullRoundConfigChanged"
)

type SoftwareCatalogsAdapter interface {
	ImagePuller(channel chan PullRound)
	ImageCleaner(channel <-chan time.Time)
}

func NewSoftwareCatalogsAdapter(catalogConfigFile, toolproviderImagesPath string, neverPull bool, transportToPullFrom string, retryDelay func()time.Duration) softwareCatalogsAdapter {
	return softwareCatalogsAdapter{
		CatalogConfigFile: catalogConfigFile,
		ToolproviderImagesPath: toolproviderImagesPath, 
		NeverPull: neverPull,
		TransportToPullFrom: transportToPullFrom,
		RetryDelay: retryDelay,
	}
}

type imageIDAndCatalog struct {
	imageID string
	catalogName string
}

func (ic imageIDAndCatalog) String() string {
	return ic.imageID + "," + ic.catalogName
} 

type softwareCatalogsAdapter struct {
	CatalogConfigFile string

	mu sync.Mutex

	// Map of ImageIDs that have been pulled.
	// Keys are image references (possibly with tags)
	images map[string]imageIDAndCatalog

	NeverPull bool 
	
	TransportToPullFrom string
	ToolproviderImagesPath string
	SoftwareCatalogs map[string]SoftwareCatalog
	RetryDelay func()time.Duration
}

func (sca *softwareCatalogsAdapter) pullImages(ctx context.Context, pullRound PullRound) error {
	logger := common.ContextLogger(ctx)
	sca.mu.Lock()
	defer sca.mu.Unlock()

	if pullRound == PullRoundConfigChanged {
		logger.Infof("Updating the list of software catalogs")
		var err error
		sca.SoftwareCatalogs, err = sca.getCatalogsFromConfigMap()
		if err != nil {
			logger.Errorf("Error Getting the Catalogs config map: %v", err)
			return err
		}
	}

	existingImages := ""
	existingImagesBytes, err := ioutil.ReadFile(sca.ToolproviderImagesPath)
	if err == nil {
		existingImages = string(existingImagesBytes)
	} else if !os.IsNotExist(err) {
		logger.Errorf("Error reading the images file: %v", err)
	}
	var errors *multierror.Error
	sca.images = make(map[string]imageIDAndCatalog, len(sca.images))
	for catalogName, catalog := range sca.SoftwareCatalogs {
		for _, entry := range catalog.Entries {
			if _, exists := sca.images[entry.Image]; exists {
				if sca.NeverPull {
					continue
				}
				if catalog.DontNeedImageUpdates {
					continue
				}
			}
	
			buildahPolicy := "--policy=always"
			if sca.NeverPull {
				buildahPolicy = "--policy=never"
			}
			if catalog.DontNeedImageUpdates {
				buildahPolicy = "--policy=missing"
			}

			imageID, err := common.RunCmd(ctx, common.BuildahPath, "pull", buildahPolicy, "--quiet", sca.TransportToPullFrom + entry.Image)
			if err != nil {
				logger.Errorf("Error pulling image %s from catalog %s: %v", entry.Image, catalogName, err)
				multierror.Append(errors, err)
				continue
			}

			sca.images[entry.Image] = imageIDAndCatalog{
				imageID: imageID,
				catalogName: catalogName,
			}
		}
	}

	var toolproviderImagesContent string
	keys := make([]string, 0, len(sca.images))
	for k := range sca.images {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		toolproviderImagesContent += k + "=" + sca.images[k].String() + "\n"
	}

	if existingImages != toolproviderImagesContent {
		err := renameio.WriteFile(sca.ToolproviderImagesPath, []byte(toolproviderImagesContent), 0666)
		if err != nil {
			logger.Errorf("Error overriding the toolingprovider images config file %s: %v", sca.ToolproviderImagesPath, err)
			multierror.Append(errors, err)
		}
	}


	return errors.ErrorOrNil()
}

func (sca *softwareCatalogsAdapter) ImagePuller(channel chan PullRound) {
	ctx := common.WithLogger(common.BuildLogger("ImagePuller", common.LevelImageManager))
	err := sca.pullImages(ctx, PullRoundConfigChanged)
	if err != nil {
		time.AfterFunc(sca.RetryDelay(), func(){
			channel <- PullRoundConfigChanged
		})
	} 
	for pullRound := range channel {
		err := sca.pullImages(ctx, pullRound)
		if err != nil && pullRound == PullRoundConfigChanged {
			time.AfterFunc(sca.RetryDelay(), func(){
				channel <- PullRoundConfigChanged
			})
		}
	}
}

func (sca *softwareCatalogsAdapter) cleanImages(ctx context.Context) {
	logger := common.ContextLogger(ctx)
	sca.mu.Lock()
	defer sca.mu.Unlock()

	logger.Infof("Image cleaning is not implemented for now",)

	// read the content of the images get get the set of used ImageIDs
	// get the list of used ImageIDs from the endpoint from the toolprovider
	// try to remove all buildh images whose imageID is not in this built list
}

func (sca *softwareCatalogsAdapter) ImageCleaner(channel <-chan time.Time) {
	ctx := common.WithLogger(common.BuildLogger("ImageCleaner", common.LevelImageManager))
	for range channel {
		sca.cleanImages(ctx)
	}
}

func (sca *softwareCatalogsAdapter) getCatalogsFromConfigMap() (map[string]SoftwareCatalog, error) {
	imagesBytes, err := ioutil.ReadFile(sca.CatalogConfigFile)
	if err != nil {
		return nil, err
	}

	result := map[string]SoftwareCatalog{}
	err = json.Unmarshal(imagesBytes, &result)
	if err != nil {
		return nil, err
	}
	
	return result, nil
}

