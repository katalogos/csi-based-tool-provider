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
	"math"
	"net/http"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var getVolumeMountLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Namespace:   "",
		Subsystem:   "",
		Name:        "grpc_request_volume_mount_duration_seconds",
		Help:        "Latency of NodePublishVolume request in second.",
		ConstLabels: map[string]string{},
		Buckets:     []float64{math.Inf(1)},
	},
	[]string{"status","catalog"},
)

var getVolumeUnmountLatency = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "grpc_request_volume_unmount_duration_seconds",
		Help:    "Latency of NodeUnpublishVolume request in second.",
		Buckets:     []float64{math.Inf(1)},
	},
	[]string{"status"},
)

func InitMetrics() {
	glog.Infof("Registering metrics")
	prometheus.MustRegister(getVolumeMountLatency)
	prometheus.MustRegister(getVolumeUnmountLatency)
	http.Handle("/metrics", promhttp.Handler())
}

