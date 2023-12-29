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
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	gcmpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/cert"
)

func TestRuleEvaluator(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := newKubeClients()
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("rule-evaluator-deployed", testRuleEvaluatorDeployed(ctx, t, kubeClient))
	t.Run("rule-evaluator-configured", testRuleEvaluatorConfigured(ctx, t, kubeClient, opClient))
	// TODO(pintohutch): testing the generated secrets and config can be
	// brittle as the checks need to be precise and could break if mechanics or
	// formatting changes in the future.
	// Ideally this is replaced by a true e2e test that deploys a custom
	// secured alertmanager and successfully sends alerts to it.
	t.Run("rule-evaluator-secrets", testRuleEvaluatorSecrets(ctx, t, kubeClient, opClient))
	t.Run("rule-evaluator-configuration", testRuleEvaluatorConfiguration(ctx, t, kubeClient))

	t.Run("rules-create", testCreateRules(ctx, t, opClient))
	if !skipGCM {
		t.Run("rules-gcm", testValidateRuleEvaluationMetrics(ctx, t))
	}
}

func testRuleEvaluatorDeployed(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking rule-evaluator is running")

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			deploy, err := kubeClient.AppsV1().Deployments(operator.DefaultOperatorNamespace).Get(ctx, operator.NameRuleEvaluator, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting rule-evaluator Deployment failed: %w", err)
			}
			if *deploy.Spec.Replicas != deploy.Status.ReadyReplicas {
				return false, nil
			}

			// Assert we have the expected annotations.
			wantedAnnotations := map[string]string{
				"components.gke.io/component-name":               "managed_prometheus",
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
			}
			if diff := cmp.Diff(wantedAnnotations, deploy.Spec.Template.Annotations); diff != "" {
				return false, fmt.Errorf("unexpected annotations (-want, +got): %s", diff)
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("failed waiting for generated rule-evaluator deployment: %s", err)
		}
	}
}

func testRuleEvaluatorConfigured(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking rule-evaluator is configured")

		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}

		// Configure
		config.Rules.ExternalLabels = map[string]string{
			"external_key": "external_val"}
		config.Rules.GeneratorURL = "http://example.com/"

		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}
		// Keep checking the state of the collectors until they're running.
		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			deploy, err := kubeClient.AppsV1().Deployments(operator.DefaultOperatorNamespace).Get(ctx, operator.NameRuleEvaluator, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting collector DaemonSet failed: %w", err)
			}

			// Ensure evaluator container has expected args.
			for _, c := range deploy.Spec.Template.Spec.Containers {
				if c.Name != operator.RuleEvaluatorContainerName {
					continue
				}
				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				wantArgs := []string{
					fmt.Sprintf("--export.label.project-id=%q", projectID),
					fmt.Sprintf("--export.label.location=%q", location),
					fmt.Sprintf("--export.label.cluster=%q", cluster),
					fmt.Sprintf("--query.project-id=%q", projectID),
					fmt.Sprintf("--query.generator-url=%q", "http://example.com/"),
				}
				gotArgs := getEnvVar(c.Env, "EXTRA_ARGS")
				for _, arg := range wantArgs {
					if !strings.Contains(gotArgs, arg) {
						return false, fmt.Errorf("expected arg %q not found in EXTRA_ARGS: %q", arg, gotArgs)
					}
				}
				return true, nil
			}
			return false, errors.New("no rule-evaluator container found")
		})
		if err != nil {
			t.Fatalf("waiting for collector configuration failed: %s", err)
		}
	}
}

func testRuleEvaluatorSecrets(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		cert, key, err := cert.GenerateSelfSignedCertKey("test", nil, nil)
		if err != nil {
			t.Errorf("generating tls cert and key pair: %s", err)
		}

		tlsPair := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-authorization",
				Namespace: operator.DefaultPublicNamespace,
			},
			Data: map[string][]byte{
				"token": []byte("auth-bearer-password"),
			},
		}
		authToken := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-tls",
				Namespace: operator.DefaultPublicNamespace,
			},
			Data: map[string][]byte{
				"cert": cert,
				"key":  key,
			},
		}

		_, err = kubeClient.CoreV1().Secrets(operator.DefaultPublicNamespace).Create(ctx, tlsPair, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("creating tls secret")
		}
		_, err = kubeClient.CoreV1().Secrets(operator.DefaultPublicNamespace).Create(ctx, authToken, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("creating tls secret")
		}

		createRuleEvaluatorOperatorConfig(ctx, opClient, "alertmanager-tls", "alertmanager-authorization")

		// Verify contents but without the GCP SA credentials file to not leak secrets in tests logs.
		// Whether the contents were copied correctly is implicitly verified by the credentials working.
		want := map[string][]byte{
			fmt.Sprintf("secret_%s_alertmanager-tls_cert", operator.DefaultPublicNamespace):            cert,
			fmt.Sprintf("secret_%s_alertmanager-tls_key", operator.DefaultPublicNamespace):             key,
			fmt.Sprintf("secret_%s_alertmanager-authorization_token", operator.DefaultPublicNamespace): []byte("auth-bearer-password"),
		}
		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			secret, err := kubeClient.CoreV1().Secrets(operator.DefaultOperatorNamespace).Get(ctx, operator.RulesSecretName, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("get secret: %w", err)
			}

			// Ensure secret contains the expected keys.
			for k, want := range want {
				if got, ok := secret.Data[k]; !ok {
					err = fmt.Errorf("expected key not found: %s", err)
					t.Logf("diff for rules secret: %s", err)
				} else if diff := cmp.Diff(want, got); diff != "" {
					err = fmt.Errorf("unexpected secret value (-want, +got): %s", diff)
					t.Logf("diff for rules secret: %s", err)
				}
			}
			return err == nil, nil
		})
		if err != nil {
			t.Fatalf("waiting for generated rule-evaluator config: %s", err)
		}
	}
}

func testRuleEvaluatorConfiguration(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		replace := func(s string) string {
			return strings.NewReplacer(
				"{namespace}", operator.DefaultOperatorNamespace,
				"{pubNamespace}", operator.DefaultPublicNamespace,
			).Replace(s)
		}

		want := map[string]string{
			"config.yaml": replace(`global:
    external_labels:
        external_key: external_val
alerting:
    alertmanagers:
        - follow_redirects: true
          enable_http2: true
          scheme: http
          timeout: 10s
          api_version: v2
          static_configs:
            - targets:
                - alertmanager.{namespace}:9093
        - authorization:
            type: Bearer
            credentials_file: /etc/secrets/secret_{pubNamespace}_alertmanager-authorization_token
          tls_config:
            cert_file: /etc/secrets/secret_{pubNamespace}_alertmanager-tls_cert
            key_file: /etc/secrets/secret_{pubNamespace}_alertmanager-tls_key
            insecure_skip_verify: false
          follow_redirects: true
          enable_http2: true
          scheme: https
          path_prefix: /test
          timeout: 30s
          api_version: v2
          relabel_configs:
            - source_labels: [__meta_kubernetes_endpoints_name]
              regex: test-am
              action: keep
            - source_labels: [__address__]
              regex: (.+):\d+
              target_label: __address__
              replacement: $1:19093
              action: replace
          kubernetes_sd_configs:
            - role: endpoints
              kubeconfig_file: ""
              follow_redirects: true
              enable_http2: true
              namespaces:
                own_namespace: false
                names:
                    - monitoring1
rule_files:
    - /etc/rules/*.yaml
`),
		}

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			cm, err := kubeClient.CoreV1().ConfigMaps(operator.DefaultOperatorNamespace).Get(ctx, operator.NameRuleEvaluator, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("get configmap: %w", err)
			}
			if diff := cmp.Diff(want, cm.Data); diff != "" {
				err = fmt.Errorf("unexpected configuration (-want, +got): %s", diff)
				t.Logf("diff for rules configmap: %s", err)
			}
			return err == nil, nil
		})
		if err != nil {
			t.Fatalf("failed waiting for generated rule-evaluator config: %s", err)
		}
	}
}

func testCreateRules(ctx context.Context, t *testing.T, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("creating rules")

		rules := &monitoringv1.Rules{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "always-one",
				Namespace: "default",
			},
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "group-1",
						Rules: []monitoringv1.Rule{
							{
								Record: "always_one",
								Expr:   "vector(1)",
							},
						},
					},
				},
			},
		}

		_, err := opClient.MonitoringV1().Rules("default").Create(ctx, rules, metav1.CreateOptions{})
		if err != nil {
			t.Errorf("creating rules: %s", err)
		}
	}
}

func testValidateRuleEvaluationMetrics(ctx context.Context, t *testing.T) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking for metrics in Cloud Monitoring")

		// Wait for metric data to show up in Cloud Monitoring.
		metricClient, err := gcm.NewMetricClient(ctx)
		if err != nil {
			t.Fatalf("create metric client: %s", err)
		}
		defer metricClient.Close()

		err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			now := time.Now()

			// Validate the majority of labels being set correctly by filtering along them.
			iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
				Name: fmt.Sprintf("projects/%s", projectID),
				Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.labels.location = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.namespace = "%s" AND
				metric.type = "prometheus.googleapis.com/always_one/gauge"
				`,
					projectID, location, cluster, "default",
				),
				Interval: &gcmpb.TimeInterval{
					EndTime:   timestamppb.New(now),
					StartTime: timestamppb.New(now.Add(-10 * time.Second)),
				},
			})
			series, err := iter.Next()
			if err == iterator.Done {
				t.Logf("no data in GCM, retrying...")
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("querying metrics failed: %w", err)
			}
			if len(series.Points) == 0 {
				return false, errors.New("unexpected zero points in result series")
			}
			// We expect exactly one result.
			series, err = iter.Next()
			if err != iterator.Done {
				return false, fmt.Errorf("expected iterator to be done but series %v: %w", series, err)
			}
			return true, nil
		})
		if err != nil {
			t.Fatalf("waiting for rule metrics to appear in GCM failed: %s", err)
		}
	}
}

// createRuleEvaluatorOperatorConfig ensures an OperatorConfig can be deployed
// that contains rule-evaluator configuration.
func createRuleEvaluatorOperatorConfig(ctx context.Context, opClient versioned.Interface, certSecretName, tokenSecretName string) error {
	config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
	if err != nil {
		return err
	}
	// Setup TLS secret selectors.
	certSecret := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: certSecretName,
		},
		Key: "cert",
	}

	keySecret := certSecret.DeepCopy()
	keySecret.Key = "key"

	config.Rules.Alerting = monitoringv1.AlertingSpec{
		Alertmanagers: []monitoringv1.AlertmanagerEndpoints{
			{
				Name:       "test-am",
				Namespace:  "monitoring",
				Port:       intstr.IntOrString{IntVal: 19093},
				Timeout:    "30s",
				APIVersion: "v2",
				PathPrefix: "/test",
				Scheme:     "https",
				Authorization: &monitoringv1.Authorization{
					Type: "Bearer",
					Credentials: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: tokenSecretName,
						},
						Key: "token",
					},
				},
				TLS: &monitoringv1.TLSConfig{
					Cert: &monitoringv1.SecretOrConfigMap{
						Secret: certSecret,
					},
					KeySecret: keySecret,
				},
			},
		},
	}

	_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	return nil
}
