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

package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"github.com/katalogos/csi-based-tool-provider/pkg/common"
	"github.com/katalogos/csi-based-tool-provider/pkg/toolprovider"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func init() {
	flag.Set("logtostderr", "true")
}

var (
	// Set by the build process
	version = ""
)

const (
	configFileName string = "toolprovider.yaml"

	endpointArg                 string = "endpoint"
	driverNameArg               string = "drivername"
	nodeIDArg                   string = "nodeid"
	showVersionArg              string = "version"
	configDirectoryArg          string = "config_dir"
	imagesDirectoryArg          string = "images_dir"
	garbageCollectorIntervalArg string = "metadata_garbage_collector_interval"
	imageManagerIntervalArg     string = "metadata_image_management_interval"
	containerCleanerIntervalArg string = "metadata_container_cleaner_interval"
)

func main() {
	endpoint := flag.String(endpointArg, "unix://tmp/csi.sock", "CSI endpoint")
	driverName := flag.String(driverNameArg, "toolprovider.csi.katalogos.dev", "name of the driver")
	nodeID := flag.String(nodeIDArg, "", "node id")
	showVersion := flag.Bool(showVersionArg, false, "Show version.")
	configDirectory := flag.String(configDirectoryArg, "/etc/katalogos/config", "Directory where general config file (toolprovider.yaml) will be found")
	flag.String(imagesDirectoryArg, "/etc/katalogos/images-config", "Directory where images config file (images) will be found")
	imagesDirectory := func() string { return viper.GetString(imagesDirectoryArg) }
	flag.Duration(garbageCollectorIntervalArg, 5*time.Minute, "interval between garbage collection of the metadata Key-Value store")
	garbageCollectorInterval := func() time.Duration { return viper.GetDuration(garbageCollectorIntervalArg) }
	flag.Duration(imageManagerIntervalArg, 2*time.Minute, "maximum interval between 2 rounds of image management")
	imageManagerInterval := func() time.Duration { return viper.GetDuration(imageManagerIntervalArg) }
	flag.Duration(containerCleanerIntervalArg, 4*time.Minute, "interval between 2 rounds of unused container cleaning")
	containerCleanerInterval := func() time.Duration { return viper.GetDuration(containerCleanerIntervalArg) }

	flag.Parse()
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	viper.BindPFlags(pflag.CommandLine)
	viper.SetEnvPrefix("args")
	viper.AutomaticEnv()

	viper.SetConfigFile(path.Join(*configDirectory, configFileName))
	err := viper.ReadInConfig()
	if err != nil { // Handle errors reading the config file
		glog.Errorf("error reading config file: %w", err)
	}	
	viper.WatchConfig()

	common.SyncLogLevelFromViper()

	glog.Infof("Settings: %v", viper.AllSettings())
	glog.Infof("Log level: %s", flag.Lookup("v").Value.String())

	if *showVersion {
		baseName := path.Base(os.Args[0])
		fmt.Println(baseName, version)
		return
	}

	toolprovider.InitMetrics()

	glog.Infof("Creating HTTP server")
	go func() {
		if err := http.ListenAndServe(":8080", nil); err != nil {
			glog.Fatal(err)
		}
	}()

	gcTicker := time.NewTicker(garbageCollectorInterval())
	imTicker := time.NewTicker(imageManagerInterval())
	ccTicker := time.NewTicker(containerCleanerInterval())
	
	viper.OnConfigChange(func(in fsnotify.Event) {
		common.SyncLogLevelFromViper()
		gcTicker.Reset(garbageCollectorInterval())
		imTicker.Reset(imageManagerInterval())
		ccTicker.Reset(containerCleanerInterval())
	})

	imageManagerChannel := make(chan time.Time)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		glog.Fatal(err)
	}

	defer func() {
		gcTicker.Stop()
		imTicker.Stop()
		ccTicker.Stop()
		watcher.Close()
	}()

	go func() {
		for {
			select {
			case t, ok := <-imTicker.C:
				if !ok {
					glog.Infof("ImageManager Ticker Finished !")
					return
				}
				imageManagerChannel <- t
			case event, ok := <-watcher.Events:
				if !ok {
					glog.Infof("File Event Watcher Finished !")
					return
				}
				if event.Op == fsnotify.Chmod {
					continue
				}
				glog.Infof("File Event Watcher: %+v", event)
				imageManagerChannel <- time.Now()

				if event.Op == fsnotify.Remove {
					watcher.Add(event.Name)
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					glog.Infof("File Error Watcher Finished !")
					return
				}
				glog.Error(err)
			}
		}
	}()

	driver, err := toolprovider.NewToolProviderDriver(*driverName, *nodeID, *endpoint, version, imagesDirectory(), imageManagerChannel, gcTicker.C, ccTicker.C)
	if err != nil {
		fmt.Printf("Failed to initialize driver: %s", err.Error())
		os.Exit(1)
	}
	watcher.Add(driver.ImagesConfigFile)

	driver.Run()

	os.Exit(0)
}
