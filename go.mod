module github.com/GoogleCloudPlatform/prometheus-engine

go 1.26.0

require (
	cloud.google.com/go/compute/metadata v0.9.0
	cloud.google.com/go/monitoring v1.24.3
	github.com/alecthomas/kingpin/v2 v2.4.0
	github.com/go-kit/log v0.2.1
	github.com/go-logr/logr v1.4.3
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-cmp v0.7.0
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/googleapis/gax-go/v2 v2.17.0 // indirect
	github.com/grafana/grafana-api-golang-client v0.27.0
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/hashicorp/go-cleanhttp v0.5.2
	github.com/oklog/run v1.2.0
	github.com/oklog/ulid v1.3.1 // indirect
	github.com/prometheus/alertmanager v0.30.0
	github.com/prometheus/client_golang v1.23.2
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.67.5
	github.com/prometheus/common/assets v0.2.0
	github.com/prometheus/prometheus v0.311.3 // v2.53.5.
	github.com/stretchr/testify v1.11.1
	github.com/thanos-io/thanos v0.42.5-0.20260916083440-531d98aa0f99
	go.uber.org/zap v1.27.1
	golang.org/x/mod v0.40.0
	golang.org/x/oauth2 v0.36.0
	golang.org/x/time v0.14.0 // indirect
	google.golang.org/api v0.264.0
	google.golang.org/genproto/googleapis/api v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2
	google.golang.org/protobuf v1.36.11
	gopkg.in/yaml.v2 v2.4.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.32.13
	k8s.io/apiextensions-apiserver v0.32.13
	k8s.io/apimachinery v0.34.3
	k8s.io/autoscaler/vertical-pod-autoscaler v1.2.2
	k8s.io/client-go v0.34.3
	k8s.io/utils v0.0.0-20250820121507-0af2bda4dd1d
	sigs.k8s.io/controller-runtime v0.20.4
)

require (
	github.com/efficientgo/e2e v0.14.1-0.20260204162810-8c75b1e33ef9
	github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring v0.75.0
	k8s.io/apiserver v0.32.13
	sigs.k8s.io/yaml v1.6.0
)

require (
	cel.dev/expr v0.25.2 // indirect
	cloud.google.com/go/auth v0.18.2 // indirect
	cloud.google.com/go/auth/oauth2adapt v0.2.8 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azcore v1.20.0 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/azidentity v1.13.1 // indirect
	github.com/Azure/azure-sdk-for-go/sdk/internal v1.11.2 // indirect
	github.com/AzureAD/microsoft-authentication-library-for-go v1.6.0 // indirect
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/alecthomas/units v0.0.0-20240927000941-0f3dac36c52b // indirect
	github.com/antlr4-go/antlr/v4 v4.13.0 // indirect
	github.com/asaskevich/govalidator v0.0.0-20230301143203-a9d515a09cc2 // indirect
	github.com/aws/aws-sdk-go v1.55.8 // indirect
	github.com/aws/aws-sdk-go-v2 v1.41.0 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.32.6 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.19.6 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.18.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/configsources v1.4.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/endpoints/v2 v2.7.16 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.8.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/accept-encoding v1.13.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.13.16 // indirect
	github.com/aws/aws-sdk-go-v2/service/signin v1.0.4 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.30.8 // indirect
	github.com/aws/aws-sdk-go-v2/service/ssooidc v1.35.12 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.41.5 // indirect
	github.com/aws/smithy-go v1.24.0 // indirect
	github.com/bboreham/go-loser v0.0.0-20230920113527-fcc2c21820a3 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/blang/semver/v4 v4.0.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/containerd/errdefs v1.0.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/davecgh/go-spew v1.1.2-0.20180830191138-d8f796af33cc // indirect
	github.com/dennwc/varint v1.0.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker v28.5.2+incompatible // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/edsrzf/mmap-go v1.2.0 // indirect
	github.com/efficientgo/core v1.0.0-rc.3 // indirect
	github.com/emicklei/go-restful/v3 v3.13.0 // indirect
	github.com/evanphx/json-patch v5.9.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.9.11 // indirect
	github.com/facette/natsort v0.0.0-20181210072756-2cd4dd1e2dcb // indirect
	github.com/felixge/httpsnoop v1.0.4 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/fxamacker/cbor/v2 v2.7.0 // indirect
	github.com/go-logfmt/logfmt v0.6.1 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-logr/zapr v1.3.0 // indirect
	github.com/go-openapi/analysis v0.24.1 // indirect
	github.com/go-openapi/errors v0.22.7 // indirect
	github.com/go-openapi/jsonpointer v0.22.1 // indirect
	github.com/go-openapi/jsonreference v0.21.3 // indirect
	github.com/go-openapi/loads v0.23.2 // indirect
	github.com/go-openapi/spec v0.22.1 // indirect
	github.com/go-openapi/strfmt v0.26.3 // indirect
	github.com/go-openapi/swag v0.25.4 // indirect
	github.com/go-openapi/swag/jsonname v0.25.4 // indirect
	github.com/go-openapi/validate v0.25.1 // indirect
	github.com/go-viper/mapstructure/v2 v2.5.0 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.0 // indirect
	github.com/golang/protobuf v1.5.4 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/google/btree v1.1.3 // indirect
	github.com/google/cel-go v0.22.0 // indirect
	github.com/google/gnostic-models v0.7.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/s2a-go v0.1.9 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.3.11 // indirect
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674 // indirect
	github.com/grafana/regexp v0.0.0-20250905093917-f7b3be9d1853 // indirect
	github.com/hashicorp/go-version v1.8.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/jpillora/backoff v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/julienschmidt/httprouter v1.3.0 // indirect
	github.com/klauspost/compress v1.18.7 // indirect
	github.com/klauspost/cpuid/v2 v2.3.0 // indirect
	github.com/kylelemons/godebug v1.1.0 // indirect
	github.com/mailru/easyjson v0.9.0 // indirect
	github.com/minio/sha256-simd v1.0.1 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/moby/spdystream v0.5.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/mwitkow/go-conntrack v0.0.0-20190716064945-2f068394615f // indirect
	github.com/mxk/go-flowrate v0.0.0-20140419014527-cca7078d478f // indirect
	github.com/oklog/ulid/v2 v2.1.1 // indirect
	github.com/onsi/ginkgo/v2 v2.26.0 // indirect
	github.com/onsi/gomega v1.38.2 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/browser v0.0.0-20240102092130-5ac0b6a4141c // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.1-0.20181226105442-5d4384ee4fb2 // indirect
	github.com/prometheus/common/sigv4 v0.1.0 // indirect
	github.com/prometheus/procfs v0.17.0 // indirect
	github.com/prometheus/sigv4 v0.3.0 // indirect
	github.com/spf13/cobra v1.8.1 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/stoewer/go-strcase v1.3.0 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xhit/go-str2duration/v2 v2.1.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/collector/featuregate v1.48.0 // indirect
	go.opentelemetry.io/collector/pdata v1.48.0 // indirect
	go.opentelemetry.io/collector/semconv v0.128.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.62.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.64.0 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	go.uber.org/goleak v1.3.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	golang.org/x/exp v0.0.0-20250819193227-8b4c13bb791b // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	gomodules.xyz/jsonpatch/v2 v2.5.0 // indirect
	google.golang.org/genproto v0.0.0-20260128011058-8636f8732409 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260615183401-62b3387ff324 // indirect
	gopkg.in/evanphx/json-patch.v4 v4.12.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/component-base v0.32.13 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/kube-openapi v0.0.0-20250701173324-9bd5c66d9911 // indirect
	sigs.k8s.io/json v0.0.0-20250730193827-2d320260d730 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.7.0 // indirect
)

replace (
	// We need replace statements to be able to use e.g. newer Thanos dependency without upgrading
	// fork version.
	// TODO(bwplotka): Remove/reduce statements once we update the fork version.

	// Thanos requires efficientgo/e2e v0.14.1-0.20260204..., which requires prometheus/common >= v0.62.0.
	// Pinned to avoid undefined expfmt.NewTextParser in monitoring/instrumented.go against prometheus/common v0.61.0.
	github.com/efficientgo/e2e => github.com/efficientgo/e2e v0.14.1-0.20230710114240-c316eb95ae5b
	// go-openapi v0.25+ split swag into submodules (swag/loading, swag/jsonutils) which have
	// broken test dependencies on nonexistent packages in github.com/go-openapi/testify/v2,
	// breaking module resolution. Keep analysis, loads, spec, swag, and validate on pre-split versions.
	github.com/go-openapi/analysis => github.com/go-openapi/analysis v0.23.0
	github.com/go-openapi/loads => github.com/go-openapi/loads v0.22.0
	github.com/go-openapi/spec => github.com/go-openapi/spec v0.21.0
	github.com/go-openapi/swag => github.com/go-openapi/swag v0.23.1
	github.com/go-openapi/validate => github.com/go-openapi/validate v0.24.0
	// Thanos requires alertmanager v0.30.0, which requires prometheus/common >= v0.62.0.
	// Pinned to avoid undefined model.LegacyValidation.IsValidLabelName against prometheus/common v0.61.0.
	github.com/prometheus/alertmanager => github.com/prometheus/alertmanager v0.28.1
	// Thanos requires client_golang v1.23.2, which requires prometheus/common >= v0.62.0.
	// Pinned to avoid undefined model.LegacyValidation.IsValidMetricName against prometheus/common v0.61.0.
	github.com/prometheus/client_golang => github.com/prometheus/client_golang v1.23.0
	// Prometheus v2.53.5 depends on the older prometheus/common.
	github.com/prometheus/common => github.com/prometheus/common v0.61.0
	// See go/gmp:fork-toil for rationales of this entry.
	github.com/prometheus/prometheus => github.com/GoogleCloudPlatform/prometheus v0.0.0-20260123105636-10ff5b96bd82 // v2.53.5-gmp.2-rc.0
	// Thanos requires k8s.io/client-go and apimachinery v0.34.3.
	// client-go >= v0.33 changed cache.DefaultWatchErrorHandler to take a context.Context,
	// which breaks the pinned Prometheus v2.53.5 fork. Keeping client-go at v0.32.13 also requires
	// keeping matching v0.32.13 for apimachinery and api to avoid removed APIs and schema type mismatches.
	k8s.io/api => k8s.io/api v0.32.13
	k8s.io/apimachinery => k8s.io/apimachinery v0.32.13
	k8s.io/client-go => k8s.io/client-go v0.32.13
)
