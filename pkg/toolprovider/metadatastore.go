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
	"context"
	"fmt"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	badger "github.com/dgraph-io/badger/v3"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
	"github.com/katalogos/csi-based-tool-provider/pkg/common"
)

const (
	imageTagPrefix         = "imageTag."
	containerImagePrefix   = "containerImage."
	containerIDSuffix      = ".containerId"
	containerPrefix        = "container."
	mountPathSuffix        = ".mountPath"
	catalogSuffix          = ".catalog"
	volumePrefix           = "volume."
	volumeMidFix           = ".volume."
	containerToCleanPrefix = "containerToClean."

	metadataStorePath = "/var/run/toolprovider-metadata-store"
)

type keyNames struct {
}

func (keyNames) containerForImage(image string) []byte {
	return []byte(imageTagPrefix + image + containerIDSuffix)
}

func (keyNames) mountPathForContainer(containerID string) []byte {
	return []byte(containerPrefix + containerID + mountPathSuffix)
}

func (keyNames) catalogForContainer(containerID string) []byte {
	return []byte(containerPrefix + containerID + catalogSuffix)
}

func (keyNames) imageIDForContainer(containerID string) []byte {
	return []byte(containerImagePrefix + containerID)
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

var keys = keyNames{}

func (store *metadataStore) selectContainerForVolume(volumeID, image string) (containerID, catalog, mountPath string, _ error) {
	containerID = ""
	mountPath = ""
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
		item, err = txn.Get(keys.mountPathForContainer(containerID))
		if err != nil {
			return err
		}
		if err = item.Value(func(mountPathBytes []byte) error {
			mountPath = string(mountPathBytes)
			return nil
		}); err != nil {
			return err
		}
		item, err = txn.Get(keys.catalogForContainer(containerID))
		if err != nil {
			return err
		}
		if err = item.Value(func(catalogBytes []byte) error {
			catalog = string(catalogBytes)
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

	return containerID, catalog, mountPath, err
}

func (store *metadataStore) deleteImagesMissingFromCatalog(ctx context.Context, images map[string]imageDescription) (errors []error) {
	imageSet := map[string]int{}
	for image := range images {
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
			var containerID string
			item, err := txn.Get(keyToDelete)
			if err != nil {
				errors = append(errors, err)
				continue
			}
			if err = item.Value(func(containerIDBytes []byte) error {
				containerID = string(containerIDBytes)
				return nil
			}); err != nil {
				errors = append(errors, err)
				continue
			}
	
			if err := txn.Set(keys.containerToClean(containerID), []byte{}); err != nil {
				errors = append(errors, err)
				continue
			}

			if err := txn.Delete(keyToDelete); err != nil {
				errors = append(errors, err)
			}
		}
		return nil
	})
	return errors
}

func (store *metadataStore) updateImage(
	ctx context.Context,
	image string,
	newImageID string,
	catalog string,
	getImageIDFromContainer func(ctx context.Context, containerID string) (string, error),
	createContainer func(ctx context.Context, image, newImageID string) (string, error),
	isPathMounted func(mountPath string) bool,
	mountContainer func(ctx context.Context, containerID string) (string, error),
	deleteContainer func(ctx context.Context, containerID string) error,
) error {
	logger := common.ContextLogger(ctx)
	return store.db.Update(func(txn *badger.Txn) error {
		containerID := ""
		currentImageID := ""
		item, err := txn.Get(keys.containerForImage(image))
		if err != nil && err != badger.ErrKeyNotFound {
			return err
		}
		if err == nil {
			containerIDBytes, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			containerID = string(containerIDBytes)
			currentImageID, err = getImageIDFromContainer(ctx, containerID)
			if err != nil {
				return err
			}
		}

		if newImageID == currentImageID {
			// The image ID is the same and it already has a container created
			// However if the node was just restarted, it might be that the container
			// is not mounted properly anymore. Since it doesn't hurt to mount a
			// container several times through buildah, let's mount it anyway.
			containerIsUsable := true
			item, err = txn.Get(keys.mountPathForContainer(containerID))
			if err == nil {
				storedMountPath, err := item.ValueCopy(nil)
				if err != nil {
					logger.Warningf("The mount path for container %s cannot be retrieved: %v", containerID, err)
					containerIsUsable = false
				} else {
					if !isPathMounted(string(storedMountPath)) {
						containerMountPath, err := mountContainer(ctx, containerID)
						if err != nil {
							logger.Warningf("The already-existing container %s cannot be mounted: %s", containerID, err)
							containerIsUsable = false
						}
						if containerMountPath != string(storedMountPath) {
							logger.Warningf("The existing mount path (%s) doesn't match the stored mount path (%s) for container %s", containerMountPath, storedMountPath, containerID)
							containerIsUsable = false
						}
					}
				}
			} else {
				logger.Warningf("The mount path should be known for container %s: %v", containerID, err)
				containerIsUsable = false
			}

			if containerIsUsable {
				return nil
			}
		}

		if containerID != "" {
			err := txn.Set(keys.containerToClean(containerID), []byte{})
			if err != nil {
				return err
			}
		}

		newContainerID, err := createContainer(ctx, image, newImageID)
		if err != nil {
			return err
		}

		mountPath, err := mountContainer(ctx, newContainerID)
		if err != nil {
			return err
		}

		err = txn.Set(keys.containerForImage(image), []byte(newContainerID))
		if err != nil {
			deleteContainer(ctx, newContainerID)
			return err
		}
		err = txn.Set(keys.mountPathForContainer(newContainerID), []byte(mountPath))
		if err != nil {
			deleteContainer(ctx, newContainerID)
			return err
		}
		err = txn.Set(keys.catalogForContainer(newContainerID), []byte(catalog))
		if err != nil {
			deleteContainer(ctx, newContainerID)
			return err
		}		
		err = txn.Set(keys.imageIDForContainer(newContainerID), []byte(newImageID))
		if err != nil {
			deleteContainer(ctx, newContainerID)
			return err
		}

		return nil
	})
}

func (store *metadataStore) getContainersToDelete(ctx context.Context) ([]string, error) {
	logger := common.ContextLogger(ctx)
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
					break
				}
			}()
			if !containerStillHasMountedVolumes {
				containersToDelete = append(containersToDelete, containerID)
			}
		}
		for _, containerToDelete := range containersToDelete {
			if err := txn.Delete(keys.mountPathForContainer(containerToDelete)); err != nil {
				logger.Warningf("Error cleaning container %s: %v", containerToDelete, err)
			}
			if err := txn.Delete(keys.catalogForContainer(containerToDelete)); err != nil {
				logger.Warningf("Error cleaning container %s: %v", containerToDelete, err)
			}
			if err := txn.Delete(keys.containerToClean(containerToDelete)); err != nil {
				logger.Warningf("Error cleaning container %s: %v", containerToDelete, err)
			}
			if err := txn.Delete(keys.imageIDForContainer(containerToDelete)); err != nil {
				logger.Warningf("Error cleaning container %s: %v", containerToDelete, err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return containersToDelete, nil
}

func (store *metadataStore) dropVolumeContainerOnMountError(ctx context.Context, containerID, volumeID string) error {
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

func (store *metadataStore) dropVolumeContainerOnUnmount(ctx context.Context, volumeID string) error {
	return store.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(keys.containerForVolume(volumeID))
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return err
		}

		err = item.Value(func(containerIDBytes []byte) error {
			containerID := string(containerIDBytes)
			if err = txn.Delete(keys.containerForVolume(volumeID)); err != nil {
				if err != nil && err != badger.ErrKeyNotFound {
					return err
				}
			}
			if err = txn.Delete(keys.volumeOnContainer(volumeID, containerID)); err != nil {
				if err != nil && err != badger.ErrKeyNotFound {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		return nil
	})
}

func (store *metadataStore) garbageCollector(channel <-chan time.Time) {
	for range channel {
	again:
		err := store.db.RunValueLogGC(0.7)
		if err == nil {
			goto again
		}
	}
}

func (store *metadataStore) imageManager(channel <-chan time.Time, imagesConfigFile string) {
	ctx := common.WithLogger(common.BuildLogger("ImageManager", common.LevelImageManager))
	updateImages(ctx, store, imagesConfigFile)
	for range channel {
		updateImages(ctx, store, imagesConfigFile)
	}
}

func (store *metadataStore) containerCleaner(channel <-chan time.Time) {
	ctx := common.WithLogger(common.BuildLogger("ContainerCleaner", common.LevelContainerCleaner))
	for range channel {
		cleanContainers(ctx, store)
	}
}

type storeLogger struct {
	common.Logger
}

func (l *storeLogger) Debugf(format string, args ...interface{}) {
	l.LeveledInfof(common.LevelBadgerDebug, format, args)
}

func (store *metadataStore) init(imagesDirectory string, imChannel, gcChannel, ccChannel <-chan time.Time) (createdStore bool, startBackgroudTasks func(), cleanup func()) {
	glog.Infof("Creating metadata store if necessary")
	created := false
	if _, err := os.Stat(path.Join(metadataStorePath, badger.ManifestFilename)); err != nil && os.IsNotExist(err) {
		created = true
	}

	var err error = nil
	options := badger.DefaultOptions(metadataStorePath)
	options.Logger = &storeLogger{
		Logger: common.BuildLogger("Badger", common.LevelBadger),
	}
	store.db, err = badger.Open(options)
	if err != nil {
		glog.Fatalf("Failed to create metadata store: %v", err)
	}

	glog.Infof("Registering Metadata Store Reader")
	r := mux.NewRouter()
	s := r.Host("localhost").PathPrefix("/metadata").Subrouter()
	iterate := func(prefix []byte) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			store.db.View(func(txn *badger.Txn) error {
				it := txn.NewIterator(badger.DefaultIteratorOptions)
				defer it.Close()
				for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
					item := it.Item()
					value, err := item.ValueCopy(nil)
					if err != nil {
						w.WriteHeader(http.StatusInternalServerError)
						fmt.Fprintf(w, "%v", err)
						return err
					}
					fmt.Fprintf(w, "%s = %s\n", string(item.Key()), string(value))
				}
				return nil
			})
		}
	}
	s.Methods("GET").Path("/iterate/{prefix}").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		prefix := []byte(vars["prefix"])
		iterate(prefix)(rw, r)
	})
	s.Methods("GET").Path("/iterate").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		iterate([]byte(""))(rw, r)
	})
	s.Methods("DELETE").Path("/item/{key}").HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/plain")
		vars := mux.Vars(r)
		if err := store.db.Update(func(txn *badger.Txn) error {
			return txn.Delete([]byte(vars["key"]))
		}); err != nil {
			fmt.Fprintf(rw, "ERROR: %v\n", err)
			rw.WriteHeader(http.StatusInternalServerError)
			return
		}
		rw.WriteHeader(http.StatusOK)
	})
	http.Handle("/", r)

	return created,
		func() {
			go store.garbageCollector(gcChannel)
			go store.imageManager(imChannel, imagesDirectory)
			go store.containerCleaner(ccChannel)
		},
		func() {
			store.db.Close()
		}
}

type metadataStore struct {
	db *badger.DB
}
