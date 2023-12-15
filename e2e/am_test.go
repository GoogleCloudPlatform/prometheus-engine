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
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

func TestAlertmanagerDefault(t *testing.T) {
	kubeClient, opClient, err := newKubeContexts()
	if err != nil {
		t.Errorf("error instantiating clients. err: %s", err)
	}
	ctx := context.Background()

	/**
		alertmanagerConfig := `
	receivers:
	  - name: "foobar"
	route:
	  receiver: "foobar"
	`
	  **/
	t.Run("alertmanager-deployed", testAlertmanagerDeployed(ctx, t, kubeClient))
	t.Run("alertmanager-configured", testAlertmanagerConfigured(ctx, t, kubeClient, opClient))
	/**
	t.Run("config set", t.subtest(func(ctx context.Context, t *OperatorContext) {
		testAlertmanagerConfig(ctx, t, secret, key)
	}))
	**/
	//testAlertmanager(context.Background(), tctx, nil,
	//	&corev1.Secret{
	//		ObjectMeta: metav1.ObjectMeta{
	//			Name:      operator.AlertmanagerPublicSecretName,
	//			Namespace: tctx.pubNamespace,
	//		},
	//		Data: map[string][]byte{
	//			operator.AlertmanagerPublicSecretKey: []byte(alertmanagerConfig),
	//		},
	//	},
	//	operator.AlertmanagerPublicSecretKey,
	//)
}

/**
func TestAlertmanagerCustom(t *testing.T) {
	kubeClient, opClient, err := newKubeContexts()
	if err != nil {
		t.Errorf("error instantiating clients. err: %s", err)
	}
	ctx := context.Background()

	alertmanagerConfig := `
receivers:
  - name: "foobar"
route:
  receiver: "foobar"
`
	testAlertmanager(context.Background(), tctx,
		&monitoringv1.ManagedAlertmanagerSpec{
			ConfigSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "my-secret-name",
				},
				Key: "my-secret-key",
			},
			ExternalURL: "https://alertmanager.mycompany.com/",
		},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-secret-name",
				Namespace: tctx.pubNamespace,
			},
			Data: map[string][]byte{
				"my-secret-key": []byte(alertmanagerConfig),
			},
		},
		"my-secret-key",
	)
}
**/

/**
func testAlertmanager(ctx context.Context, t *testing.T, spec *monitoringv1.ManagedAlertmanagerSpec, secret *corev1.Secret, key string) {
	t.createOperatorConfigFrom(ctx, monitoringv1.OperatorConfig{
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
	})
	t.Run("deployed", t.subtest(func(ctx context.Context, t *OperatorContext) {
		testAlertmanagerDeployed(ctx, t, spec)
	}))
	t.Run("config set", t.subtest(func(ctx context.Context, t *OperatorContext) {
		testAlertmanagerConfig(ctx, t, secret, key)
	}))
}
**/

func testAlertmanagerDeployed(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		err := wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
			ss, err := kubeClient.AppsV1().StatefulSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting alertmanager StatefulSet failed: %w", err)
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

func testAlertmanagerConfigured(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {

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

		// Update OperatorConfig alertmanager spec with secret info.
		spec := &monitoringv1.ManagedAlertmanagerSpec{
			ConfigSecret: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "my-secret-name",
				},
				Key: "my-secret-key",
			},
		}
		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}
		config.ManagedAlertmanager = spec

		// Update OperatorConfig.
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		err = wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
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
			return true, nil
			/**
			ss, err := kubeClient.AppsV1().StatefulSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameAlertmanager, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting alertmanager StatefulSet failed: %w", err)
			}
			**/
			/**
			const containerName = "alertmanager"
			// TODO(pintohutch): clean-up wantArgs init logic.
			var wantArgs []string
			for _, c := range ss.Spec.Template.Spec.Containers {
				if c.Name != containerName {
					continue
				}
				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				if externalURL := spec.ExternalURL; externalURL != "" {
					wantArgs = append(wantArgs, fmt.Sprintf("--web.external-url=%q", config.ExternalURL))
				}

				if diff := cmp.Diff(strings.Join(wantArgs, " "), getEnvVar(c.Env, "EXTRA_ARGS")); diff != "" {
					err = fmt.Errorf("unexpected flags (-want, +got): %s", diff)
				}
				return err == nil, nil
			}

			return false, fmt.Errorf("no container with name %q found", containerName)
			**/
		})
		if err != nil {
			t.Fatalf("waiting for alertmanager Statefulset failed: %s", err)
		}
	}
}

/**
			// If config spec is empty, no need to assert EXTRA_ARGS.
			if config == nil {
				return true, nil
			}

			const containerName = "alertmanager"
			// TODO(pintohutch): clean-up wantArgs init logic.
			var wantArgs []string
			for _, c := range ss.Spec.Template.Spec.Containers {
				if c.Name != containerName {
					continue
				}
				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				if externalURL := config.ExternalURL; externalURL != "" {
					wantArgs = append(wantArgs, fmt.Sprintf("--web.external-url=%q", config.ExternalURL))
				}

				if diff := cmp.Diff(strings.Join(wantArgs, " "), getEnvVar(c.Env, "EXTRA_ARGS")); diff != "" {
					err = fmt.Errorf("unexpected flags (-want, +got): %s", diff)
				}
				return err == nil, nil
			}

			return false, fmt.Errorf("no container with name %q found", containerName)
		})
		if pollErr != nil {
			if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
				pollErr = err
			}
			t.Errorf("unable to get alertmanager statefulset: %s", pollErr)
		}
	}
}
**/

/**
func testAlertmanagerConfig(ctx context.Context, t *OperatorContext, pub *corev1.Secret, key string) {
	if err := t.Client().Create(ctx, pub); err != nil {
		t.Fatalf("unable to create alertmanager config secret: %s", err)
	}

	var err error
	if pollErr := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
		var secret corev1.Secret
		if err = t.Client().Get(ctx, client.ObjectKey{Namespace: t.namespace, Name: operator.AlertmanagerSecretName}, &secret); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("getting alertmanager secret failed: %w", err)
		}

		bytes, ok := secret.Data["config.yaml"]
		if !ok {
			return false, errors.New("getting alertmanager secret data in config.yaml failed")
		}

		// Grab data from public secret and compare.
		data := pub.Data[key]
		if diff := cmp.Diff(data, bytes); diff != "" {
			err = fmt.Errorf("unexpected configuration (-want, +got): %s", diff)
		}
		return err == nil, nil
	}); pollErr != nil {
		if errors.Is(pollErr, context.DeadlineExceeded) && err != nil {
			pollErr = err
		}
		t.Errorf("unable to get alertmanager config: %s", pollErr)
	}
}
**/
