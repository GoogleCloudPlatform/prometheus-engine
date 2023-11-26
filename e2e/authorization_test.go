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
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/operatorutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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

func defaultEndpoint(endpoint *monitoringv1.ScrapeEndpoint) {
	endpoint.Port = intstr.FromString(operatorutil.SyntheticAppPortName)
	endpoint.Interval = "5s"
}

// setupAuthTest sets up tests for PodMonitoring and ClusterPodMonitoring for when
// authentication configurations are present and when they are not present.
func setupAuthTest(ctx context.Context, t *OperatorContext, appName string, args []string, podMonitoringNamePrefix string, endpointNoAuth, endpointAuth monitoringv1.ScrapeEndpoint) {
	defaultEndpoint(&endpointAuth)
	defaultEndpoint(&endpointNoAuth)

	deployment, err := operatorutil.SyntheticAppDeploy(ctx, t.Client(), t.namespace, appName, args)
	if err != nil {
		t.Fatal(err)
	}
	if err := kubeutil.WaitForDeploymentReady(ctx, t.Client(), t.namespace, appName); err != nil {
		kubeutil.DeploymentDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, appName)
		t.Fatalf("failed to start app: %s", err)
	}

	t.Run("podmonitoring-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%s-missing-config", podMonitoringNamePrefix),
				Namespace: t.namespace,
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

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, isPodMonitoringTargetCertificateError); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected failure: %s", err)
		}
	}))

	t.Run("clusterpodmonitoring-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: fmt.Sprintf("c%s-failure", podMonitoringNamePrefix),
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

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, isPodMonitoringTargetCertificateError); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected failure: %s", err)
		}
	}))

	t.Run("podmonitoring-success", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("pod%s-success", podMonitoringNamePrefix),
				Namespace: t.namespace,
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointAuth},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			deployment, err := operatorutil.SyntheticAppDeploy(ctx, t.Client(), t.namespace, appName, args)
			if err != nil {
				t.Fatal(err)
			}
			if err := kubeutil.WaitForDeploymentReady(ctx, t.Client(), t.namespace, appName); err != nil {
				kubeutil.DeploymentDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, appName)
				t.Fatalf("failed to start app: %s", err)
			}

			t.Run("podmonitoring-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
				t.Parallel()

				pm := &monitoringv1.PodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("pod%s-missing-config", podMonitoringNamePrefix),
						Namespace: t.namespace,
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

				if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
					kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
					t.Fatalf("collector not ready: %s", err)
				}

				if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, isPodMonitoringTargetCertificateError); err != nil {
					kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
					t.Fatalf("scrape endpoint expected failure: %s", err)
				}
			}))

			t.Run("clusterpodmonitoring-missing-config", t.subtest(func(ctx context.Context, t *OperatorContext) {
				t.Parallel()

				pm := &monitoringv1.ClusterPodMonitoring{
					ObjectMeta: metav1.ObjectMeta{
						Name: fmt.Sprintf("c%s-failure", podMonitoringNamePrefix),
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

				if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
					kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
					t.Fatalf("collector not ready: %s", err)
				}

				if err := operatorutil.WaitForPodMonitoringFailure(ctx, t.Client(), pm, isPodMonitoringTargetCertificateError); err != nil {
					kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
					t.Fatalf("scrape endpoint expected failure: %s", err)
				}
			}))
			t.Fatalf("create collector: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Errorf("collector not ready: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringSuccess(ctx, t.Client(), pm); err != nil {
			kubeutil.DaemonSetDebug(t.T, ctx, t.RestConfig(), t.Client(), t.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected success: %s", err)
		}
	}))

	t.Run("clusterpodmonitoring-success", t.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.ClusterPodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("c%s-success", podMonitoringNamePrefix),
				Namespace: t.namespace,
			},
			Spec: monitoringv1.ClusterPodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{endpointAuth},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
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
		})
}
