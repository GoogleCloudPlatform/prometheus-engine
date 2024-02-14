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

package e2e

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func TestAlertmanager(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := newKubeClients()
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("rules-create", testCreateRules(ctx, t, opClient))
	t.Run("alertmanager-deployed", testAlertmanagerDeployed(ctx, t, kubeClient))
	t.Run("alertmanager-operatorconfig", testAlertmanagerOperatorConfig(ctx, t, kubeClient, opClient))
}

func testAlertmanagerDeployed(ctx context.Context, _ *testing.T, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking alertmanager is running")

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			ss, err := kubeClient.AppsV1().StatefulSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("get alertmanager: %w", err)
			}

			// Ensure alertmanager pod is ready.
			if ss.Status.ReadyReplicas != 1 {
				return false, nil
			}

			// Assert we have the expected annotations.
			wantedAnnotations := map[string]string{
				"components.gke.io/component-name":               "managed_prometheus",
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
			}
			if diff := cmp.Diff(wantedAnnotations, ss.Spec.Template.Annotations); diff != "" {
				return false, fmt.Errorf("unexpected annotations (-want, +got): %s", diff)
			}

			// Ensure default no-op alertmanager secret has been created by operator.
			secret, err := kubeClient.CoreV1().Secrets(operator.DefaultOperatorNamespace).Get(ctx, operator.AlertmanagerSecretName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting alertmanager StatefulSet failed: %w", err)
			}

			bytes, ok := secret.Data[operator.AlertmanagerConfigKey]
			if !ok {
				return false, errors.New("getting alertmanager secret data failed")
			}

			if diff := cmp.Diff([]byte(operator.AlertmanagerNoOpConfig), bytes); diff != "" {
				return false, fmt.Errorf("unexpected secret payload (-want, +got): %s", diff)
			}

			return true, nil
		})
		if err != nil {
			t.Fatalf("waiting for alertmanager Statefulset failed: %s", err)
		}
	}
}

func testAlertmanagerOperatorConfig(ctx context.Context, _ *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking alertmanager is configured")

		// Provision custom Alertmanager secret.
		alertmanagerConfig := `
receivers:
  - name: "foobar"
route:
  receiver: "foobar"
`
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "my-secret-name",
			},
			Data: map[string][]byte{
				"my-secret-key": []byte(alertmanagerConfig),
			},
		}

		_, err := kubeClient.CoreV1().Secrets(operator.DefaultPublicNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create alertmanager custom secret: %s", err)
		}

		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}
		// Update OperatorConfig alertmanager spec with secret info.
		config.ManagedAlertmanager = &monitoringv1.ManagedAlertmanagerSpec{
			ConfigSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "my-secret-name",
				},
				Key: "my-secret-key",
			},
			ExternalURL: "https://alertmanager.mycompany.com/",
		}

		// Update OperatorConfig.
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			secret, err := kubeClient.CoreV1().Secrets(operator.DefaultOperatorNamespace).Get(ctx, operator.AlertmanagerSecretName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting alertmanager secret failed: %w", err)
			}
			bytes, ok := secret.Data[operator.AlertmanagerConfigKey]
			if !ok {
				return false, errors.New("getting alertmanager secret data failed")
			}

			// Grab data from public secret and compare.
			if diff := cmp.Diff([]byte(alertmanagerConfig), bytes); diff != "" {
				return false, fmt.Errorf("unexpected configuration (-want, +got): %s", diff)
			}

			// Check externalURL was set on statefulset.
			ss, err := kubeClient.AppsV1().StatefulSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting alertmanager StatefulSet failed: %w", err)
			}

			// Ensure alertmanager container has expected args.
			for _, c := range ss.Spec.Template.Spec.Containers {
				if c.Name != operator.AlertmanagerContainerName {
					continue
				}
				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				wantArgs := []string{
					fmt.Sprintf("--web.external-url=%q", "https://alertmanager.mycompany.com/"),
				}

				if diff := cmp.Diff(strings.Join(wantArgs, " "), getEnvVar(c.Env, "EXTRA_ARGS")); diff != "" {
					return false, fmt.Errorf("unexpected flags (-want, +got): %s", diff)
				}
				return err == nil, nil
			}

			return false, errors.New("no alertmanager container found")
		})
		if err != nil {
			t.Fatalf("waiting for alertmanager Statefulset failed: %s", err)
		}
	}
}
