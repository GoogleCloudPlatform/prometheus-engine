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
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	gcmpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/api"
	prometheus "github.com/prometheus/client_golang/api/prometheus/v1"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
)

const configReloaderContainerName = "config-reloader"

func TestRuleEvaluatorNoCompression(t *testing.T) {
	testRuleEvaluator(t, monitoringv1.OperatorFeatures{
		Config: monitoringv1.ConfigSpec{
			Compression: monitoringv1.CompressionNone,
		},
	})
}

func TestRuleEvaluatorGzipCompression(t *testing.T) {
	testRuleEvaluator(t, monitoringv1.OperatorFeatures{
		Config: monitoringv1.ConfigSpec{
			Compression: monitoringv1.CompressionGzip,
		},
	})
}

func testRuleEvaluator(t *testing.T, features monitoringv1.OperatorFeatures) {
	ctx := contextWithDeadline(t)
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("rule-evaluator-deployed", testRuleEvaluatorDeployed(ctx, kubeClient))
	t.Run("rule-evaluator-operatorconfig", testRuleEvaluatorOperatorConfig(ctx, kubeClient, features))
	// TODO(pintohutch): testing the generated secrets and config can be
	// brittle as the checks need to be precise and could break if mechanics or
	// formatting changes in the future.
	// Ideally this is replaced by a true e2e test that deploys a custom
	// secured alertmanager and successfully sends alerts to it.
	t.Run("rule-evaluator-secrets", testRuleEvaluatorSecrets(ctx, kubeClient))
	t.Run("rule-evaluator-configuration", testRuleEvaluatorConfiguration(ctx, kubeClient))

	t.Run("rules-create", testCreateRules(ctx, restConfig, kubeClient, operator.DefaultOperatorNamespace, metav1.NamespaceDefault, features))
	if !skipGCM {
		t.Run("rules-gcm", testValidateRuleEvaluationMetrics(ctx))
	}
}

func testRuleEvaluatorDeployed(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking rule-evaluator is running")

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			deploy := appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.NameRuleEvaluator,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&deploy), &deploy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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

func testRuleEvaluatorOperatorConfig(ctx context.Context, kubeClient client.Client, features monitoringv1.OperatorFeatures) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking rule-evaluator is configured")

		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}

		// Configure
		config.Rules.ExternalLabels = map[string]string{
			"external_key": "external_val"}
		config.Rules.GeneratorURL = "http://example.com/"
		config.Features = features

		if err := kubeClient.Update(ctx, &config); err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}
		// Keep checking the state of the collectors until they're running.
		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			deploy := appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.NameRuleEvaluator,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&deploy), &deploy); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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

func testRuleEvaluatorSecrets(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		cert, key, err := cert.GenerateSelfSignedCertKey("test", nil, nil)
		if err != nil {
			t.Errorf("generating tls cert and key pair: %s", err)
		}

		tlsPair := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-authorization",
				Namespace: operator.DefaultPublicNamespace,
			},
			Data: map[string][]byte{
				"token": []byte("auth-bearer-password"),
			},
		}
		authToken := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-tls",
				Namespace: operator.DefaultPublicNamespace,
			},
			Data: map[string][]byte{
				"cert": cert,
				"key":  key,
			},
		}

		if err := kubeClient.Create(ctx, &tlsPair); err != nil {
			t.Errorf("creating tls secret: %s", err)
		}
		if err := kubeClient.Create(ctx, &authToken); err != nil {
			t.Errorf("creating tls secret: %s", err)
		}

		if err := createRuleEvaluatorOperatorConfig(ctx, kubeClient, "alertmanager-tls", "alertmanager-authorization"); err != nil {
			t.Errorf("creating operatorconfig: %s", err)
		}

		// Verify contents but without the GCP SA credentials file to not leak secrets in tests logs.
		// Whether the contents were copied correctly is implicitly verified by the credentials working.
		want := map[string][]byte{
			fmt.Sprintf("secret_%s_alertmanager-tls_cert", operator.DefaultPublicNamespace):            cert,
			fmt.Sprintf("secret_%s_alertmanager-tls_key", operator.DefaultPublicNamespace):             key,
			fmt.Sprintf("secret_%s_alertmanager-authorization_token", operator.DefaultPublicNamespace): []byte("auth-bearer-password"),
		}
		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			secret := corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.RulesSecretName,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&secret), &secret); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
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

func testRuleEvaluatorConfiguration(ctx context.Context, kubeClient client.Client) func(*testing.T) {
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
                    - monitoring
rule_files:
    - /etc/rules/*.yaml
`),
		}

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			cm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      operator.NameRuleEvaluator,
					Namespace: operator.DefaultOperatorNamespace,
				},
			}
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&cm), &cm); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, fmt.Errorf("get configmap: %w", err)
			}
			var err error
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

func testCreateRules(
	ctx context.Context,
	restConfig *rest.Config,
	kubeClient client.Client,
	systemNamespace,
	userNamespace string,
	features monitoringv1.OperatorFeatures,
) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("creating rules")

		timeStart := time.Now()
		replace := strings.NewReplacer(
			"{project_id}", projectID,
			"{cluster}", cluster,
			"{location}", location,
			"{namespace}", userNamespace,
		).Replace

		// Create multiple rules in the cluster and expect their scoped equivalents
		// to be present in the generated rule file.
		if err := kubeClient.Create(ctx, &monitoringv1.GlobalRules{
			ObjectMeta: metav1.ObjectMeta{
				Name: userNamespace + "-global-rules",
			},
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "group-1",
						Rules: []monitoringv1.Rule{
							{
								Record: "bar",
								Expr:   "avg(up)",
								Labels: map[string]string{
									"flavor": "test",
								},
							},
						},
					},
				},
			},
		}); err != nil {
			t.Fatal(err)
		}

		if err := kubeClient.Create(ctx, &monitoringv1.ClusterRules{
			ObjectMeta: metav1.ObjectMeta{
				Name: userNamespace + "-cluster-rules",
			},
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "group-1",
						Rules: []monitoringv1.Rule{
							{
								Record: "foo",
								Expr:   "sum(up)",
								Labels: map[string]string{
									"flavor": "test",
								},
							},
						},
					},
				},
			},
		}); err != nil {
			t.Fatal(err)
		}

		if err := kubeClient.Create(ctx, &monitoringv1.Rules{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "rules",
				Namespace: userNamespace,
			},
			Spec: monitoringv1.RulesSpec{
				Groups: []monitoringv1.RuleGroup{
					{
						Name: "group-1",
						Rules: []monitoringv1.Rule{
							{
								Alert: "Bar",
								Expr:  "avg(down) > 1",
								Annotations: map[string]string{
									"description": "bar avg down",
								},
								Labels: map[string]string{
									"flavor": "test",
								},
							},
							{
								Record: "always_one",
								Expr:   "vector(1)",
							},
						},
					},
				},
			},
		}); err != nil {
			t.Fatal(err)
		}

		want := map[string]string{
			"empty.yaml": "",
			replace("globalrules__{namespace}-global-rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - record: bar
          expr: avg(up)
          labels:
            flavor: test
`),
			replace("clusterrules__{namespace}-cluster-rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - record: foo
          expr: sum(up{cluster="{cluster}",location="{location}",project_id="{project_id}"})
          labels:
            cluster: {cluster}
            flavor: test
            location: {location}
            project_id: {project_id}
`),
			replace("rules__{namespace}__rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - alert: Bar
          expr: avg(down{cluster="{cluster}",location="{location}",namespace="{namespace}",project_id="{project_id}"}) > 1
          labels:
            cluster: {cluster}
            flavor: test
            location: {location}
            namespace: {namespace}
            project_id: {project_id}
          annotations:
            description: bar avg down
        - record: always_one
          expr: vector(1)
          labels:
            cluster: {cluster}
            location: {location}
            namespace: {namespace}
            project_id: {project_id}
`),
		}

		var diff string

		err := wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			var cm corev1.ConfigMap
			if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: systemNamespace, Name: "rules-generated"}, &cm); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, fmt.Errorf("get ConfigMap: %w", err)
			}
			data := cm.Data
			if features.Config.Compression == monitoringv1.CompressionGzip {
				// When compression is enabled, we expect the config map with recording
				// rules to be compressed with gzip. Decompress all files for validation.
				for key, compressedData := range cm.BinaryData {
					r, err := gzip.NewReader(bytes.NewReader(compressedData))
					if err != nil {
						t.Fatal(err)
					}
					decompressed, err := io.ReadAll(r)
					if err != nil {
						t.Fatal(err)
					}
					if _, ok := data[key]; ok {
						t.Errorf("duplicate ConfigMap key %q", key)
					}
					data[key] = string(decompressed)
				}
			}

			diff = cmp.Diff(want, data)
			return diff == "", nil
		})
		if err != nil {
			t.Errorf("diff (-want, +got): %s", diff)
			t.Fatalf("failed waiting for generated rules: %s", err)
		}

		httpClient, err := kube.PortForwardClient(
			restConfig,
			kubeClient,
			writerFn(func(p []byte) (n int, err error) {
				t.Logf("portforward: info: %s", string(p))
				return len(p), nil
			}),
			writerFn(func(p []byte) (n int, err error) {
				t.Logf("portforward: error: %s", string(p))
				return len(p), nil
			}),
		)
		if err != nil {
			t.Fatalf("failed to create port forward client: %s", err)
		}

		if err := kube.WaitForDeploymentReady(ctx, kubeClient, systemNamespace, operator.NameRuleEvaluator); err != nil {
			t.Errorf("rule-evaluator is not ready: %s", err)
			out := strings.Builder{}
			if err := kube.Debug(ctx, restConfig, kubeClient, &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: systemNamespace,
					Name:      operator.NameRuleEvaluator,
				},
			}, &out,
			); err != nil {
				t.Fatalf("unable to debug: %s", err)
			}
			t.Fatalf("debug:\n%s", out.String())
		}
		pod, err := ruleEvaluatorPod(ctx, kubeClient, systemNamespace)
		if err != nil {
			t.Fatalf("unable to get rule-evaluator pod: %s", err)
		}

		err = wait.PollUntilContextTimeout(ctx, 3*time.Second, 3*time.Minute, true, func(ctx context.Context) (bool, error) {
			updated, err := isRuntimeInfoUpdatedSince(ctx, httpClient, pod, 19092, timeStart)
			if err != nil {
				t.Logf("unable to check rule-evaluator status: %s", err)
				return false, nil
			}
			return updated, nil
		})
		if err != nil {
			t.Fatalf("failed waiting for collectors to be updated: %s", err)
		}

		logs, err := kube.PodLogs(ctx, restConfig, pod.Namespace, pod.Name, configReloaderContainerName)
		if err != nil {
			t.Fatalf("unable to fetch rule-evaluator config-reloader logs: %s", err)
		}
		line, err := logsError(logs)
		if err != nil {
			t.Fatalf("unable to read logs: %s", err)
		}
		if line != "" {
			t.Fatalf("found error in rule-evaluator config-reloader logs: %s", line)
		}
	}
}

func testValidateRuleEvaluationMetrics(ctx context.Context) func(*testing.T) {
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
func createRuleEvaluatorOperatorConfig(ctx context.Context, kubeClient client.Client, certSecretName, tokenSecretName string) error {
	config := monitoringv1.OperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.NameOperatorConfig,
			Namespace: operator.DefaultPublicNamespace,
		},
	}
	if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
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

	if err := kubeClient.Update(ctx, &config); err != nil {
		return err
	}
	return nil
}

func ruleEvaluatorPod(ctx context.Context, kubeClient client.Client, namespace string) (*corev1.Pod, error) {
	podList, err := kube.DeploymentPods(ctx, kubeClient, namespace, operator.NameRuleEvaluator)
	if err != nil {
		return nil, err
	}
	if len(podList) != 1 {
		return nil, fmt.Errorf("expected 1 pod, found %d", len(podList))
	}
	return &podList[0], nil
}

type writerFn func(p []byte) (n int, err error)

func (w writerFn) Write(p []byte) (n int, err error) {
	return w(p)
}

func isRuntimeInfoUpdatedSince(ctx context.Context, httpClient *http.Client, pod *corev1.Pod, port int32, since time.Time) (bool, error) {
	runtimeInfo, err := getRuntimeInfo(ctx, httpClient, pod, port)
	if err != nil {
		return false, err
	}
	if since.After(runtimeInfo.LastConfigTime) {
		return false, nil
	}
	if !runtimeInfo.ReloadConfigSuccess {
		return false, fmt.Errorf("pod %s failed to reload config", pod.Name)
	}

	return true, nil
}

func getRuntimeInfo(ctx context.Context, httpClient *http.Client, pod *corev1.Pod, port int32) (*prometheus.RuntimeinfoResult, error) {
	client, err := api.NewClient(api.Config{
		Address: fmt.Sprintf("http://%s:%d", pod.Status.PodIP, port),
		Client:  httpClient,
	})
	if err != nil {
		return nil, err
	}
	v1api := prometheus.NewAPI(client)
	runtimeInfo, err := v1api.Runtimeinfo(ctx)
	if err != nil {
		return nil, err
	}

	return &runtimeInfo, nil
}

func logsError(logs string) (string, error) {
	lines := strings.Split(logs, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		data := map[string]string{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			return "", fmt.Errorf("unable to unmarshal log line: %s", err)
		}
		if data["level"] == "error" {
			return line, nil
		}
	}
	return "", nil
}
