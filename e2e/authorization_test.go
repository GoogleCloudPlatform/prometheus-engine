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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/operatorutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func isPodMonitoringTargetCertificateError(message string) error {
	err := tls.CertificateVerificationError{
		Err: x509.UnknownAuthorityError{},
	}
	expected := err.Error()
	if !strings.HasSuffix(message, expected) {
		return fmt.Errorf("expected %q", expected)
	}
	return nil
}

func isPodMonitoringTargetUnauthorizedError(message string) error {
	const expected = "server returned HTTP status 401 Unauthorized"
	if message != expected {
		return fmt.Errorf("expected %q", expected)
	}
	return nil
}

func isPodMonitoringTargetInvalidOAuthCredentialsError(message string) error {
	const expected = "oauth2: \"invalid_client\" \"incorrect client credentials\""
	if !strings.HasSuffix(message, expected) {
		return fmt.Errorf("expected suffix %q", expected)
	}
	return nil
}

func defaultEndpoint(endpoint *monitoringv1.ScrapeEndpoint) {
	endpoint.Port = intstr.FromString(operatorutil.SyntheticAppPortName)
	endpoint.Interval = "5s"
}

// setupAuthTest sets up tests for PodMonitoring and ClusterPodMonitoring for when
// authentication configurations are present and when they are not present.
func setupAuthTest(ctx context.Context, t *OperatorContext, appName string, args []string, podMonitoringNamePrefix string, endpointNoAuth, endpointAuth monitoringv1.ScrapeEndpoint, expectedFn func(string) error) {
	defaultEndpoint(&endpointAuth)
	defaultEndpoint(&endpointNoAuth)

	deployment, err := operatorutil.SyntheticAppDeploy(ctx, t.Client(), t.userNamespace, appName, args)
	if err != nil {
		t.Fatal(err)
	}
	service := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      appName,
			Namespace: t.userNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: deployment.Spec.Template.Labels,
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromString(operatorutil.SyntheticAppPortName),
				},
			},
		},
	}
	if err := t.Client().Create(ctx, &service); err != nil {
		t.Fatalf("create service: %s", err)
	}

	if err := kubeutil.WaitForDeploymentReady(ctx, t.Client(), t.userNamespace, appName); err != nil {
		kubeutil.DeploymentDebug(t.T, ctx, t.RestConfig(), t.Client(), t.userNamespace, appName)
		t.Fatalf("failed to start app: %s", err)
	}
	if _, err := kubeutil.WaitForServiceReady(ctx, t.Client(), t.userNamespace, appName); err != nil {
		t.Fatalf("service %s/%s not ready: %s", t.userNamespace, appName, err)
	}

	t.Run("podmon-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%s-missing-config", podMonitoringNamePrefix),
				Namespace: t.userNamespace,
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointNoAuth},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), t.namespace, pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("PodMonitoring not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, expectedFn); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected failure: %s", err)
		}
	}))

	t.Run("clusterpodmon-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("%s-c%s-failure", t.namespace, podMonitoringNamePrefix),
			},
			Spec: monitoringv1.ClusterPodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointNoAuth},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), t.namespace, pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, expectedFn); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected failure: %s", err)
		}
	}))

	t.Run("podmon-success", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%s-success", podMonitoringNamePrefix),
				Namespace: t.userNamespace,
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointAuth},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), t.namespace, pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Errorf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringSuccess(ctx, t.Client(), pm); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected success: %s", err)
		}
	}))

	t.Run("clusterpodmon-success", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("c%s-success", podMonitoringNamePrefix),
			},
			Spec: monitoringv1.ClusterPodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointAuth},
			},
		}
		results, err := json.Marshal(endpointAuth)
		if err != nil {
			t.Logf("Error: %s", err)
		}
		t.Logf("ClusterPodMonitoring: %s\n", string(results))
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), t.namespace, pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Errorf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringSuccess(ctx, t.Client(), pm); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected success: %s", err)
		}
	}))
}

func TestTLS(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)
	ctx := context.Background()

	tctx.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
		Features: monitoringv1.OperatorFeatures{
			TargetStatus: monitoringv1.TargetStatusSpec{
				Enabled: true,
			},
		},
	})

	const appName = "tls-insecure"
	setupAuthTest(ctx, tctx, appName, []string{
		"--tls-create-self-signed=true",
	}, "mon-tls-insecure",
		monitoringv1.ScrapeEndpoint{
			Scheme: "https",
		}, monitoringv1.ScrapeEndpoint{
			Scheme: "https",
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				TLS: &monitoringv1.TLS{
					InsecureSkipVerify: true,
				},
			},
		}, isPodMonitoringTargetCertificateError)
}

func TestBasicAuth(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)
	ctx := context.Background()

	tctx.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
		Features: monitoringv1.OperatorFeatures{
			TargetStatus: monitoringv1.TargetStatusSpec{
				Enabled: true,
			},
		},
	})

	t.Run("no-password", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "basic-auth-no-password"
		const appUsername = "gmp-user-basic-auth-no-password"
		setupAuthTest(ctx, t, appName, []string{
			fmt.Sprintf("--basic-auth-username=%s", appUsername),
		}, "mon-basic-auth-no-password", monitoringv1.ScrapeEndpoint{}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: appUsername,
				},
			},
		}, isPodMonitoringTargetUnauthorizedError)
	}))

	t.Run("no-username", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "basic-auth-no-username"
		const appPassword = "secret-no-username"
		const secretKey = "k1"

		if err := addSecret(ctx, t.Client(), t.namespace, t.userNamespace, appName, secretKey, []byte(appPassword)); err != nil {
			t.Fatalf("unable to add secret: %s", err)
		}

		setupAuthTest(ctx, t, appName, []string{
			fmt.Sprintf("--basic-auth-password=%s", appPassword),
		}, "mon-basic-auth-no-username", monitoringv1.ScrapeEndpoint{}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					PasswordSecret: &monitoringv1.ClusterSecretKeySelector{
						Name:      appName,
						Key:       secretKey,
						Namespace: t.userNamespace,
					},
				},
			},
		}, isPodMonitoringTargetUnauthorizedError)
	}))

	t.Run("full", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "basic-auth-full"
		const appUsername = "gmp-user-basic-auth-full"
		const appPassword = "secret-full"
		const secretKey = "k1"

		if err := addSecret(ctx, t.Client(), t.namespace, t.userNamespace, appName, secretKey, []byte(appPassword)); err != nil {
			t.Fatalf("unable to add secret: %s", err)
		}

		setupAuthTest(ctx, t, appName, []string{
			fmt.Sprintf("--basic-auth-username=%s", appUsername),
			fmt.Sprintf("--basic-auth-password=%s", appPassword),
		}, "mon-basic-auth-full", monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: appUsername,
				},
			},
		}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				BasicAuth: &monitoringv1.BasicAuth{
					Username: appUsername,
					PasswordSecret: &monitoringv1.ClusterSecretKeySelector{
						Name:      appName,
						Key:       secretKey,
						Namespace: t.userNamespace,
					},
				},
			},
		}, isPodMonitoringTargetUnauthorizedError)
	}))
}

func TestAuthorization(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)
	ctx := context.Background()

	tctx.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
		Features: monitoringv1.OperatorFeatures{
			TargetStatus: monitoringv1.TargetStatusSpec{
				Enabled: true,
			},
		},
	})

	t.Run("no-credentials", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "auth-no-credentials"
		setupAuthTest(ctx, t, appName, []string{
			"--auth-scheme=Bearer",
		}, "mon-auth-no-credentials", monitoringv1.ScrapeEndpoint{},
			monitoringv1.ScrapeEndpoint{
				HTTPClientConfig: monitoringv1.HTTPClientConfig{
					Authorization: &monitoringv1.Auth{
						Type: "Bearer",
					},
				},
			}, isPodMonitoringTargetUnauthorizedError)
	}))

	t.Run("credentials", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "auth-credentials"
		const appCredentials = "gmp-token-abc123"
		const secretKey = "k1"

		if err := addSecret(ctx, t.Client(), t.namespace, t.userNamespace, appName, secretKey, []byte(appCredentials)); err != nil {
			t.Fatalf("unable to add secret: %s", err)
		}

		setupAuthTest(ctx, t, appName, []string{
			"--auth-scheme=Bearer",
			fmt.Sprintf("--auth-parameters=%s", appCredentials),
		}, "mon-auth-credentials", monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{},
			},
		}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				Authorization: &monitoringv1.Auth{
					CredentialsSecret: &monitoringv1.ClusterSecretKeySelector{
						Name:      appName,
						Key:       secretKey,
						Namespace: t.userNamespace,
					},
				},
			},
		}, isPodMonitoringTargetUnauthorizedError)
	}))
}

func TestOAuth2(t *testing.T) {
	t.Parallel()
	tctx := newOperatorContext(t)
	ctx := context.Background()

	tctx.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
		Features: monitoringv1.OperatorFeatures{
			TargetStatus: monitoringv1.TargetStatusSpec{
				Enabled: true,
			},
		},
	})

	t.Run("no-client-secret", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "oauth2-no-client-secret"
		const clientID = "gmp-user-client-id-no-client-secret"
		const clientScope = "read"
		const accessToken = "abc123"

		setupAuthTest(ctx, t, appName, []string{
			fmt.Sprintf("--oauth2-client-id=%s", clientID),
			fmt.Sprintf("--oauth2-scopes=%s", clientScope),
			fmt.Sprintf("--oauth2-access-token=%s", accessToken),
		}, "mon-oauth2-no-client-secret", monitoringv1.ScrapeEndpoint{}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/token", appName, t.userNamespace),
				},
			},
		}, isPodMonitoringTargetUnauthorizedError)
	}))

	t.Run("client-secret", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()
		const appName = "oauth2-client-secret"
		const clientID = "gmp-user-client-id-client-secret"
		const clientSecret = "secret-client-secret"
		const clientScope = "read"
		const accessToken = "321xyz"
		const secretKey = "k1"

		if err := addSecret(ctx, t.Client(), t.namespace, t.userNamespace, appName, secretKey, []byte(clientSecret)); err != nil {
			t.Fatalf("unable to add secret: %s", err)
		}

		setupAuthTest(ctx, t, appName, []string{
			fmt.Sprintf("--oauth2-client-id=%s", clientID),
			fmt.Sprintf("--oauth2-client-secret=%s", clientSecret),
			fmt.Sprintf("--oauth2-scopes=%s", clientScope),
			fmt.Sprintf("--oauth2-access-token=%s", accessToken),
		}, "mon-oauth2-client-secret", monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					Scopes:   []string{clientScope},
					TokenURL: fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/token", appName, t.userNamespace),
				},
			},
		}, monitoringv1.ScrapeEndpoint{
			HTTPClientConfig: monitoringv1.HTTPClientConfig{
				OAuth2: &monitoringv1.OAuth2{
					ClientID: clientID,
					ClientSecret: &monitoringv1.ClusterSecretKeySelector{
						Name:      appName,
						Key:       secretKey,
						Namespace: t.userNamespace,
					},
					Scopes:   []string{clientScope},
					TokenURL: fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/token", appName, t.userNamespace),
				},
			},
		}, isPodMonitoringTargetInvalidOAuthCredentialsError)
	}))
}

func addSecret(ctx context.Context, client client.Client, operatorNamespace, namespace, name, key string, data []byte) error {
	secret := corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			key: data,
		},
	}
	if err := client.Create(ctx, &secret); err != nil {
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
	if err := client.Create(ctx, &role); err != nil {
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
	if err := client.Create(ctx, &roleBinding); err != nil {
		return fmt.Errorf("unable to create secret role binding: %w", err)
	}
	return nil
}
