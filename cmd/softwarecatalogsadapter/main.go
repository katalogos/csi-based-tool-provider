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
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/golang/glog"
	"github.com/katalogos/csi-based-tool-provider/pkg/catalogs"
	"github.com/katalogos/csi-based-tool-provider/pkg/common"
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
	configFileName string = "softwarecatalogsadapter.yaml"
	catalogsConfigFile string = "catalogs.json"

	showVersionArg            string = "version"
	configDirectoryArg        string = "config_dir"
	catalogsDirectoryArg      string = "catalogs_dir"
	toolproviderImagesPathArg string = "tool_provider_images_path"
	neverPullArg              string = "never_pull"
	transportToPullFromArg    string = "transport_to_pull_from"
	cleaningIntervalArg       string = "cleaning_interval"
	pullingIntervalArg        string = "pulling_interval"
	retryDelayArg             string = "retry_delay"
)

func main() {
	showVersion := flag.Bool(showVersionArg, false, "Show version")
	configDirectory := flag.String(configDirectoryArg, "/etc/katalogos/config", "Directory where general config file (softwarecatalogsadapter.yaml) will be found")
	flag.String(catalogsDirectoryArg, "/etc/katalogos/mergedcatalogs-config", "Directory where catalogs config file (catalogs.json) will be found")
	catalogsDirectory := func() string { return viper.GetString(catalogsDirectoryArg) }
	flag.String(toolproviderImagesPathArg, "/etc/katalogos/images-config/"+common.ImagesFileName, "Path of the 'images' file used by the toolproviderplug to mount images as CSI volumes")
	toolproviderImagesPath := func() string { return viper.GetString(toolproviderImagesPathArg) }
	flag.Bool(neverPullArg, false, "option to never really pull the image. Pull command is only used to check that the image is already there, and get its image ID")
	neverPull := func() bool { return viper.GetBool(neverPullArg) }
	flag.String(transportToPullFromArg, "docker://", "Transport to pull the container images from (see https://github.com/containers/image/blob/main/docs/containers-transports.5.md). When importing images from a CRI-O runtime, it would have the form: \"containers-storage:[<storage-root>+<run-root>]\"")
	transportToPullFrom := func() string { return viper.GetString(transportToPullFromArg) }
	flag.Duration(cleaningIntervalArg, 5*time.Minute, "Interval between 2 rounds of image cleaning")
	cleaningInterval := func() time.Duration { return viper.GetDuration(cleaningIntervalArg) }
	flag.Duration(pullingIntervalArg, 2*time.Minute, "Interval between 2 rounds of image pulling")
	pullingInterval := func() time.Duration { return viper.GetDuration(pullingIntervalArg) }
	flag.Duration(retryDelayArg, 30*time.Second, "Delay before retrying a failed image pull round")
	retryDelay := func() time.Duration { return viper.GetDuration(retryDelayArg) }

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

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	cleanerTicker := time.NewTicker(cleaningInterval())
	pullerTicker := time.NewTicker(pullingInterval())

	viper.OnConfigChange(func(in fsnotify.Event) {
		common.SyncLogLevelFromViper()
		pullerTicker.Reset(pullingInterval())
		cleanerTicker.Reset(cleaningInterval())
	})

	pullerChannel := make(chan catalogs.PullRound)

	defer func() {
		cleanerTicker.Stop()
		pullerTicker.Stop()
	}()
	
	go func() {
		for {
			select {
			case _, ok := <-pullerTicker.C:
				if !ok {
					glog.Infof("Puller Ticker Finished !")
					return
				}
				pullerChannel <- catalogs.PullRoundTicker
			}
		}
	}()

	adapter := catalogs.NewSoftwareCatalogsAdapter(path.Join(catalogsDirectory(), catalogsConfigFile), toolproviderImagesPath(), neverPull(), transportToPullFrom(), retryDelay)

	catalogsFileViper := viper.New()
	catalogsFileViper.SetConfigFile(adapter.CatalogConfigFile)

	if err := catalogsFileViper.ReadInConfig(); err != nil {
		glog.Fatal(err)
	}
	
	catalogsFileViper.WatchConfig()
	catalogsFileViper.OnConfigChange(func(event fsnotify.Event) {
		glog.Infof("File Event Watcher: %+v", event)
		pullerChannel <- catalogs.PullRoundConfigChanged
	})

	go adapter.ImagePuller(pullerChannel)
	go adapter.ImageCleaner(cleanerTicker.C)

	<-sigs
}
