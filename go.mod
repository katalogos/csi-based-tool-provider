module github.com/davidfestal/csi-based-tool-provider

go 1.12

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/containers/image/v5 v5.9.0
	github.com/dgraph-io/badger/v3 v3.2011.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/gorilla/mux v1.8.0
	github.com/kubernetes-csi/csi-lib-utils v0.3.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/prometheus/client_golang v1.9.0
	golang.org/x/net v0.0.0-20201021035429-f5854403a974
	google.golang.org/grpc v1.26.0
	k8s.io/apimachinery v0.0.0-20181110190943-2a7c93004028 // indirect
	k8s.io/kubernetes v1.12.2
	k8s.io/utils v0.0.0-20210111153108-fddb29f9d009
)
