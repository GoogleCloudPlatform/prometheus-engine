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
	"sort"
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
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/util/cert"
	ctrl "sigs.k8s.io/controller-runtime"
	kyaml "sigs.k8s.io/yaml"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1alpha1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1alpha1"
)

var (
	kubeconfig        *rest.Config
	projectID         string
	cluster           string
	location          string
	skipGCM           bool
	gcpServiceAccount string
)

func TestMain(m *testing.M) {
	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")

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
	t.Run("self-podmonitoring", tctx.subtest(testCollectorSelfPodMonitoring))
	t.Run("self-clusterpodmonitoring", tctx.subtest(testCollectorSelfClusterPodMonitoring))
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

func TestRuleEvaluation(t *testing.T) {
	tctx := newTestContext(t)

	cert, key, err := cert.GenerateSelfSignedCertKey("test", nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("rule evaluator create alertmanager secrets", tctx.subtest(func(ctx context.Context, t *testContext) {
		testCreateAlertmanagerSecrets(ctx, t, cert, key)
	}))
	t.Run("rule evaluator operatorconfig", tctx.subtest(testRuleEvaluatorOperatorConfig))
	t.Run("rule evaluator secrets", tctx.subtest(func(ctx context.Context, t *testContext) {
		testRuleEvaluatorSecrets(ctx, t, cert, key)
	}))
	t.Run("rule evaluator config", tctx.subtest(testRuleEvaluatorConfig))
	t.Run("rule generation", tctx.subtest(testRulesGeneration))
	t.Run("rule evaluator deploy", tctx.subtest(testRuleEvaluatorDeployment))

	if !skipGCM {
		t.Log("Waiting rule results to become readable")
		t.Run("check rule metrics", tctx.subtest(testValidateRuleEvaluationMetrics))
	}
}

// testRuleEvaluatorOperatorConfig ensures an OperatorConfig can be deployed
// that contains rule-evaluator configuration.
func testRuleEvaluatorOperatorConfig(ctx context.Context, t *testContext) {
	// Setup TLS secret selectors.
	certSecret := &corev1.SecretKeySelector{
		LocalObjectReference: corev1.LocalObjectReference{
			Name: "alertmanager-tls",
		},
		Key: "cert",
	}

	keySecret := certSecret.DeepCopy()
	keySecret.Key = "key"

	opCfg := &monitoringv1alpha1.OperatorConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: operator.NameOperatorConfig,
		},
		Rules: monitoringv1alpha1.RuleEvaluatorSpec{
			ExternalLabels: map[string]string{
				"external_key": "external_val",
			},
			QueryProjectID: projectID,
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
							Credentials: &v1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: "alertmanager-authorization",
								},
								Key: "token",
							},
						},
						TLS: &monitoringv1alpha1.TLSConfig{
							Cert: &monitoringv1alpha1.SecretOrConfigMap{
								Secret: certSecret,
							},
							KeySecret: keySecret,
						},
					},
				},
			},
		},
	}
	if gcpServiceAccount != "" {
		opCfg.Rules.Credentials = &v1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "user-gcp-service-account",
			},
			Key: "key.json",
		}
	}
	_, err := t.operatorClient.MonitoringV1alpha1().OperatorConfigs(t.pubNamespace).Create(ctx, opCfg, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create rules operatorconfig: %s", err)
	}
}

func testCreateAlertmanagerSecrets(ctx context.Context, t *testContext, cert, key []byte) {
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

func testRuleEvaluatorSecrets(ctx context.Context, t *testContext, cert, key []byte) {
	// Verify contents but without the GCP SA credentials file to not leak secrets in tests logs.
	// Whether the contents were copied correctly is implicitly verified by the credentials working.
	want := map[string][]byte{
		fmt.Sprintf("secret_%s_alertmanager-tls_cert", t.pubNamespace):            cert,
		fmt.Sprintf("secret_%s_alertmanager-tls_key", t.pubNamespace):             key,
		fmt.Sprintf("secret_%s_alertmanager-authorization_token", t.pubNamespace): []byte("auth-bearer-password"),
	}
	err := wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		secret, err := t.kubeClient.CoreV1().Secrets(t.namespace).Get(ctx, operator.RulesSecretName, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get secret")
		}
		delete(secret.Data, fmt.Sprintf("secret_%s_user-gcp-service-account_key.json", t.pubNamespace))

		if diff := cmp.Diff(want, secret.Data); diff != "" {
			return false, errors.Errorf("unexpected configuration (-want, +got): %s", diff)
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("failed waiting for generated rule-evaluator config: %s", err)
	}

}

func testRuleEvaluatorConfig(ctx context.Context, t *testContext) {
	replace := func(s string) string {
		return strings.NewReplacer(
			"{namespace}", t.namespace, "{pubNamespace}", t.pubNamespace,
		).Replace(s)
	}

	want := map[string]string{
		"config.yaml": replace(`global:
    external_labels:
        external_key: external_val
alerting:
    alertmanagers:
        - authorization:
            type: Bearer
            credentials_file: /etc/secrets/secret_{pubNamespace}_alertmanager-authorization_token
          tls_config:
            cert_file: /etc/secrets/secret_{pubNamespace}_alertmanager-tls_cert
            key_file: /etc/secrets/secret_{pubNamespace}_alertmanager-tls_key
            insecure_skip_verify: false
          follow_redirects: true
          scheme: https
          path_prefix: /test
          timeout: 30s
          api_version: v2
          relabel_configs:
            - source_labels: [__meta_kubernetes_service_name]
              regex: test-am
              action: keep
            - source_labels: [__meta_kubernetes_pod_container_port_number]
              regex: "19093"
              action: keep
          kubernetes_sd_configs:
            - role: endpoints
              follow_redirects: true
              namespaces:
                names:
                    - {namespace}
rule_files:
    - /etc/rules/*.yaml
`),
	}
	err := wait.Poll(1*time.Second, 1*time.Minute, func() (bool, error) {
		cm, err := t.kubeClient.CoreV1().ConfigMaps(t.namespace).Get(ctx, "rule-evaluator", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get configmap")
		}
		if diff := cmp.Diff(want, cm.Data); diff != "" {
			return false, errors.Errorf("unexpected configuration (-want, +got): %s", diff)
		}
		return true, nil
	})
	if err != nil {
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
		// When not using GCM, we check the available replicas rather than ready ones
		// as the rule-evaluator's readyness probe does check for connectivity to GCM.
		if skipGCM {
			// TODO(pintohutch): stub CTS API during e2e tests to remove
			// this conditional.
			if *deploy.Spec.Replicas != deploy.Status.UpdatedReplicas {
				return false, nil
			}
		} else if *deploy.Spec.Replicas != deploy.Status.ReadyReplicas {
			return false, nil
		}

		for _, c := range deploy.Spec.Template.Spec.Containers {
			if c.Name != "evaluator" {
				continue
			}
			// We're mainly interested in the dynamic flags but checking the entire set including
			// the static ones is ultimately simpler.
			wantArgs := []string{
				"--config.file=/prometheus/config_out/config.yaml",
				"--web.listen-address=:19092",
				fmt.Sprintf("--export.label.project-id=%s", projectID),
				fmt.Sprintf("--export.label.location=%s", location),
				fmt.Sprintf("--export.label.cluster=%s", cluster),
				fmt.Sprintf("--query.project-id=%s", projectID),
			}
			if skipGCM {
				wantArgs = append(wantArgs, "--export.disable")
			}
			if gcpServiceAccount != "" {
				filepath := fmt.Sprintf("/etc/secrets/secret_%s_user-gcp-service-account_key.json", t.pubNamespace)
				wantArgs = append(wantArgs,
					fmt.Sprintf("--export.credentials-file=%s", filepath),
					fmt.Sprintf("--query.credentials-file=%s", filepath),
				)
			}
			sort.Strings(wantArgs)
			sort.Strings(c.Args)

			if diff := cmp.Diff(wantArgs, c.Args); diff != "" {
				return false, errors.Errorf("unexpected flags (-want, +got): %s", diff)
			}
			return true, nil
		}
		return false, errors.New("no container with name evaluator found")
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
		// CSR v1 API only available in 1.19+ k8s clusters.
		if csr, err := t.kubeClient.CertificatesV1().CertificateSigningRequests().Get(ctx, fqdn, metav1.GetOptions{}); apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Errorf("getting v1 CSR: %s", err)
		} else {
			caBundle = csr.Status.Certificate
		}
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
		Collection: monitoringv1alpha1.CollectionSpec{
			ExternalLabels: map[string]string{
				"external_key": "external_val",
			},
			Filter: monitoringv1alpha1.ExportFilters{
				MatchOneOf: []string{
					"{job='foo'}",
					"{__name__=~'up'}",
				},
			},
		},
	}
	if gcpServiceAccount != "" {
		opCfg.Collection.Credentials = &v1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "user-gcp-service-account",
			},
			Key: "key.json",
		}
	}
	_, err := t.operatorClient.MonitoringV1alpha1().OperatorConfigs(t.pubNamespace).Create(ctx, opCfg, metav1.CreateOptions{})
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
		if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
			return false, nil
		}
		for _, c := range ds.Spec.Template.Spec.Containers {
			if c.Name != "prometheus" {
				continue
			}
			// We're mainly interested in the dynamic flags but checking the entire set including
			// the static ones is ultimately simpler.
			wantArgs := []string{
				"--config.file=/prometheus/config_out/config.yaml",
				"--storage.tsdb.path=/prometheus/data",
				"--storage.tsdb.no-lockfile",
				"--storage.tsdb.retention.time=30m",
				"--storage.tsdb.wal-compression",
				"--storage.tsdb.min-block-duration=10m",
				"--storage.tsdb.max-block-duration=10m",
				fmt.Sprintf("--web.listen-address=:%d", t.collectorPort),
				"--web.enable-lifecycle",
				"--web.route-prefix=/",
				fmt.Sprintf("--export.label.project-id=%s", projectID),
				fmt.Sprintf("--export.label.location=%s", location),
				fmt.Sprintf("--export.label.cluster=%s", cluster),
				"--export.match={job='foo'}",
				"--export.match={__name__=~'up'}",
			}
			if skipGCM {
				wantArgs = append(wantArgs, "--export.disable")
			}
			if gcpServiceAccount != "" {
				wantArgs = append(wantArgs, fmt.Sprintf("--export.credentials-file=/etc/secrets/secret_%s_user-gcp-service-account_key.json", t.pubNamespace))
			}
			sort.Strings(wantArgs)
			sort.Strings(c.Args)

			if diff := cmp.Diff(wantArgs, c.Args); diff != "" {
				return false, errors.Errorf("unexpected flags (-want, +got): %s", diff)
			}
			return true, nil
		}
		return false, errors.New("no container with name prometheus found")
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

// testCollectorSelfClusterPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfClusterPodMonitoring(ctx context.Context, t *testContext) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	podmon := &monitoringv1alpha1.ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "collector-cmon",
			OwnerReferences: t.ownerReferences,
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

	_, err := t.operatorClient.MonitoringV1alpha1().ClusterPodMonitorings().Create(ctx, podmon, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector ClusterPodMonitoring: %s", err)
	}
	t.Log("Waiting for PodMonitoring collector-podmon to be processed")

	var resVer = ""
	err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
		pm, err := t.operatorClient.MonitoringV1alpha1().ClusterPodMonitorings().Get(ctx, "collector-cmon", metav1.GetOptions{})
		if err != nil {
			return false, errors.Errorf("getting ClusterPodMonitoring failed: %s", err)
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
		t.Errorf("unable to validate ClusterPodMonitoring status: %s", err)
	}

	if !skipGCM {
		t.Log("Waiting for up metrics for collector targets")
		validateCollectorUpMetrics(ctx, t, "collector-cmon")
	}
}

// validateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func validateCollectorUpMetrics(ctx context.Context, t *testContext, job string) {
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
				metric.type = "prometheus.googleapis.com/up/gauge" AND
				metric.labels.external_key = "external_val"
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

func testRulesGeneration(ctx context.Context, t *testContext) {
	replace := strings.NewReplacer(
		"{project_id}", projectID,
		"{cluster}", cluster,
		"{namespace}", t.namespace,
	).Replace

	// Create multiple rules in the cluster and expect their scoped equivalents
	// to be present in the generated rule file.
	content := replace(`
apiVersion: monitoring.googleapis.com/v1alpha1
kind: ClusterRules
metadata:
  name: {namespace}-cluster-rules
spec:
  groups:
  - name: group-1
    rules:
    - record: foo
      expr: sum(up)
      labels:
        flavor: test
`)
	var clusterRules monitoringv1alpha1.ClusterRules
	if err := kyaml.Unmarshal([]byte(content), &clusterRules); err != nil {
		t.Fatal(err)
	}
	clusterRules.OwnerReferences = t.ownerReferences

	if _, err := t.operatorClient.MonitoringV1alpha1().ClusterRules().Create(context.TODO(), &clusterRules, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	// TODO(freinartz): Instantiate structs directly rather than templating strings.
	content = `
apiVersion: monitoring.googleapis.com/v1alpha1
kind: Rules
metadata:
  name: rules
spec:
  groups:
  - name: group-1
    rules:
    - alert: Bar
      expr: avg(down) > 1
      annotations:
        description: "bar avg down"
      labels:
        flavor: test
    - record: always_one
      expr: vector(1)
`
	var rules monitoringv1alpha1.Rules
	if err := kyaml.Unmarshal([]byte(content), &rules); err != nil {
		t.Fatal(err)
	}
	if _, err := t.operatorClient.MonitoringV1alpha1().Rules(t.namespace).Create(context.TODO(), &rules, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		replace("clusterrules__{namespace}-cluster-rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - record: foo
          expr: sum(up{cluster="{cluster}",project_id="{project_id}"})
          labels:
            cluster: {cluster}
            flavor: test
            project_id: {project_id}
`),
		replace("rules__{namespace}__rules.yaml"): replace(`groups:
    - name: group-1
      rules:
        - alert: Bar
          expr: avg(down{cluster="{cluster}",namespace="{namespace}",project_id="{project_id}"}) > 1
          labels:
            cluster: {cluster}
            flavor: test
            namespace: {namespace}
            project_id: {project_id}
          annotations:
            description: bar avg down
        - record: always_one
          expr: vector(1)
          labels:
            cluster: {cluster}
            namespace: {namespace}
            project_id: {project_id}
`),
	}

	var diff string

	err := wait.Poll(1*time.Second, time.Minute, func() (bool, error) {
		cm, err := t.kubeClient.CoreV1().ConfigMaps(t.namespace).Get(context.TODO(), "rules-generated", metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			return false, errors.Wrap(err, "get ConfigMap")
		}
		// The operator observes Rules across all namespaces. For the purpose of this test we drop
		// all outputs from the result that aren't in the expected set.
		for name := range cm.Data {
			if _, ok := want[name]; !ok {
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

func testValidateRuleEvaluationMetrics(ctx context.Context, t *testContext) {
	// The project and cluster name in which we look for the metric data must
	// be provided by the user. Check this only in this test so tests that don't need these
	// flags can still be run without them.
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

	err = wait.Poll(1*time.Second, 3*time.Minute, func() (bool, error) {
		now := time.Now()

		// Validate the majority of labels being set correctly by filtering along them.
		iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
			Name: fmt.Sprintf("projects/%s", projectID),
			Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.namespace = "%s" AND
				metric.type = "prometheus.googleapis.com/always_one/gauge"
				`,
				projectID, cluster, t.namespace,
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
		if len(series.Points) == 0 {
			return false, errors.New("unexpected zero points in result series")
		}
		// We expect exactly one result.
		series, err = iter.Next()
		if err != iterator.Done {
			return false, errors.Errorf("expected iterator to be done but got error %q and series %v", err, series)
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("Waiting for rule metrics to appear in Cloud Monitoring failed: %s", err)
	}
}
