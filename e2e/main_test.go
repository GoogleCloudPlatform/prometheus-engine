// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package e2e

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"go.uber.org/zap/zapcore"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	metav1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

const (
	// Arbitrary amount of time to let tests exit cleanly before the main process
	// terminates.
	timeoutGracePeriod = 10 * time.Second
)

var (
	projectID, location, cluster string
	fakeGCM                      bool
	pollDuration                 time.Duration

	imageTag          string
	imageRegistryPort int

	gcpServiceAccount string
)

// TestMain injects custom flags to tests.
func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.Level(zapcore.DebugLevel)))

	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&fakeGCM, "fake-metric-service", true, "Use a fake GCM endpoint.")
	flag.DurationVar(&pollDuration, "duration", 3*time.Second, "How often to poll and retry for resources.")

	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")

	flag.StringVar(&imageTag, "image-tag", "", "The tag to use from the local registry.")
	flag.IntVar(&imageRegistryPort, "registry-port", -1, "The port of the local registry.")

	flag.Parse()

	os.Exit(m.Run())
}

func setupCluster(ctx context.Context, t testing.TB) (client.Client, *rest.Config, error) {
	restConfig, err := newRestConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := newKubeClient(restConfig)
	if err != nil {
		return nil, nil, err
	}

	t.Log(">>> deploying static resources")
	if err := createResources(ctx, kubeClient, imageTag, imageRegistryPort); err != nil {
		return nil, nil, err
	}

	t.Log(">>> waiting for operator to be deployed")
	if err := kube.WaitForDeploymentReady(ctx, kubeClient, operator.DefaultOperatorNamespace, operator.NameOperator); err != nil {
		return nil, nil, err
	}
	t.Log(">>> waiting for operator to be ready")
	if err := deploy.WaitForOperatorReady(ctx, kubeClient); err != nil {
		return nil, nil, err
	}
	t.Log(">>> operator started successfully")
	return kubeClient, restConfig, nil
}

func setRESTConfigDefaults(restConfig *rest.Config) error {
	// https://github.com/kubernetes/client-go/issues/657
	// https://github.com/kubernetes/client-go/issues/1159
	// https://github.com/kubernetes/kubectl/blob/6fb6697c77304b7aaf43a520d30cb17563c69886/pkg/cmd/util/kubectl_match_version.go#L115
	defaultGroupVersion := &schema.GroupVersion{Group: "", Version: "v1"}
	if restConfig.GroupVersion == nil {
		restConfig.GroupVersion = defaultGroupVersion
	}
	if restConfig.APIPath == "" {
		restConfig.APIPath = "/api"
	}
	if restConfig.NegotiatedSerializer == nil {
		restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	return rest.SetKubernetesDefaults(restConfig)
}

func newRestConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, nil).ClientConfig()
	if err != nil {
		return nil, err
	}
	if err := setRESTConfigDefaults(restConfig); err != nil {
		return nil, err
	}
	return restConfig, nil
}

func newKubeClient(restConfig *rest.Config) (client.Client, error) {
	scheme, err := newScheme()
	if err != nil {
		return nil, err
	}

	return client.New(restConfig, client.Options{
		Scheme: scheme,
	})
}

func newScheme() (*runtime.Scheme, error) {
	scheme, err := operator.NewScheme()
	if err != nil {
		return nil, err
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}
	return scheme, nil
}

func createResources(ctx context.Context, kubeClient client.Client, imageTag string, imageRegistryPort int) error {
	deployOpts := []deploy.DeployOption{
		deploy.WithMeta(projectID, cluster, location),
	}
	if fakeGCM {
		if imageRegistryPort == -1 {
			return fmt.Errorf("--registry-port must be provided with --fake-metric-service")
		}
		metricServiceImage := fmt.Sprintf("localhost:%d/fake-metric-service:%s", imageRegistryPort, imageTag)
		if err := deploy.CreateFakeMetricService(ctx, kubeClient, metav1.NamespaceDefault, "metric-service", metricServiceImage); err != nil {
			return err
		}
		deployOpts = append(deployOpts, deploy.WithGCMEndpoint(deploy.FakeMetricServiceEndpoint(metav1.NamespaceDefault, "metric-service")))
		deployOpts = append(deployOpts, deploy.WithPrometheusEndpoint("http://"+deploy.FakeMetricCollectorEndpoint(operator.DefaultOperatorNamespace, "metric-collector")))
	}
	if err := deploy.CreateResources(ctx, kubeClient, deployOpts...); err != nil {
		return err
	}
	if fakeGCM {
		// Create once the GMP system namespace exists.
		if err := deploy.CreateFakeMetricCollector(ctx, kubeClient, operator.DefaultOperatorNamespace, "metric-collector", deploy.FakeMetricServiceWebEndpoint(metav1.NamespaceDefault, "metric-service")); err != nil {
			return err
		}
	}

	if gcpServiceAccount == "" {
		gcpServiceAccount, _ = os.LookupEnv("GOOGLE_APPLICATION_CREDENTIALS")
	}
	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return fmt.Errorf("read service account file %q: %w", gcpServiceAccount, err)
		}
		if err := deploy.CreateGCPSecretResources(ctx, kubeClient, metav1.NamespaceDefault, b); err != nil {
			return err
		}
	}
	return nil
}

// contextWithDeadline returns a context that will timeout before t.Deadline().
// See: https://github.com/golang/go/issues/36532
func contextWithDeadline(t *testing.T) context.Context {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return context.Background()
	}

	ctx, cancel := context.WithDeadline(context.Background(), deadline.Truncate(timeoutGracePeriod))
	t.Cleanup(cancel)
	return ctx
}

func newMetricClient(ctx context.Context, t *testing.T, restConfig *rest.Config, kubeClient client.Client) (*gcm.MetricClient, error) {
	clientOpts := []option.ClientOption{
		option.WithUserAgent("prometheus-engine-e2e"),
	}
	if fakeGCM {
		clientOpts = append(clientOpts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithEndpoint(deploy.FakeMetricServiceEndpoint(metav1.NamespaceDefault, "metric-service")),
			option.WithGRPCDialOption(grpc.WithContextDialer(func(ctx context.Context, address string) (net.Conn, error) {
				t.Log("Forwarding address:", address)
				conn, err := kube.PortForward(ctx, restConfig, kubeClient, address,
					writerFn(func(p []byte) (n int, err error) {
						t.Logf("portforward: info: %s", string(p))
						return len(p), nil
					}),
					writerFn(func(p []byte) (n int, err error) {
						t.Logf("portforward: error: %s", string(p))
						return len(p), nil
					}))
				if err != nil {
					t.Error("unable to port-forward:", err)
				}
				return conn, err
			})),
		)
	}
	return gcm.NewMetricClient(ctx, clientOpts...)
}

func newPortForwardClient(t *testing.T, restConfig *rest.Config, kubeClient client.Client) (*http.Client, error) {
	return kube.PortForwardClient(
		restConfig,
		kubeClient,
		writerFn(func(p []byte) (n int, err error) {
			t.Logf("portforward: info: %s", string(p))
			return len(p), nil
		}),
		writerFn(func(p []byte) (n int, err error) {
			t.Logf("portforward: error: %s", string(p))
			return len(p), nil
		}),
	)
}
