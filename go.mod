module github.com/GoogleCloudPlatform/prometheus-engine

go 1.16

require (
	cloud.google.com/go/compute v1.5.0
	cloud.google.com/go/monitoring v1.5.0
	github.com/go-kit/log v0.2.0
	github.com/go-logr/logr v1.2.3
	github.com/golang/protobuf v1.5.2
	github.com/google/go-cmp v0.5.7
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510
	github.com/googleapis/gax-go/v2 v2.3.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/oklog/run v1.1.0
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.12.1
	github.com/prometheus/common v0.34.0
	github.com/prometheus/common/assets v0.1.0
	github.com/prometheus/prometheus v1.8.2-0.20211119115433-692a54649ed7
	github.com/thanos-io/thanos v0.25.2
	go.uber.org/zap v1.19.1
	golang.org/x/oauth2 v0.0.0-20220309155454-6242fa91716a
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65
	google.golang.org/api v0.74.0
	google.golang.org/genproto v0.0.0-20220413183235-5e96e2839df9
	google.golang.org/grpc v1.45.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/alecthomas/kingpin.v2 v2.2.6
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b
	k8s.io/api v0.24.2
	k8s.io/apimachinery v0.24.2
	k8s.io/client-go v0.24.2
	k8s.io/code-generator v0.24.2
	sigs.k8s.io/controller-runtime v0.12.3
	sigs.k8s.io/yaml v1.3.0
)

// Exclude pre-go-mod kubernetes tags, as they are older
// than v0.x releases but are picked when we update the dependencies.
exclude (
	k8s.io/client-go v1.4.0
	k8s.io/client-go v1.4.0+incompatible
	k8s.io/client-go v1.5.0
	k8s.io/client-go v1.5.0+incompatible
	k8s.io/client-go v1.5.1
	k8s.io/client-go v1.5.1+incompatible
	k8s.io/client-go v10.0.0+incompatible
	k8s.io/client-go v11.0.0+incompatible
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/client-go v2.0.0+incompatible
	k8s.io/client-go v2.0.0-alpha.1+incompatible
	k8s.io/client-go v3.0.0+incompatible
	k8s.io/client-go v3.0.0-beta.0+incompatible
	k8s.io/client-go v4.0.0+incompatible
	k8s.io/client-go v4.0.0-beta.0+incompatible
	k8s.io/client-go v5.0.0+incompatible
	k8s.io/client-go v5.0.1+incompatible
	k8s.io/client-go v6.0.0+incompatible
	k8s.io/client-go v7.0.0+incompatible
	k8s.io/client-go v8.0.0+incompatible
	k8s.io/client-go v9.0.0+incompatible
	k8s.io/client-go v9.0.0-invalid+incompatible
)

// Go modules keeps resetting the required v0.35.0 to a random unversioned commit.
// So this is required for unknown reasons.
replace github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.35.0
