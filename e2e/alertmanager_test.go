// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	https://www.apache.org/licenses/LICENSE-2.0
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
	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestAlertmanager(t *testing.T) {
	ctx := context.Background()
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("rules-create", testCreateRules(ctx, restConfig, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, monitoringv1.OperatorFeatures{}))

	t.Run("alertmanager-deployed", testAlertmanagerDeployed(ctx, kubeClient))
	t.Run("alertmanager-operatorconfig", testAlertmanagerOperatorConfig(ctx, kubeClient))
}

func testAlertmanagerDeployed(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking alertmanager is running")

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			ss := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.NameAlertmanager,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&ss), &ss); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.AlertmanagerSecretName,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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

func testAlertmanagerOperatorConfig(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking alertmanager is configured")

		// Provision custom Alertmanager secret.
		alertmanagerConfig := `
receivers:
  - name: "foobar"
route:
  receiver: "foobar"
`
		secret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret-name",
				Namespace: operator.DefaultPublicNamespace,
			},
			Data: map[string][]byte{
				"my-secret-key": []byte(alertmanagerConfig),
			},
		}

		if err := kubeClient.Create(ctx, &secret); err != nil {
			t.Fatalf("create alertmanager custom secret: %s", err)
		}

		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
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
		if err := kubeClient.Update(ctx, &config); err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.AlertmanagerSecretName,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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
			ss := appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.NameAlertmanager,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&ss), &ss); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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
				return true, nil
			}

			return false, errors.New("no alertmanager container found")
		})
		if err != nil {
			t.Fatalf("waiting for alertmanager Statefulset failed: %s", err)
		}
	}
}
