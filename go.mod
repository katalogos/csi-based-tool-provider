module github.com/davidfestal/csi-based-tool-provider

go 1.12

require (
	github.com/container-storage-interface/spec v1.2.0
	github.com/containers/image/v5 v5.9.0
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/google/uuid v1.0.0 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.3.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.0.2-0.20190823105129-775207bd45b6
	github.com/pborman/uuid v0.0.0-20180906182336-adf5a7427709 // indirect
	github.com/spf13/afero v1.2.2 // indirect
	golang.org/x/net v0.0.0-20200324143707-d3edc9973b7e
	google.golang.org/grpc v1.26.0
	k8s.io/apimachinery v0.0.0-20181110190943-2a7c93004028 // indirect
	k8s.io/kubernetes v1.12.2
	k8s.io/utils v0.0.0-20181102055113-1bd4f387aa67 // indirect
)
