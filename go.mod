module github.com/katalogos/csi-based-tool-provider

go 1.12

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/containers/image/v5 v5.14.0
	github.com/dgraph-io/badger/v3 v3.2011.0
	github.com/fsnotify/fsnotify v1.4.9
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/renameio v0.1.0
	github.com/gorilla/mux v1.8.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/kubernetes-csi/csi-lib-utils v0.3.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/prometheus/client_golang v1.9.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.4.0
	google.golang.org/grpc v1.38.0
	k8s.io/api v0.21.3
	k8s.io/mount-utils v0.21.3
	k8s.io/utils v0.0.0-20210709001253-0e1f9d693477
)
