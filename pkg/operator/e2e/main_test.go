// Copyright 2021 Google LLC
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

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package e2e

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
	gcmpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	kyaml "sigs.k8s.io/yaml"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

var (
	kubeconfig *rest.Config
	projectID  string
	cluster    string
	location   string
	skipGCM    bool
)

func TestMain(m *testing.M) {
	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", true, "Skip validating GCM ingested points.")

	flag.Parse()

	var err error
	kubeconfig, err = ctrl.GetConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Loading kubeconfig failed:", err)
		os.Exit(1)
	}

	go func() {
		os.Exit(m.Run())
	}()

	// If the process gets terminated by the user, the Go test package
	// doesn't ensure that test cleanup functions are run.
	// Deleting all namespaces ensures we don't leave anything behind regardless.
	// Non-namespaced resources are owned by a namespace and thus cleaned up
	// by Kubernetes' garbage collection.
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	<-term
	if err := cleanupAllNamespaces(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "Cleaning up namespaces failed:", err)
		os.Exit(1)
	}
}

func TestCollectorPodMonitoring(t *testing.T) {
	tctx := newTestContext(t)

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("deployed", tctx.subtest(testCollectorDeployed))
	t.Run("self-monitoring", tctx.subtest(testCollectorSelfPodMonitoring))
}

// This is hacky.
// This is set during the subtest call to `testCSRIssued` and
// validated against in `testValidatingWebhookConfig`.
var caBundle []byte

func TestCSRWithValidatingWebhookConfig(t *testing.T) {
	tctx := newTestContext(t)

	t.Cleanup(func() { caBundle = []byte{} })
	t.Run("certificate issue", tctx.subtest(testCSRIssued))
	t.Run("validatingwebhook configuration valid", tctx.subtest(testValidatingWebhookConfig))
}

func TestOperatorConfig(t *testing.T) {
	tctx := newTestContext(t)

	t.Run("rule evaluator create alertmanager secrets", tctx.subtest(testCreateAlertmanagerSecrets))
	t.Run("rule evaluator operatorconfig", tctx.subtest(testRuleEvaluatorOperatorConfig))
	t.Run("rule evaluator secrets", tctx.subtest(testRuleEvaluatorSecrets))
	t.Run("rule evaluator config", tctx.subtest(testRuleEvaluatorConfig))
	t.Run("rule evaluator deploy", tctx.subtest(testRuleEvaluatorDeployment))
}

// testRuleEvaluatorOperatorConfig ensures an OperatorConfig can be deployed
// that contains rule-evaluator configuration.
func testRuleEvaluatorOperatorConfig(ctx context.Context, t *testContext) {
	// Setup TLS secret selectors.
	caSecret := &monitoringv1alpha1.NamespacedSecretKeySelector{
		// Choose the non-test namespace to ensure we can read across namespaces.
		Namespace: "default",
		SecretKeySelector: corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "alertmanager-tls",
			},
			Key: "ca",
		},
	}

	certSecret := caSecret.DeepCopy()
	certSecret.Key = "cert"

	keySecret := caSecret.DeepCopy()
	keySecret.Key = "key"

	opCfg := &monitoringv1alpha1.OperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: operator.NameOperatorConfig,
		},
		Rules: monitoringv1alpha1.RuleEvaluatorSpec{
			ProjectID:      "test-proj",
			LabelProjectID: "label-proj",
			LabelLocation:  "label-loc",
			Alerting: monitoringv1alpha1.AlertingSpec{
				Alertmanagers: []monitoringv1alpha1.AlertmanagerEndpoints{
					{
						Name:       "test-am",
						Namespace:  t.namespace,
						Port:       intstr.IntOrString{IntVal: 19093},
						Timeout:    "30s",
						APIVersion: "v2",
						PathPrefix: "/test",
						Scheme:     "https",
						Authorization: &monitoringv1alpha1.Authorization{
							Type: "Bearer",
							Credentials: &monitoringv1alpha1.NamespacedSecretKeySelector{
								// Choose the non-test namespace to ensure we can read across namespaces.
								Namespace: "default",
								SecretKeySelector: corev1.SecretKeySelector{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "alertmanager-authorization",
									},
									Key: "auth-credentials",
								},
							},
						},
						TLSConfig: &monitoringv1alpha1.TLSConfig{
							CA: monitoringv1alpha1.NamespacedSecretOrConfigMap{
								Secret: caSecret,
							},
							Cert: monitoringv1alpha1.NamespacedSecretOrConfigMap{
								Secret: certSecret,
							},
							KeySecret: keySecret,
						},
					},
				},
			},
		},
	}
	_, err := t.operatorClient.MonitoringV1alpha1().OperatorConfigs(t.namespace).Create(ctx, opCfg, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create rules operatorconfig: %s", err)
	}
}

func testCreateAlertmanagerSecrets(ctx context.Context, t *testContext) {
	secrets := []*corev1.Secret{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-authorization",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"auth-credentials": []byte("auth-bearer-password"),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "alertmanager-tls",
				Namespace: "default",
			},
			Data: map[string][]byte{
				"ca":   []byte("cert-authority"),
				"cert": []byte("cert-client"),
				"key":  []byte("key-client"),
			},
		},
	}

	for _, s := range secrets {
		if _, err := t.kubeClient.CoreV1().Secrets(s.Namespace).Update(ctx, s, metav1.UpdateOptions{}); apierrors.IsNotFound(err) {
			if _, err := t.kubeClient.CoreV1().Secrets(s.Namespace).Create(ctx, s, metav1.CreateOptions{}); err != nil {
				t.Fatalf("create alertmanager secret: %s", err)
			}
		} else if err != nil {
			t.Fatalf("update alertmanager secret: %s", err)
		}
	}
}

func testRuleEvaluatorSecrets(ctx context.Context, t *testContext) {
	var diff string
	want := map[string][]byte{
		"secret_default_alertmanager-tls_ca":   []byte("cert-authority"),
		"secret_default_alertmanager-tls_cert": []byte("cert-client"),
		"secret_default_alertmanager-tls_key":  []byte("key-client"),
	}
	err := wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		cm, err := t.kubeClient.CoreV1().Secrets(t.namespace).Get(ctx, operator.RulesSecretName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get secret")
		}

		diff = cmp.Diff(want, cm.Data)
		return diff == "", nil
	})
	if err != nil {
		t.Errorf("diff (-want, +got): %s", diff)
		t.Fatalf("failed waiting for generated rule-evaluator config: %s", err)
	}

}

func testRuleEvaluatorConfig(ctx context.Context, t *testContext) {
	var diff string
	replace := func(s string) []byte {
		r := strings.NewReplacer(
			"{namespace}", t.namespace,
		).Replace(s)
		return []byte(r)
	}

	want := map[string][]byte{
		"config.yaml": replace(`alerting:
  alertmanagers:
  - timeout: 30s
    api_version: v2
    path_prefix: /test
    scheme: https
    authorization:
      type: Bearer
      credentials: auth-bearer-password
    tls_config:
      insecure_skip_verify: false
      ca_file: /etc/secrets/secret_default_alertmanager-tls_ca
      cert_file: /etc/secrets/secret_default_alertmanager-tls_cert
      key_file: /etc/secrets/secret_default_alertmanager-tls_key
    kubernetes_sd_configs:
    - role: endpoints
      namespaces:
        names:
        - {namespace}
    relabel_configs:
    - action: keep
      source_labels:
      - __meta_kubernetes_service_name
      regex: test-am
    - action: keep
      source_labels:
      - __meta_kubernetes_pod_container_port_number
      regex: "19093"
rule_files:
- /etc/rules/*.yaml
`),
	}
	err := wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		cm, err := t.kubeClient.CoreV1().Secrets(t.namespace).Get(ctx, "rule-evaluator", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get configmap")
		}
		diff = cmp.Diff(want, cm.Data)
		return diff == "", nil
	})
	if err != nil {
		t.Errorf("diff (-want, +got): %s", diff)
		t.Fatalf("failed waiting for generated rule-evaluator config: %s", err)
	}

}

func testRuleEvaluatorDeployment(ctx context.Context, t *testContext) {
	err := wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		deploy, err := t.kubeClient.AppsV1().Deployments(t.namespace).Get(ctx, "rule-evaluator", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get deployment")
		}
		// Note: using UpdatedReplicas here instead of ReadyReplicas.
		// This is not ideal, but since the rule-evaluator requires queries
		// against the GMP backend to be returning successfully, this
		// may not always work.
		return *deploy.Spec.Replicas == deploy.Status.UpdatedReplicas, nil
	})
	if err != nil {
		t.Fatalf("failed waiting for generated rule-evaluator deployment: %s", err)
	}
}

// testCSRIssued checks to see if the kube-apiserver issued a valid
// certificate from the CSR.
func testCSRIssued(ctx context.Context, t *testContext) {
	// Operator creates CSR using FQDN format.
	var fqdn = fmt.Sprintf("system:node:%s.%s.svc", operator.NameOperator, t.namespace)
	err := wait.Poll(time.Second, 3*time.Minute, func() (bool, error) {
		// Use v1b1 for now as GKE 1.18 currently uses that version.
		csr, err := t.kubeClient.CertificatesV1beta1().CertificateSigningRequests().Get(ctx, fqdn, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Errorf("getting CSR: %s", err)
		}
		caBundle = csr.Status.Certificate
		// This field is populated once a valid certificate has been issued by the API server.
		return len(caBundle) > 0, nil
	})
	if err != nil {
		t.Fatalf("waiting for CSR issued certificate: %s", err)
	}
}

// testValidatingWebhookConfig checks to see if the validating webhook configuration
// was created with the issued CSR caBundle.
func testValidatingWebhookConfig(ctx context.Context, t *testContext) {
	err := wait.Poll(time.Second, 3*time.Minute, func() (bool, error) {
		vwc, err := t.kubeClient.AdmissionregistrationV1().ValidatingWebhookConfigurations().Get(ctx, operator.NameOperator, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Errorf("getting validatingwebhook configuration: %s", err)
		}
		// Verify all webhooks use correct caBundle from issued CSR.
		for _, wh := range vwc.Webhooks {
			if whBundle := wh.ClientConfig.CABundle; bytes.Compare(whBundle, caBundle) != 0 {
				return false, errors.Errorf("caBundle from CSR: %v mismatches with webhook: %v", caBundle, whBundle)
			}
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("waiting for validatingwebhook configuration: %s", err)
	}
}

// testCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(ctx context.Context, t *testContext) {
	// Create initial OperatorConfig to trigger deployment of resources.
	opCfg := &monitoringv1alpha1.OperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: operator.NameOperatorConfig,
		},
	}
	_, err := t.operatorClient.MonitoringV1alpha1().OperatorConfigs(t.namespace).Create(ctx, opCfg, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create rules operatorconfig: %s", err)
	}

	err = wait.Poll(time.Second, 3*time.Minute, func() (bool, error) {
		ds, err := t.kubeClient.AppsV1().DaemonSets(t.namespace).Get(ctx, operator.NameCollector, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Errorf("getting collector DaemonSet failed: %s", err)
		}
		// At first creation the DaemonSet may appear with 0 desired replicas. This should
		// change shortly after.
		if ds.Status.DesiredNumberScheduled == 0 {
			return false, nil
		}
		var projMatch, clusterMatch, locMatch, collectorArgs bool
		for _, c := range ds.Spec.Template.Spec.Containers {
			if c.Name != "prometheus" {
				continue
			}
			for _, a := range c.Args {
				if a == "--export.label.project-id=test-proj" {
					projMatch = true
				} else if a == "--export.label.cluster=test-cluster" {
					clusterMatch = true
				} else if a == "--export.label.location=test-loc" {
					locMatch = true
				}
			}
		}
		collectorArgs = projMatch && clusterMatch && locMatch
		return (ds.Status.NumberReady == ds.Status.DesiredNumberScheduled) && collectorArgs, nil
	})
	if err != nil {
		t.Fatalf("Waiting for DaemonSet deployment failed: %s", err)
	}
}

// testCollectorSelfPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfPodMonitoring(ctx context.Context, t *testContext) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	podmon := &monitoringv1alpha1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collector-podmon",
		},
		Spec: monitoringv1alpha1.PodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					operator.LabelAppName: operator.NameCollector,
				},
			},
			Endpoints: []monitoringv1alpha1.ScrapeEndpoint{
				{Port: intstr.FromString("prom-metrics"), Interval: "5s"},
				{Port: intstr.FromString("cfg-rel-metrics"), Interval: "5s"},
			},
		},
	}
	_, err := t.operatorClient.MonitoringV1alpha1().PodMonitorings(t.namespace).Create(ctx, podmon, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector PodMonitoring: %s", err)
	}
	t.Log("Waiting for PodMonitoring collector-podmon to be processed")

	var resVer = ""
	err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
		pm, err := t.operatorClient.MonitoringV1alpha1().PodMonitorings(t.namespace).Get(ctx, "collector-podmon", metav1.GetOptions{})
		if err != nil {
			return false, errors.Errorf("getting PodMonitoring failed: %s", err)
		}
		// Ensure no status update cycles.
		// This is not a perfect check as it's possible the get call returns before the operator
		// would sync again, however it can serve as a valuable guardrail in case sporadic test
		// failures start happening due to update cycles.
		if size := len(pm.Status.Conditions); size == 1 {
			if resVer == "" {
				resVer = pm.ResourceVersion
				return false, nil
			}
			success := pm.Status.Conditions[0].Type == monitoringv1alpha1.ConfigurationCreateSuccess
			steadyVer := resVer == pm.ResourceVersion
			return success && steadyVer, nil
		} else if size > 1 {
			return false, errors.Errorf("status conditions should be of length 1, but got: %d", size)
		}
		return false, nil
	})
	if err != nil {
		t.Errorf("unable to validate PodMonitoring status: %s", err)
	}

	if !skipGCM {
		t.Log("Waiting for up metrics for collector targets")
		validateCollectorUpMetrics(ctx, t, "collector-podmon")
	}
}

// validateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func validateCollectorUpMetrics(ctx context.Context, t *testContext, job string) {
	// We rely on the default service account of the collector having write access to GCM.
	// This means it will only work on GKE where the default service account has the default
	// permissions.
	// For support of other environments, the operator will need to be extended by flags
	// to inject different service accounts or secrets.

	// The project and cluster name in which we look for the metric data must
	// be provided by the user. Check this only in this test so tests that don't need these
	// flags can still be run without them.
	// They can be configured on the operator but our current test setup (targeting GKE)
	// relies on the operator inferring them from the environment.
	if projectID == "" {
		t.Fatalf("no project specified (--project-id flag)")
	}
	if cluster == "" {
		t.Fatalf("no cluster name specified (--cluster flag)")
	}

	// Wait for metric data to show up in Cloud Monitoring.
	metricClient, err := gcm.NewMetricClient(ctx)
	if err != nil {
		t.Fatalf("Create GCM metric client: %s", err)
	}
	defer metricClient.Close()

	pods, err := t.kubeClient.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", operator.LabelAppName, operator.NameCollector),
	})
	if err != nil {
		t.Fatalf("List collector pods: %s", err)
	}

	// See whether the `up` metric is written for each pod/port combination. It is set to 1 by
	// Prometheus on successful scraping of the target. Thereby we validate service discovery
	// configuration, config reload handling, as well as data export are correct.
	//
	// Make a single query for each pod/port combo as this is simpler than untangling the result
	// of a single query.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for _, pod := range pods.Items {
		for _, port := range []string{"prom-metrics", "cfg-rel-metrics"} {
			t.Logf("Poll up metric for pod %q and port %q", pod.Name, port)

			err = wait.PollImmediateUntil(3*time.Second, func() (bool, error) {
				now := time.Now()

				// Validate the majority of labels being set correctly by filtering along them.
				iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
					Name: fmt.Sprintf("projects/%s", projectID),
					Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.namespace = "%s" AND
				resource.labels.job = "%s" AND
				resource.labels.instance = "%s:%s" AND
				metric.type = "prometheus.googleapis.com/up/gauge"
				`,
						projectID, cluster, t.namespace, job, pod.Name, port,
					),
					Interval: &gcmpb.TimeInterval{
						EndTime:   timestamppb.New(now),
						StartTime: timestamppb.New(now.Add(-10 * time.Second)),
					},
				})
				series, err := iter.Next()
				if err == iterator.Done {
					t.Logf("No data, retrying...")
					return false, nil
				} else if err != nil {
					return false, errors.Wrap(err, "querying metrics failed")
				}
				if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
					t.Logf("Up still %v, retrying...", v)
					return false, nil
				}
				// We expect exactly one result.
				series, err = iter.Next()
				if err != iterator.Done {
					return false, errors.Errorf("expected iterator to be done but got error %q and series %v", err, series)
				}
				return true, nil
			}, ctx.Done())
			if err != nil {
				t.Fatalf("Waiting for collector metrics to appear in Cloud Monitoring failed: %s", err)
			}
		}
	}
}

func TestRulesGeneration(t *testing.T) {
	tctx := newTestContext(t)

	// Create multiple rules in the cluster and expect their scoped equivalents
	// to be present in the generated rule file.
	files := []string{`
apiVersion: monitoring.googleapis.com/v1alpha1
kind: Rules
metadata:
  name: cluster-rules
spec:
  scope: Cluster
  groups:
  - name: group-1
    rules:
    - record: foo
      expr: sum(up)
      annotations:
        description: "foo sum up"
      labels:
        flavor: test
`, `
apiVersion: monitoring.googleapis.com/v1alpha1
kind: Rules
metadata:
  name: namespace-rules
spec:
  scope: Namespace
  groups:
  - name: group-1
    rules:
    - alert: Bar
      expr: avg(down)
      annotations:
        description: "bar avg down"
      labels:
        flavor: test
`}
	for _, content := range files {
		var rules monitoringv1alpha1.Rules
		if err := kyaml.Unmarshal([]byte(content), &rules); err != nil {
			t.Fatal(err)
		}
		if _, err := tctx.operatorClient.MonitoringV1alpha1().Rules(tctx.namespace).Create(context.TODO(), &rules, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}

	var diff string
	replace := strings.NewReplacer(
		"{project_id}", projectID,
		"{cluster}", cluster,
		"{namespace}", tctx.namespace,
	).Replace

	want := map[string]string{
		replace("{namespace}__cluster-rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - record: foo
          expr: sum(up{cluster="{cluster}",project_id="{project_id}"})
          labels:
            cluster: {cluster}
            flavor: test
            project_id: {project_id}
          annotations:
            description: foo sum up
`),
		replace("{namespace}__namespace-rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - alert: Bar
          expr: avg(down{cluster="{cluster}",namespace="{namespace}",project_id="{project_id}"})
          labels:
            cluster: {cluster}
            flavor: test
            namespace: {namespace}
            project_id: {project_id}
          annotations:
            description: bar avg down
`),
	}

	err := wait.Poll(1*time.Second, time.Minute, func() (bool, error) {
		cm, err := tctx.kubeClient.CoreV1().ConfigMaps(tctx.namespace).Get(context.TODO(), "rules-generated", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get ConfigMap")
		}
		// The operator observes Rules across all namespaces. For the purpose of this test we drop
		// all outputs from the result that were created by Rules not in the test's namespace.
		for name := range cm.Data {
			if !strings.HasPrefix(name, tctx.namespace+"__") {
				delete(cm.Data, name)
			}
		}
		diff = cmp.Diff(want, cm.Data)
		return diff == "", nil
	})
	if err != nil {
		t.Errorf("diff (-want, +got): %s", diff)
		t.Fatalf("failed waiting for generated rules: %s", err)
	}
}
