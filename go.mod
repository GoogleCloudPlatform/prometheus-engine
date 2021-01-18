module github.com/google/gpe-collector

go 1.15

require (
	cloud.google.com/go v0.70.0
	github.com/go-kit/kit v0.10.0
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.2
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/oklog/run v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.7.1
	github.com/prometheus/common v0.14.0
	github.com/prometheus/prometheus v1.8.2-0.20201101200738-7558e9d3c374
	github.com/thanos-io/thanos v0.17.2
	google.golang.org/api v0.33.0
	google.golang.org/genproto v0.0.0-20201019141844-1ed22bb0c154
	google.golang.org/grpc v1.32.0
	google.golang.org/protobuf v1.25.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.3.0
	k8s.io/api v0.20.1
	k8s.io/apimachinery v0.20.1
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.20.1
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.4.0
)

replace (
	// Requiring the version above is not enough, probably because transitive dependencies
	// may be allowed to use an older version without the below lines, which causes
	// lookup errors on vendoring.
	k8s.io/api => k8s.io/api v0.20.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.20.1
	k8s.io/client-go => k8s.io/client-go v0.20.1
	k8s.io/code-generator => k8s.io/code-generator v0.20.1

	// Substitute klog packages so their usage plays nicely with gokit's logger.
	k8s.io/klog => github.com/simonpasquier/klog-gokit v0.3.0
	k8s.io/klog/v2 => github.com/simonpasquier/klog-gokit/v2 v2.0.1
)
