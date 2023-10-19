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
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/operatorutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

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

	c := tctx.Client()
	const appName = "tls-insecure"
	deployment, err := operatorutil.SyntheticAppDeploy(ctx, c, tctx.namespace, appName, []string{
		"--tls-create-self-signed=true",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := kubeutil.WaitForDeploymentReady(ctx, c, tctx.namespace, appName); err != nil {
		kubeutil.DeploymentDebug(tctx.T, ctx, tctx.RestConfig(), tctx.Client(), tctx.namespace, appName)
		t.Fatalf("failed to start app: %s", err)
	}

	tctx.Run("tls-missing-config", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "collector-tls-missing-config",
				Namespace: t.namespace,
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{
					{
						Port:     intstr.FromString(operatorutil.SyntheticAppPortName),
						Scheme:   "https",
						Interval: "5s",
					},
				},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
			kubeutil.DaemonSetDebug(tctx.T, ctx, tctx.RestConfig(), tctx.Client(), tctx.namespace, operator.NameCollector)
			t.Fatalf("collector not ready: %s", err)
		}

		var err error
		if pollErr := wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			if err = t.Client().Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
				return false, nil
			}

			const expected = "tls: failed to verify certificate: x509: certificate signed by unknown authority"
			err = operatorutil.IsPodMonitoringScrapeEndpointFailure(pm, operatorutil.SyntheticAppPortName, func(message string) error {
				if !strings.HasSuffix(message, expected) {
					return fmt.Errorf("expected %q", expected)
				}
				return nil
			})
			return err == nil, nil
		}); pollErr != nil {
			if errors.Is(pollErr, wait.ErrWaitTimeout) {
				pollErr = err
			}
			kubeutil.DaemonSetDebug(tctx.T, ctx, tctx.RestConfig(), tctx.Client(), tctx.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected failure: %s", pollErr)
		}
	}))

	tctx.Run("tls-insecure", tctx.subtest(func(ctx context.Context, t *OperatorContext) {
		t.Parallel()

		pm := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "collector-tls-insecure",
				Namespace: t.namespace,
			},
			Spec: monitoringv1.PodMonitoringSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: deployment.Spec.Template.Labels,
				},
				Endpoints: []monitoringv1.ScrapeEndpoint{
					{
						Port:     intstr.FromString(operatorutil.SyntheticAppPortName),
						Scheme:   "https",
						Interval: "5s",
						HTTPClientConfig: monitoringv1.HTTPClientConfig{
							TLS: &monitoringv1.TLS{
								InsecureSkipVerify: true,
							},
						},
					},
				},
			},
		}
		if err := t.Client().Create(ctx, pm); err != nil {
			t.Fatalf("create collector: %s", err)
		}

		if err := operatorutil.WaitForPodMonitoringReady(ctx, t.Client(), pm, true); err != nil {
			kubeutil.DaemonSetDebug(tctx.T, ctx, tctx.RestConfig(), tctx.Client(), tctx.namespace, operator.NameCollector)
			t.Errorf("collector not ready: %s", err)
		}

		var err error
		if pollErr := wait.Poll(5*time.Second, 3*time.Minute, func() (bool, error) {
			if err = t.Client().Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
				return false, nil
			}
			err = operatorutil.IsPodMonitoringScrapeEndpointSuccess(pm, operatorutil.SyntheticAppPortName)
			return err == nil, nil
		}); pollErr != nil {
			if errors.Is(pollErr, wait.ErrWaitTimeout) {
				pollErr = err
			}
			kubeutil.DaemonSetDebug(tctx.T, ctx, tctx.RestConfig(), tctx.Client(), tctx.namespace, operator.NameCollector)
			t.Fatalf("scrape endpoint expected success: %s", pollErr)
		}
	}))
}
