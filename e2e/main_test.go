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
	"errors"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"go.uber.org/zap/zapcore"
	"google.golang.org/api/option"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
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
	gcmSecretEnv       = "GCM_SECRET"

	gcmExplicitSecretName = "user-gcp-service-account"
	gcmExplicitSecretKey  = "key.json"
)

var (
	projectID, location, cluster string
	skipGCM                      bool
	pollDuration                 time.Duration

	gcpServiceAccount string
)

// TestMain injects custom flags to tests.
func TestMain(m *testing.M) {
	ctrl.SetLogger(zap.New(zap.Level(zapcore.DebugLevel)))

	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.DurationVar(&pollDuration, "duration", 3*time.Second, "How often to poll and retry for resources.")

	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")

	flag.Parse()

	os.Exit(m.Run())
}

func setupCluster(ctx context.Context, t testing.TB, dOpts ...deploy.DeployOption) (client.Client, *rest.Config, error) {
	t.Log(">>> deploying static resources")
	restConfig, err := newRestConfig()
	if err != nil {
		return nil, nil, err
	}

	kubeClient, err := newKubeClient(restConfig)
	if err != nil {
		return nil, nil, err
	}

	dOpts = append(dOpts, deploy.WithMeta(projectID, cluster, location), deploy.WithDisableGCM(skipGCM))
	if explicitCredentialsConfigured() {
		t.Log(">>> setup explicit credentials")
		// Due to https://github.com/GoogleCloudPlatform/prometheus/pull/259/files#r2350691932
		// we have to configure correct credential for fork to use before it applies config.
		// It will crashloop until operator sets up credentials, but eventually it will work.
		// TODO(bwplotka): Remove once we make fork apply config on start correctly.
		dOpts = append(dOpts, deploy.WithExplicitCredentials(collectorExplicitCredentials()))
	}

	if err := deploy.CreateResources(ctx, kubeClient, dOpts...); err != nil {
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

	if explicitCredentialsConfigured() {
		t.Log(">>> configure operator explicit credentials")
		if err := configureOperatorExplicitCredentials(ctx, kubeClient); err != nil {
			return nil, nil, err
		}
	}
	return kubeClient, restConfig, nil
}

// Setup explicit credentials for GCM_SECRET or explicit gcpServiceAccount flag.
// Otherwise, GOOGLE_APPLICATION_CREDENTIALS will use default flow.
func explicitCredentialsConfigured() bool {
	return !skipGCM && (gcpServiceAccount != "" || os.Getenv(gcmSecretEnv) != "")
}

func getExplicitGCMSAJSON() ([]byte, error) {
	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return b, fmt.Errorf("read service account file %q: %w", gcpServiceAccount, err)
		}
		return b, nil
	}
	if gcmSA := os.Getenv(gcmSecretEnv); gcmSA != "" {
		return []byte(gcmSA), nil
	}
	return nil, errors.New("gcp-service-account flag or GCM_SECRET environment variable not set")
}

func createGCMSecret(ctx context.Context, kubeClient client.Client, serviceAccount []byte) error {
	if err := kubeClient.Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gcmExplicitSecretName,
			Namespace: operator.DefaultPublicNamespace,
		},
		Data: map[string][]byte{
			gcmExplicitSecretKey: serviceAccount,
		},
	}); err != nil {
		return fmt.Errorf("create GCM service account secret: %w", err)
	}
	return nil
}

// collectorExplicitCredentials returns the local path for operator injected
// credentials in the local collector filesystem, when using createGCMSecret().
func collectorExplicitCredentials() string {
	return fmt.Sprintf("/etc/secrets/secret_%s_%s_%s", operator.DefaultPublicNamespace, gcmExplicitSecretName, gcmExplicitSecretKey)
}

// newMetricClient returns GCM client. Our e2e tests supports both GOOGLE_APPLICATION_CREDENTIALS (file)
// and GCM_SECRET (content, required with CI use).
func newMetricClient(ctx context.Context) (*gcm.MetricClient, error) {
	if !explicitCredentialsConfigured() {
		return gcm.NewMetricClient(ctx)
	}
	gcmSA, err := getExplicitGCMSAJSON()
	if err != nil {
		return nil, err
	}
	return gcm.NewMetricClient(ctx, option.WithCredentialsJSON(gcmSA))
}

func configureOperatorExplicitCredentials(ctx context.Context, kubeClient client.Client) error {
	gcmSA, err := getExplicitGCMSAJSON()
	if err != nil {
		return err
	}
	// Deploy explicit secret.
	if err := createGCMSecret(ctx, kubeClient, gcmSA); err != nil {
		return err
	}
	// Select credentials via OperatorConfig.

	// Do it in loop as update can fail if starting operator will create an
	// empty OperatorConfig in the same time.
	return wait.PollUntilContextCancel(ctx, 500*time.Millisecond, false, func(ctx context.Context) (bool, error) {
		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			return false, fmt.Errorf("get operatorconfig: %w", err)
		}
		config.Collection.Credentials = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: gcmExplicitSecretName},
			Key:                  gcmExplicitSecretKey,
		}
		config.Rules.Credentials = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: gcmExplicitSecretName},
			Key:                  gcmExplicitSecretKey,
		}
		if err := kubeClient.Update(ctx, &config); err != nil {
			return false, fmt.Errorf("update operatorconfig: %w", err)
		}
		return true, nil
	})
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

// contextWithDeadline returns a context that will timeout before t.Deadline().
// See: https://github.com/golang/go/issues/36532
func contextWithDeadline(t *testing.T) context.Context {
	t.Helper()

	deadline, ok := t.Deadline()
	if !ok {
		return t.Context()
	}

	ctx, cancel := context.WithDeadline(t.Context(), deadline.Truncate(timeoutGracePeriod))
	t.Cleanup(cancel)
	return ctx
}
