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
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	gcmpb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kube"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
)

func TestCollectorPodMonitoring(t *testing.T) {
	ctx := contextWithDeadline(t)
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-deployed", testCollectorDeployed(ctx, restConfig, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, kubeClient))
	// Self-scrape podmonitoring.
	t.Run("self-podmonitoring-ready", testEnsurePodMonitoringReady(ctx, kubeClient, collectorPodMonitoring))
	if !skipGCM {
		t.Run("self-podmonitoring-gcm", testValidateCollectorUpMetrics(ctx, kubeClient, "collector-podmon"))
	}
}

func TestCollectorClusterPodMonitoring(t *testing.T) {
	ctx := contextWithDeadline(t)
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-running", testCollectorDeployed(ctx, restConfig, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, kubeClient))
	// Self-scrape clusterpodmonitoring.
	cpm := &monitoringv1.ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: "collector-cmon",
		},
		Spec: monitoringv1.ClusterPodMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					operator.LabelAppName: operator.NameCollector,
				},
			},
			Endpoints: []monitoringv1.ScrapeEndpoint{
				{
					Port:     intstr.FromString(operator.CollectorPrometheusContainerPortName),
					Interval: "5s",
				},
				{
					Port:     intstr.FromString(operator.CollectorConfigReloaderContainerPortName),
					Interval: "5s",
				},
			},
		},
	}
	t.Run("self-clusterpodmonitoring-ready", testEnsureClusterPodMonitoringReady(ctx, kubeClient, cpm))
	if !skipGCM {
		t.Run("self-clusterpodmonitoring-gcm", testValidateCollectorUpMetrics(ctx, kubeClient, "collector-cmon"))
	}
}

func TestCollectorKubeletScraping(t *testing.T) {
	ctx := contextWithDeadline(t)
	kubeClient, restConfig, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-deployed", testCollectorDeployed(ctx, restConfig, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient))

	t.Run("enable-kubelet-scraping", testEnableKubeletScraping(ctx, kubeClient))
	if !skipGCM {
		t.Run("scrape-kubelet", testCollectorScrapeKubelet(ctx, kubeClient))
	}
}

// testCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(ctx context.Context, restConfig *rest.Config, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking collector is running")

		ds := appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameCollector,
				Namespace: operator.DefaultOperatorNamespace,
			},
		}

		// Keep checking the state of the collectors until they're running.
		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&ds), &ds); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, fmt.Errorf("getting collector DaemonSet failed: %w", err)
			}
			// At first creation the DaemonSet may appear with 0 desired replicas. This should
			// change shortly after.
			if ds.Status.DesiredNumberScheduled == 0 {
				return false, nil
			}

			// Ensure all collectors are ready.
			if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
				return false, nil
			}

			// Assert we have the expected annotations.
			wantedAnnotations := map[string]string{
				"components.gke.io/component-name":               "managed_prometheus",
				"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
			}
			if diff := cmp.Diff(wantedAnnotations, ds.Spec.Template.Annotations); diff != "" {
				return false, fmt.Errorf("unexpected annotations (-want, +got): %s", diff)
			}
			return true, nil
		})
		if err != nil {
			t.Errorf("collector DaemonSet is not ready: %s", err)
			out := strings.Builder{}
			if err := kube.Debug(t.Context(), restConfig, kubeClient, &ds, &out); err != nil {
				t.Fatalf("unable to debug: %s", err)
			}
			t.Fatalf("debug:\n%s", out.String())
		}
	}
}

func testCollectorOperatorConfig(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return testCollectorOperatorConfigWithParams(ctx, kubeClient, "external_val", stateEmpty, false)
}

func testCollectorOperatorConfigWithParams(ctx context.Context, kubeClient client.Client, externalKey string, filter filterState, trimScrapeConfigs bool) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking collector is configured")

		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}

		// Test propagation of the custom options.
		config.Collection.Filter = monitoringv1.ExportFilters{
			MatchOneOf: filter.filters(t),
		}
		config.Collection.Compression = monitoringv1.CompressionGzip
		config.Collection.ExternalLabels = map[string]string{
			"external_key": externalKey,
		}

		// Update OperatorConfig.
		if err := kubeClient.Update(ctx, &config); err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		// Check if operator propagates collection options to enhanced fork Prometheus config.
		replace := func(s string) string {
			return strings.NewReplacer(
				"{projectID}", projectID,
				"{location}", location,
				"{cluster}", cluster,
				"{external_key}", externalKey,
				"{exportCredentialsEntry}", func() string {
					if !explicitCredentialsConfigured() {
						return ""
					}
					return fmt.Sprintf(`
        credentials: %s`, collectorExplicitCredentials())
				}(),
				"{expectedMatchEntry}", filter.expectedForkConfigEntry(t),
			).Replace(s)
		}
		want := map[string]string{
			"config.yaml": replace(`global:
    external_labels:
        cluster: {cluster}
        external_key: {external_key}
        location: {location}
        project_id: {projectID}
google_cloud:
    export:
        compression: gzip{exportCredentialsEntry}{expectedMatchEntry}
`),
		}

		var err error
		pollErr := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			cm := corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: operator.DefaultOperatorNamespace,
					Name:      operator.NameCollector,
				},
			}
			if err = kubeClient.Get(ctx, client.ObjectKeyFromObject(&cm), &cm); err != nil {
				if apierrors.IsNotFound(err) {
					return false, nil
				}
				return false, fmt.Errorf("getting collector ConfigMap failed: %w", err)
			}

			got := cm.Data

			// It's impractical to try test complex generated scrape configuration here,
			// so trim scrapeConfigs section to test external labels and forked google cloud section
			// config propagation, unless asked otherwise.
			if trimScrapeConfigs {
				t.Log("comparing configuration without 'scrape_configs' entry")

				gotMap := map[string]any{}
				if err := yaml.Unmarshal([]byte(got["config.yaml"]), &gotMap); err != nil {
					return false, err
				}

				delete(gotMap, "scrape_configs")

				trimmed, err := yaml.Marshal(gotMap)
				if err != nil {
					return false, err
				}
				got["config.yaml"] = string(trimmed)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				err = fmt.Errorf("unexpected collector config entry; diff: %s", diff)
				return false, nil
			}
			return true, nil
		})
		if pollErr != nil {
			if wait.Interrupted(pollErr) {
				pollErr = err
			}
			t.Fatalf("waiting for collector configuration failed: %s", pollErr)
		}
	}
}

type statusFn func(*monitoringv1.ScrapeEndpointStatus) error

// testEnsurePodMonitoringReady sets up a PodMonitoring and ensures its status
// is successfully scraping targets.
func testEnsurePodMonitoringReady(ctx context.Context, kubeClient client.Client, pm *monitoringv1.PodMonitoring) func(*testing.T) {
	return testEnsurePodMonitoringStatus(ctx, kubeClient, pm, isPodMonitoringScrapeEndpointSuccess)
}

// testEnsurePodMonitoringStatus sets up a PodMonitoring and runs validations against
// its status with the provided function.
func testEnsurePodMonitoringStatus(ctx context.Context, kubeClient client.Client, pm *monitoringv1.PodMonitoring, validate statusFn) func(*testing.T) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	return func(t *testing.T) {
		t.Log("ensuring PodMonitoring is created and ready")

		if err := kubeClient.Create(ctx, pm); err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(pm), pm); err != nil {
				return false, fmt.Errorf("getting PodMonitoring failed: %w", err)
			}
			// Ensure no status update cycles.
			// This is not a perfect check as it's possible the get call returns before the operator
			// would sync again, however it can serve as a valuable guardrail in case sporadic test
			// failures start happening due to update cycles.
			if size := len(pm.Status.Conditions); size > 1 {
				return false, fmt.Errorf("status conditions should be of length 1, but got: %d", size)
			}
			// Ensure podmonitoring status shows created configuration.
			if pm.Status.Conditions[0].Type != monitoringv1.ConfigurationCreateSuccess {
				t.Log("status != configuration success")
				return false, nil
			}

			// Check status reflects discovered endpoints.
			if len(pm.Status.EndpointStatuses) < 1 {
				t.Logf("no endpoint statuses yet")
				return false, nil
			}

			// Check target status.
			for _, status := range pm.Status.EndpointStatuses {
				if err := validate(&status); err != nil {
					t.Logf("endpoint status is not valid: %s", err)
					return false, nil
				}
			}
			t.Log("status validated!")
			return true, nil
		})
		if err != nil {
			t.Errorf("unable to validate PodMonitoring status: %s", err)
		}
	}
}

// testEnsureClusterPodMonitoringReady sets up a ClusterPodMonitoring and
// ensures its status is successfully scraping targets.
func testEnsureClusterPodMonitoringReady(ctx context.Context, kubeClient client.Client, cpm *monitoringv1.ClusterPodMonitoring) func(*testing.T) {
	return testEnsureClusterPodMonitoringStatus(ctx, kubeClient, cpm, isPodMonitoringScrapeEndpointSuccess)
}

// testEnsureClusterPodMonitoringStatus sets up a ClusterPodMonitoring and runs
// validations against its status with the provided function.
func testEnsureClusterPodMonitoringStatus(ctx context.Context, kubeClient client.Client, cpm *monitoringv1.ClusterPodMonitoring, validate statusFn) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("ensuring ClusterPodMonitoring is created and ready")

		// The operator should configure the collector to scrape itself and its metrics
		// should show up in Cloud Monitoring shortly after.
		if err := kubeClient.Create(ctx, cpm); err != nil {
			t.Fatalf("create collector ClusterPodMonitoring: %s", err)
		}

		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(cpm), cpm); err != nil {
				return false, fmt.Errorf("getting ClusterPodMonitoring failed: %w", err)
			}
			// Ensure no status update cycles.
			// This is not a perfect check as it's possible the get call returns before the operator
			// would sync again, however it can serve as a valuable guardrail in case sporadic test
			// failures start happening due to update cycles.
			if size := len(cpm.Status.Conditions); size > 1 {
				return false, fmt.Errorf("status conditions should be of length 1, but got: %d", size)
			}

			// Ensure podmonitoring status shows created configuration.
			if cpm.Status.Conditions[0].Type != monitoringv1.ConfigurationCreateSuccess {
				t.Log("status != configuration success")
				return false, nil
			}

			// Check status reflects discovered endpoints.
			if len(cpm.Status.EndpointStatuses) < 1 {
				t.Logf("no endpoint statuses yet")
				return false, nil
			}

			// Check target status.
			for _, status := range cpm.Status.EndpointStatuses {
				if err := validate(&status); err != nil {
					t.Logf("endpoint status is not ready: %s", err)
					return false, nil
				}
			}
			t.Log("status validated!")
			return true, nil
		})
		if err != nil {
			t.Errorf("unable to validate ClusterPodMonitoring status: %s", err)
		}
	}
}

func testEnableTargetStatus(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("enabling target status reporting")

		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			t.Errorf("get operatorconfig: %s", err)
		}
		// Enable target status reporting.
		config.Features.TargetStatus = monitoringv1.TargetStatusSpec{
			Enabled: true,
		}
		if err := kubeClient.Update(ctx, &config); err != nil {
			t.Errorf("updating operatorconfig: %s", err)
		}
	}
}

func testEnableKubeletScraping(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("enabling kubelet scraping")

		config := monitoringv1.OperatorConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      operator.NameOperatorConfig,
				Namespace: operator.DefaultPublicNamespace,
			},
		}
		if err := kubeClient.Get(ctx, client.ObjectKeyFromObject(&config), &config); err != nil {
			t.Errorf("get operatorconfig: %s", err)
		}
		// Enable kubelet scraping.
		// TODO(pintohutch): use ClusterNodeMonitoring instead once TLSInsecureSkipVerify is added there.
		// Since kubelet scraping wont work in kind clusters without this option.
		config.Collection.KubeletScraping = &monitoringv1.KubeletScraping{
			Interval:              "5s",
			TLSInsecureSkipVerify: true,
		}
		if err := kubeClient.Update(ctx, &config); err != nil {
			t.Errorf("updating operatorconfig: %s", err)
		}
	}
}

type listTimeSeriesFilter struct {
	metricType, job, instance, namespace, node, pod, container, externalKey string
}

func (l listTimeSeriesFilter) Filter(t testing.TB) string {
	t.Helper()

	require.NotEmpty(t, l.metricType)
	require.NotEmpty(t, l.job)
	require.NotEmpty(t, l.instance)

	// Validate the majority of labels being set correctly by filtering along them (e.g. project, location, cluster).
	entries := []string{
		`resource.type = "prometheus_target"`,
		fmt.Sprintf(`resource.labels.project_id = "%s"`, projectID),
		fmt.Sprintf(`resource.labels.location = "%s"`, location),
		fmt.Sprintf(`resource.labels.cluster = "%s"`, cluster),
		fmt.Sprintf(`resource.labels.job = "%s"`, l.job),
		fmt.Sprintf(`resource.labels.instance = "%s"`, l.instance),
		fmt.Sprintf(`metric.type = "%s"`, l.metricType),
	}
	if l.pod != "" {
		entries = append(entries, fmt.Sprintf(`metric.labels.pod = "%s"`, l.pod))
	}
	if l.container != "" {
		entries = append(entries, fmt.Sprintf(`metric.labels.container = "%s"`, l.container))
	}
	if l.namespace != "" {
		entries = append(entries, fmt.Sprintf(`resource.labels.namespace = "%s"`, l.namespace))
	}
	if l.node != "" {
		entries = append(entries, fmt.Sprintf(`metric.labels.node = "%s"`, l.node))
	}
	if l.externalKey != "" {
		entries = append(entries, fmt.Sprintf(`metric.labels.external_key = "%s"`, l.externalKey))
	}
	return strings.Join(entries, " AND ")
}

type metricExpectation struct {
	isQueryable  bool
	expectValue1 bool
}

// testValidateGCMMetric checks whether the given metric has expected state on GCM.
func testValidateGCMMetric(ctx context.Context, metricClient *gcm.MetricClient, f listTimeSeriesFilter, expected metricExpectation) func(*testing.T) {
	return func(t *testing.T) {
		filter := f.Filter(t)
		t.Log("checking for metric in Cloud Monitoring", filter)
		if err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			endTime := time.Now() // Always check for fresh data, so we don't have a potential race between collector starting to send data vs this timestamp.

			iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
				Name:   fmt.Sprintf("projects/%s", projectID),
				Filter: filter,
				Interval: &gcmpb.TimeInterval{
					EndTime:   timestamppb.New(endTime),
					StartTime: timestamppb.New(endTime.Add(-10 * time.Second)),
				},
			})
			series, err := iter.Next()
			if errors.Is(err, iterator.Done) {
				if !expected.isQueryable {
					// We expect not having this metric.
					return true, nil
				}
				t.Log("no data in GCM, retrying...")
				return false, nil
			}
			if err != nil {
				return false, fmt.Errorf("querying metrics failed: %w", err)
			}

			if !expected.isQueryable {
				t.Logf("%q is queryable, retrying...", f.metricType)
				return false, nil
			}

			if expected.expectValue1 {
				if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
					t.Logf("%q has unexpected value %v (expected: %v), retrying...", f.metricType, v, 1)
					return false, nil
				}
			}

			// We expect exactly one result.
			series, err = iter.Next()
			if !errors.Is(err, iterator.Done) {
				return false, fmt.Errorf("expected iterator to be done but got series %v: %w", series, err)
			}
			return true, nil
		}); err != nil {
			t.Fatalf("waiting for collector metric to appear in GCM failed: %s; filter: %v", err, filter)
		}
	}
}

// testValidateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func testValidateCollectorUpMetrics(ctx context.Context, kubeClient client.Client, job string) func(*testing.T) {
	return func(t *testing.T) {
		// Wait for metric data to show up in Cloud Monitoring.
		metricClient, err := newMetricClient(ctx)
		if err != nil {
			t.Fatalf("create metric client: %s", err)
		}
		defer metricClient.Close()

		nodes := corev1.NodeList{}
		if err := kubeClient.List(ctx, &nodes); err != nil {
			t.Fatalf("list nodes: %s", err)
		}
		if len(nodes.Items) == 0 {
			t.Fatal("expected more than 0 nodes in the cluster")
		}

		pods := corev1.PodList{}
		if err = kubeClient.List(ctx, &pods, client.InNamespace(operator.DefaultOperatorNamespace), &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(map[string]string{
				operator.LabelAppName: operator.NameCollector,
			}),
		}); err != nil {
			t.Fatalf("list collector pods: %s", err)
		}
		if got, want := len(pods.Items), len(nodes.Items); got != want {
			t.Fatalf("expected %v collector pods, got %v", want, got)
		}

		// See whether the `up` metric is written for each pod/port combination. It is set to 1 by
		// Prometheus on successful scraping of the target. Thereby we validate service discovery
		// configuration, config reload handling, as well as data export are correct.
		//
		// Make a single query for each pod/port combo as this is simpler than untangling the result
		// of a single query.
		for _, pod := range pods.Items {
			for _, port := range []string{operator.CollectorPrometheusContainerPortName, operator.CollectorConfigReloaderContainerPortName} {
				instance := fmt.Sprintf("%s:%s", pod.Spec.NodeName, port)
				t.Run(instance, testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:  "prometheus.googleapis.com/up/gauge",
					job:         job,
					instance:    instance,
					pod:         pod.Name,
					externalKey: "external_val",
					namespace:   operator.DefaultOperatorNamespace,
				}, metricExpectation{isQueryable: true, expectValue1: true}))
			}
		}
	}
}

// testCollectorScrapeKubelet verifies that kubelet metric endpoints are successfully scraped.
func testCollectorScrapeKubelet(ctx context.Context, kubeClient client.Client) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking for metrics in Cloud Monitoring")

		// Wait for metric data to show up in Cloud Monitoring.
		metricClient, err := newMetricClient(ctx)
		if err != nil {
			t.Fatalf("create GCM metric client: %s", err)
		}
		defer metricClient.Close()

		nodes := corev1.NodeList{}
		if err := kubeClient.List(ctx, &nodes); err != nil {
			t.Fatalf("list nodes: %s", err)
		}
		if len(nodes.Items) == 0 {
			t.Fatal("expected more than 0 nodes in the cluster")
		}

		for _, node := range nodes.Items {
			for _, port := range []string{"metrics", "cadvisor"} {
				instance := fmt.Sprintf("%s:%s", node.Name, port)
				t.Run(instance, testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:  "prometheus.googleapis.com/up/gauge",
					job:         "kubelet",
					instance:    instance,
					node:        node.Name,
					externalKey: "external_val",
				}, metricExpectation{isQueryable: true, expectValue1: true}))
			}
		}
	}
}

func isPodMonitoringScrapeEndpointSuccess(status *monitoringv1.ScrapeEndpointStatus) error {
	if status.UnhealthyTargets != 0 {
		return fmt.Errorf("unhealthy targets: %d", status.UnhealthyTargets)
	}
	if status.CollectorsFraction != "1" {
		return fmt.Errorf("collectors failed: %s", status.CollectorsFraction)
	}
	if len(status.SampleGroups) == 0 {
		return errors.New("missing sample groups")
	}
	for i, group := range status.SampleGroups {
		if len(group.SampleTargets) == 0 {
			return fmt.Errorf("missing sample targets for group %d", i)
		}
		for _, target := range group.SampleTargets {
			if target.Health != "up" {
				lastErr := "no error reported"
				if target.LastError != nil {
					lastErr = *target.LastError
				}
				return fmt.Errorf("unhealthy target %q at group %d: %s", target.Health, i, lastErr)
			}
		}
	}
	return nil
}
