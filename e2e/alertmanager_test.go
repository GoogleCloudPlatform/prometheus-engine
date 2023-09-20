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
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

func TestAlertmanagerDefault(t *testing.T) {
	tctx := newOperatorContext(t)

	alertmanagerConfig := `
receivers:
  - name: "foobar"
route:
  receiver: "foobar"
`
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: operator.AlertmanagerPublicSecretName},
		Data: map[string][]byte{
			operator.AlertmanagerPublicSecretKey: []byte(alertmanagerConfig),
		},
	}
	t.Run("deployed", tctx.subtest(testAlertmanagerDeployed(nil)))
	t.Run("config set", tctx.subtest(testAlertmanagerConfig(secret, operator.AlertmanagerPublicSecretKey)))
}

func TestAlertmanagerCustom(t *testing.T) {
	tctx := newOperatorContext(t)

	alertmanagerConfig := `
receivers:
  - name: "foobar"
route:
  receiver: "foobar"
`
	spec := &monitoringv1.ManagedAlertmanagerSpec{
		ConfigSecret: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "my-secret-name",
			},
			Key: "my-secret-key",
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "my-secret-name"},
		Data: map[string][]byte{
			"my-secret-key": []byte(alertmanagerConfig),
		},
	}
	t.Run("deployed", tctx.subtest(testAlertmanagerDeployed(spec)))
	t.Run("config set", tctx.subtest(testAlertmanagerConfig(secret, "my-secret-key")))
}

func testCreateAlertmanagerSecrets(ctx context.Context, t *OperatorContext, cert, key []byte) {
	secrets := []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "alertmanager-authorization",
			},
			Data: map[string][]byte{
				"token": []byte("auth-bearer-password"),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "alertmanager-tls",
			},
			Data: map[string][]byte{
				"cert": cert,
				"key":  key,
			},
		},
	}

	for _, s := range secrets {
		if _, err := t.kubeClient.CoreV1().Secrets(t.pubNamespace).Create(ctx, s, metav1.CreateOptions{}); err != nil {
			t.Fatalf("create alertmanager secret: %s", err)
		}
	}
}

func testAlertmanagerDeployed(spec *monitoringv1.ManagedAlertmanagerSpec) func(context.Context, *OperatorContext) {
	return func(ctx context.Context, t *OperatorContext) {
		opCfg := &monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name: operator.NameOperatorConfig,
			},
			Collection: monitoringv1.CollectionSpec{
				ExternalLabels: map[string]string{
					"external_key": "external_val",
				},
				Filter: monitoringv1.ExportFilters{
					MatchOneOf: []string{
						"{job='foo'}",
						"{__name__=~'up'}",
					},
				},
				KubeletScraping: &monitoringv1.KubeletScraping{
					Interval: "5s",
				},
			},
			ManagedAlertmanager: spec,
		}
		if gcpServiceAccount != "" {
			opCfg.Collection.Credentials = &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "user-gcp-service-account",
				},
				Key: "key.json",
			}
		}
		_, err := t.operatorClient.MonitoringV1().OperatorConfigs(t.pubNamespace).Create(ctx, opCfg, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create rules operatorconfig: %s", err)
		}

		err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
			ss, err := t.kubeClient.AppsV1().StatefulSets(t.namespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				t.Log(fmt.Errorf("getting alertmanager StatefulSet failed: %w", err))
				return false, fmt.Errorf("getting alertmanager StatefulSet failed: %w", err)
			}

			// Assert we have the expected annotations.
			wantedAnnotations := map[string]string{
				"components.gke.io/component-name":               "managed_prometheus",
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
			}
			if diff := cmp.Diff(wantedAnnotations, ss.Spec.Template.Annotations); diff != "" {
				return false, fmt.Errorf("unexpected annotations (-want, +got): %s", diff)
			}

			return true, nil
		})
		if err != nil {
			t.Errorf("unable to get alertmanager statefulset: %s", err)
		}
	}
}

func testAlertmanagerConfig(pub *corev1.Secret, key string) func(context.Context, *OperatorContext) {
	return func(ctx context.Context, t *OperatorContext) {
		_, err := t.kubeClient.CoreV1().Secrets(t.pubNamespace).Create(ctx, pub, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("unable to create alertmanager config secret: %s", err)
		}

		err = wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
			secret, err := t.kubeClient.CoreV1().Secrets(t.namespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				t.Log(fmt.Errorf("getting alertmanager secret failed: %w", err))
				return false, fmt.Errorf("getting alertmanager secret failed: %w", err)
			}

			bytes, ok := secret.Data["config.yaml"]
			if !ok {
				t.Log(errors.New("getting alertmanager secret data in config.yaml failed"))
				return false, errors.New("getting alertmanager secret data in config.yaml failed")
			}

			// Grab data from public secret and compare.
			data := pub.Data[key]
			if diff := cmp.Diff(data, bytes); diff != "" {
				return false, fmt.Errorf("unexpected configuration (-want, +got): %s", diff)
			}
			return true, nil
		})
		if err != nil {
			t.Errorf("unable to get alertmanager config: %s", err)
		}
	}
}
