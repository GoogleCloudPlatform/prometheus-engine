module github.com/GoogleCloudPlatform/prometheus-engine

go 1.15

require (
	cloud.google.com/go v0.83.0
	github.com/Azure/azure-sdk-for-go v55.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.19 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.14 // indirect
	github.com/digitalocean/godo v1.62.0 // indirect
	github.com/docker/docker v20.10.7+incompatible // indirect
	github.com/go-kit/kit v0.10.0
	github.com/go-logr/logr v0.4.0
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.6
	github.com/googleapis/gax-go/v2 v2.0.5
	github.com/gophercloud/gophercloud v0.18.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hetznercloud/hcloud-go v1.26.2 // indirect
	github.com/miekg/dns v1.1.42 // indirect
	github.com/oklog/run v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/alertmanager v0.23.0 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.30.0
	// The Prometheus dependency must be hand-crafted as it does not support Go
	// modules versioning. It must not be more recent than the Prometheus version
	// deployed by the operator as otherwise the generated config may contain
	// unrecognized fields (e.g. if 'omitempty' wasn't used properly for new fields).
	//
	// Find the commit of the Prometheus release to use, then:
	//   go get github.com/prometheus/prometheus@${COMMIT_SHA}
	github.com/prometheus/prometheus v1.8.2-0.20210518124745-db7f0bcec27b
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546
	github.com/thanos-io/thanos v0.17.2
	github.com/uber/jaeger-client-go v2.29.1+incompatible // indirect
	go.uber.org/zap v1.17.0
	google.golang.org/api v0.48.0
	google.golang.org/genproto v0.0.0-20210604141403-392c879c8b08
	google.golang.org/grpc v1.38.0
	google.golang.org/protobuf v1.26.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.21.2
	k8s.io/apimachinery v0.21.2
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.21.2
	k8s.io/klog/v2 v2.9.0 // indirect
	sigs.k8s.io/controller-runtime v0.9.2
	sigs.k8s.io/yaml v1.2.0
)

// Dependency resolution fails without adding this override. It's not entirely
// understandable why but this appears to sufficiently fix it.
replace k8s.io/client-go => k8s.io/client-go v0.21.2
