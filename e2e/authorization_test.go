// Copyright 2023 Google LLC
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

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

const errCertificate = "x509: certificate signed by unknown authority"
const errUnauthorized = "server returned HTTP status 401 Unauthorized"
const errInvalidClientCredentials = "oauth2: \"invalid_client\" \"incorrect client credentials\""

func TestTLSPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--tls-create-self-signed=true",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "tls",
		&monitoringv1.ScrapeEndpoint{
			Scheme:   "https",
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				TLS: &monitoringv1.TLS{
					InsecureSkipVerify: true,
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Scheme:   "https",
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errCertificate,
	)
}

func TestTLSClusterPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--tls-create-self-signed=true",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "tls",
		&monitoringv1.ScrapeEndpoint{
			Scheme:   "https",
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				TLS: &monitoringv1.TLS{
					InsecureSkipVerify: true,
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Scheme:   "https",
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errCertificate,
	)
}

func TestBasicAuthPodMonitoringNoPassword(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-username=user",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth-no-password",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestBasicAuthClusterPodMonitoringNoPassword(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-username=user",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth-no-password",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestBasicAuthPodMonitoringNoUsername(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "basic-auth-no-username"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-password=pass",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth-no-username",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Password: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestBasicAuthClusterPodMonitoringNoUsername(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "basic-auth-no-username"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-password=pass",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth-no-username",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Password: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
							Namespace: metav1.NamespaceDefault,
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestBasicAuthPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "basic-auth"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-username=user",
		"--basic-auth-password=pass",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
					Password: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
				},
			},
		},
		errUnauthorized,
	)
}

func TestBasicAuthClusterPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "basic-auth"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--basic-auth-username=user",
		"--basic-auth-password=pass",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "basic-auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
					Password: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
							Namespace: metav1.NamespaceDefault,
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: "user",
				},
			},
		},
		errUnauthorized,
	)
}

func TestAuthPodMonitoringNoCredentials(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--auth-scheme=Bearer",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{
					Type: "Bearer",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestAuthClusterPodMonitoringNoCredentials(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--auth-scheme=Bearer",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{
					Type: "Bearer",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestAuthPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "auth"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--auth-scheme=Bearer",
		"--auth-parameters=pass",
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{
					Credentials: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{},
			},
		},
		errUnauthorized,
	)
}

func TestAuthClusterPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	const secretName = "auth"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte("pass")); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		"--auth-scheme=Bearer",
		"--auth-parameters=pass",
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "auth",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{
					Credentials: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
							Namespace: metav1.NamespaceDefault,
						},
					},
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{},
			},
		},
		errUnauthorized,
	)
}

func TestOAuth2PodMonitoringNoSecret(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	var (
		clientID    = "gmp-user-client-id-no-client-secret"
		clientScope = "read"
		accessToken = "abc123"
	)
	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		fmt.Sprintf("--oauth2-client-id=%s", clientID),
		fmt.Sprintf("--oauth2-scopes=%s", clientScope),
		fmt.Sprintf("--oauth2-access-token=%s", accessToken),
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "oauth2-no-client-secret",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestOAuth2ClusterPodMonitoringNoSecret(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	var (
		clientID    = "gmp-user-client-id-no-client-secret"
		clientScope = "read"
		accessToken = "abc123"
	)
	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		fmt.Sprintf("--oauth2-client-id=%s", clientID),
		fmt.Sprintf("--oauth2-scopes=%s", clientScope),
		fmt.Sprintf("--oauth2-access-token=%s", accessToken),
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "oauth2-no-client-secret",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
		},
		errUnauthorized,
	)
}

func TestOAuth2PodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}
	const (
		clientID    = "gmp-user-client-id-no-client-secret"
		clientPass  = "pass"
		clientScope = "read"
		accessToken = "abc123"
	)

	const secretName = "oauth2"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte(clientPass)); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		fmt.Sprintf("--oauth2-client-id=%s", clientID),
		fmt.Sprintf("--oauth2-client-secret=%s", clientPass),
		fmt.Sprintf("--oauth2-scopes=%s", clientScope),
		fmt.Sprintf("--oauth2-access-token=%s", accessToken),
	}))
	authorizationPodMonitoringTest(ctx, t, kubeClient, opClient, "oauth2",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					ClientSecret: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
						},
					},
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		},
		errInvalidClientCredentials,
	)
}

func TestOAuth2ClusterPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}
	const (
		clientID    = "gmp-user-client-id-no-client-secret"
		clientPass  = "pass"
		clientScope = "read"
		accessToken = "abc123"
	)

	const secretName = "oauth2"
	const secretKey = "k1"
	if err := addSecret(ctx, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, secretName, secretKey, []byte(clientPass)); err != nil {
		t.Fatalf("error adding secret: %s", err)
	}

	t.Run("patch-example-app-args", testPatchExampleAppArgs(ctx, kubeClient, []string{
		fmt.Sprintf("--oauth2-client-id=%s", clientID),
		fmt.Sprintf("--oauth2-client-secret=%s", clientPass),
		fmt.Sprintf("--oauth2-scopes=%s", clientScope),
		fmt.Sprintf("--oauth2-access-token=%s", accessToken),
	}))
	authorizationClusterPodMonitoringTest(ctx, t, kubeClient, opClient, "oauth2",
		&monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					ClientSecret: monitoringv1.ClusterSecretSelector{
						Secret: &monitoringv1.ClusterSecretKeySelector{
							SecretKeySelector: monitoringv1.SecretKeySelector{
								Name: secretName,
								Key:  secretKey,
							},
							Namespace: metav1.NamespaceDefault,
						},
					},
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		}, &monitoringv1.ScrapeEndpoint{
			Port:     intstr.FromString("web"),
			Interval: "5s",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: "http://go-synthetic.default.svc.cluster.local:8080/token",
				},
			},
		},
		errInvalidClientCredentials,
	)
}

func authorizationPodMonitoringTest(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface, name string, successConfig, failureConfig *monitoringv1.ScrapeEndpoint, errMsg string) {
	t.Run("collector-deployed", testCollectorDeployed(ctx, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, opClient))

	pm := &monitoringv1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-ready", name),
			Namespace: "default",
		},
		Spec: monitoringv1.PodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "go-synthetic",
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				*successConfig,
			},
		},
	}
	t.Run(fmt.Sprintf("%s-podmon-ready", name), testEnsurePodMonitoringReady(ctx, opClient, pm))

	pmFail := &monitoringv1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-fail", name),
			Namespace: "default",
		},
		Spec: monitoringv1.PodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "go-synthetic",
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				*failureConfig,
			},
		},
	}
	t.Run(fmt.Sprintf("%s-podmon-failure", name), testEnsurePodMonitoringFailure(ctx, opClient, pmFail, errMsg))
}

func authorizationClusterPodMonitoringTest(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface, name string, successConfig, failureConfig *monitoringv1.ScrapeEndpoint, errMsg string) {
	t.Run("collector-deployed", testCollectorDeployed(ctx, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, opClient))

	cpm := &monitoringv1.ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-ready", name),
		},
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "go-synthetic",
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				*successConfig,
			},
		},
	}
	t.Run(fmt.Sprintf("%s-cmon-ready", name), testEnsureClusterPodMonitoringReady(ctx, opClient, cpm))

	cpmFail := &monitoringv1.ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-fail", name),
		},
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "go-synthetic",
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				*failureConfig,
			},
		},
	}
	t.Run(fmt.Sprintf("%s-cmon-failure", name), testEnsureClusterPodMonitoringFailure(ctx, opClient, cpmFail, errMsg))
}

func testPatchExampleAppArgs(ctx context.Context, kubeClient kubernetes.Interface, args []string) func(*testing.T) {
	return func(t *testing.T) {
		scheme, err := newScheme()
		if err != nil {
			t.Errorf("create scheme: %s", err)
		}

		deployment, service, err := deploy.SyntheticAppResources(scheme)
		if err != nil {
			t.Errorf("get synthetic app resources: %s", err)
		}

		container, err := kube.DeploymentContainer(deployment, deploy.SyntheticAppContainerName)
		if err != nil {
			t.Errorf("find synthetic app container: %s", err)
		}
		container.Args = append(container.Args, args...)
		_, err = kubeClient.AppsV1().Deployments("default").Create(ctx, deployment, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("create deployment: %s", err)
		}

		_, err = kubeClient.CoreV1().Services("default").Create(ctx, service, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("create service: %s", err)
		}
	}
}

func isPodMonitoringScrapeEndpointFailure(status *monitoringv1.ScrapeEndpointStatus, errMsg string) error {
	if status.UnhealthyTargets == 0 {
		return errors.New("expected no healthy targets")
	}
	if status.CollectorsFraction == "0" {
		return fmt.Errorf("expected collectors fraction to be 0 but found: %s", status.CollectorsFraction)
	}
	if len(status.SampleGroups) == 0 {
		return errors.New("missing sample groups")
	}
	for i, group := range status.SampleGroups {
		if len(group.SampleTargets) == 0 {
			return fmt.Errorf("missing sample targets for group %d", i)
		}
		for _, target := range group.SampleTargets {
			if target.Health == "up" {
				return fmt.Errorf("healthy target %q at group %d", target.Health, i)
			}
			if target.LastError == nil {
				return fmt.Errorf("missing error for target at group %d", i)
			}
			if !strings.Contains(*target.LastError, errMsg) {
				return fmt.Errorf("expected error message %q at group %d: got %s", errMsg, i, *target.LastError)
			}
		}
	}
	return nil
}

func testEnsurePodMonitoringFailure(ctx context.Context, opClient versioned.Interface, pm *monitoringv1.PodMonitoring, errMsg string) func(*testing.T) {
	return testEnsurePodMonitoringStatus(ctx, opClient, pm,
		func(status *monitoringv1.ScrapeEndpointStatus) error {
			return isPodMonitoringScrapeEndpointFailure(status, errMsg)
		})
}

func testEnsureClusterPodMonitoringFailure(ctx context.Context, opClient versioned.Interface, cpm *monitoringv1.ClusterPodMonitoring, errMsg string) func(*testing.T) {
	return testEnsureClusterPodMonitoringStatus(ctx, opClient, cpm,
		func(status *monitoringv1.ScrapeEndpointStatus) error {
			return isPodMonitoringScrapeEndpointFailure(status, errMsg)
		})
}

func addSecret(ctx context.Context, client kubernetes.Interface, operatorNamespace, namespace, name, key string, data []byte) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: data,
		},
	}
	if _, err := client.CoreV1().Secrets(namespace).Create(ctx, &secret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("unable to create secret: %w", err)
	}
	role := rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:         []string{"get", "list", "watch"},
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: []string{name},
			},
		},
	}
	if _, err := client.RbacV1().Roles(namespace).Create(ctx, &role, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("unable to create secret role: %w", err)
	}
	roleBinding := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		RoleRef: rbacv1.RoleRef{
			Kind: "Role",
			Name: name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      operator.NameCollector,
				Namespace: operatorNamespace,
			},
		},
	}
	if _, err := client.RbacV1().RoleBindings(namespace).Create(ctx, &roleBinding, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("unable to create secret role binding: %w", err)
	}
	return nil
}
