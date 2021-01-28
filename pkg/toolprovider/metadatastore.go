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
	"strings"
	"os"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/golang/glog"
)

const (
	imageTagPrefix = "imageTag."
	containerIDSuffix = ".containerId"
	containerPrefix = "container."
	mountPathSuffix = ".mountPath"
	volumePrefix = "volume."
	volumeMidFix = ".volume."
	containerToCleanPrefix = "containerToClean."
)

type keyNames struct {
}

func (keyNames) containerForImage(image string) []byte {
	return []byte(imageTagPrefix + image + containerIDSuffix)
}

func (keyNames) containerMountPath(containerID string) []byte {
	return []byte(containerPrefix + containerID + mountPathSuffix)
}

func (keyNames) containerForVolume(volumeID string) []byte {
	return []byte(volumePrefix + volumeID + containerIDSuffix)
}

func (keyNames) volumeOnContainer(volumeID, containerID string) []byte {
	return []byte(containerPrefix + containerID + volumeMidFix + volumeID)
}

func (keyNames) containerToClean(containerID string) []byte {
	return []byte(containerToCleanPrefix + containerID)
}

var keys = keyNames {} 


func (store *metadataStore) selectContainerForVolume(volumeID, image string) (string, string, error) {
	containerID := ""
	mountPath := ""
	err := store.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(keys.containerForImage(image))
		if err != nil {
			return err
		}
		if err = item.Value(func(containerIDBytes []byte) error {
			containerID = string(containerIDBytes)
			return nil
		}); err != nil {
			return err
		}
		item, err = txn.Get(keys.containerMountPath(containerID))
		if err != nil {
			return err
		}
		if err = item.Value(func(mountPathBytes []byte) error {
			mountPath = string(mountPathBytes)
			return nil
		}); err != nil {
			return err
		}
		if err = txn.Set(keys.containerForVolume(volumeID), []byte(containerID)); err != nil {
			return err
		}
		if err = txn.Set(keys.volumeOnContainer(volumeID, containerID), []byte(image)); err != nil {
			return err
		}

		return nil
	})

	return containerID, mountPath, err
}

func (store *metadataStore) deleteImagesMissingFromCatalog(images []string) (errors []error) {
	imageSet := map[string]int {}
	for _, image := range images {
		imageSet[string(keys.containerForImage(image))] = 0
	}
	store.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		keysToDelete := [][]byte{}
		prefix := []byte(imageTagPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		  item := it.Item()
		  if _, isThere := imageSet[string(item.Key())]; !isThere {
			keysToDelete = append(keysToDelete, item.KeyCopy(nil))
		  }
		}
		for _, keyToDelete := range keysToDelete {
			if err := txn.Delete(keyToDelete); err != nil {
				errors = append(errors, err)
			}
		} 
		return nil
	})
	return errors
}

func (store *metadataStore) updateImage(
	image string,
	newDigest string,
	getImageDigestFromContainer func(containerID string) (string, error),
	createContainer func(image, newDigest string) (string, string, error),
	deleteContainer func(containerID string) error,
) error {
	return store.db.Update(func(txn *badger.Txn) error {
		containerID := ""
		currentDigest := ""
		item, err := txn.Get(keys.containerForImage(image))
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if err == nil {
			containerIDBytes, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			containerID := string(containerIDBytes)
			currentDigest, err = getImageDigestFromContainer(containerID)
			if err != nil {
				return err
			}
		}
	
		if newDigest == currentDigest {
			// Nothing to update. The image Sha is the same and it already has a container created
			return nil
		}
	
		if containerID != "" {
			err := txn.Set(keys.containerToClean(containerID), []byte{})
			if err != nil {
				return err
			}
		}

		newContainerID, mountPath, err := createContainer(image, newDigest) 		
		if err != nil {
			return err
		}
	
		err = txn.Set(keys.containerForImage(image), []byte(newContainerID))
		if err != nil {
			deleteContainer(newContainerID)
			return err
		}
		err = txn.Set(keys.containerMountPath(newContainerID), []byte(mountPath))
		if err != nil {
			deleteContainer(newContainerID)
			return err
		}

		return nil
	})
}

func (store *metadataStore) getContainersToDelete() ([]string, error) {
	containersToDelete := []string{}
	if err := store.db.Update(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		prefix := []byte(containerToCleanPrefix)
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		  item := it.Item()
		  containerID := strings.TrimPrefix(string(item.Key()), containerToCleanPrefix)
		  prefix := []byte(containerPrefix + containerID + volumeMidFix)
		  containerStillHasMountedVolumes := false
		  func() {
			it := txn.NewIterator(opts)
			defer it.Close()
			for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
				containerStillHasMountedVolumes = true
				break;
			}		  
		  }()
		  if !containerStillHasMountedVolumes {
			containersToDelete = append(containersToDelete, containerID)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return containersToDelete, nil
}

func (store *metadataStore) dropVolumeContainerOnMountError(containerID, volumeID string) error {
	return store.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete(keys.containerForVolume(volumeID)); err != nil {
			return err
		}
		if err := txn.Delete(keys.volumeOnContainer(volumeID, containerID)); err != nil {
			return err
		}
		return nil
	})
}

func (store *metadataStore) dropVolumeContainerOnUnmount(volumeID string) error {
	return store.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(keys.containerForVolume(volumeID))
		if err != nil {
			return err
		}

		err = item.Value(func(containerIDBytes []byte) error {
			containerID := string(containerIDBytes)
			if err = txn.Delete(keys.containerForVolume(volumeID)); err != nil {
				return err
			}
			if err = txn.Delete(keys.volumeOnContainer(volumeID, containerID)); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
}

func (store *metadataStore) garbageCollector(ticker *time.Ticker) {
	for range ticker.C {
	again:
		err := store.db.RunValueLogGC(0.7)
		if err == nil {
			goto again
		}
	}
}

func (store *metadataStore) imageManager(ticker *time.Ticker) {
	for range ticker.C {
		updateImages(store)
	}
}

func (store *metadataStore) containerCleaner(ticker *time.Ticker) {
	for range ticker.C {
		cleanContainers(store)
	}
}

func (store *metadataStore) init(logger badger.Logger) (createdStore bool, cleanup func()) {
	glog.Infof("Creating index if necessary")
	created := false
	if _, err := os.Stat("/var/lib/toolprovider-metadata-store"); err != nil && os.IsNotExist(err) {
		created = true
	}

	var err error = nil
	options := badger.DefaultOptions("/var/lib/toolprovider-metadata-store")
	options.Logger = logger
	store.db, err = badger.Open(options)
	if err != nil {
		glog.Fatalf("Failed to create metadata store: %v", err)
	}

	gcTicker := time.NewTicker(5 * time.Minute)
	go store.garbageCollector(gcTicker)

	imTicker := time.NewTicker(2 * time.Minute)
	go store.imageManager(imTicker)

	ccTicker := time.NewTicker(4 * time.Minute)
	go store.containerCleaner(ccTicker)

	return created, func() {
		imTicker.Stop()
		ccTicker.Stop()
		gcTicker.Stop()
		store.db.Close()
	}
}

type metadataStore struct {
	db *badger.DB
}
