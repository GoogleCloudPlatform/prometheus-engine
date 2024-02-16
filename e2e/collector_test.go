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
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
)

func TestCollectorPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-deployed", testCollectorDeployed(ctx, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient, opClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, opClient))
	// Self-scrape podmonitoring.
	pm := &monitoringv1.PodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "collector-podmon",
			Namespace: operator.DefaultOperatorNamespace,
		},
		Spec: monitoringv1.PodMonitoringSpec{
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
	t.Run("self-podmonitoring-ready", testEnsurePodMonitoringReady(ctx, opClient, pm))
	if !skipGCM {
		t.Run("self-podmonitoring-gcm", testValidateCollectorUpMetrics(ctx, kubeClient, "collector-podmon"))
	}
}

func TestCollectorClusterPodMonitoring(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-running", testCollectorDeployed(ctx, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient, opClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, opClient))
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
	t.Run("self-clusterpodmonitoring-ready", testEnsureClusterPodMonitoringReady(ctx, opClient, cpm))
	if !skipGCM {
		t.Run("self-clusterpodmonitoring-gcm", testValidateCollectorUpMetrics(ctx, kubeClient, "collector-cmon"))
	}
}

func TestCollectorKubeletScraping(t *testing.T) {
	ctx := context.Background()
	kubeClient, opClient, err := setupCluster(ctx, t)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-deployed", testCollectorDeployed(ctx, kubeClient))
	t.Run("collector-operatorconfig", testCollectorOperatorConfig(ctx, kubeClient, opClient))

	t.Run("enable-kubelet-scraping", testEnableKubeletScraping(ctx, opClient))
	if !skipGCM {
		t.Run("scrape-kubelet", testCollectorScrapeKubelet(ctx, kubeClient))
	}
}

// testCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(ctx context.Context, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking collector is running")

		// Keep checking the state of the collectors until they're running.
		err := wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			ds, err := kubeClient.AppsV1().DaemonSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameCollector, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
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
			t.Fatalf("waiting for collector DaemonSet failed: %s", err)
		}
	}
}

func testCollectorOperatorConfig(ctx context.Context, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking collector is configured")

		// Add some export filters.
		projectFilter := fmt.Sprintf("{project_id='%s'}", projectID)
		locationFilter := fmt.Sprintf("{location=~'%s$'}", location)
		// TODO(pintohutch): remove once we've fixed: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/728.
		kubeletFilter := "{job='kubelet'}"

		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}
		config.Collection.Filter = monitoringv1.ExportFilters{
			MatchOneOf: []string{projectFilter, locationFilter, kubeletFilter},
		}

		// TODO(pintohutch): add external_labels.
		// Update OperatorConfig.
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		// Keep checking the state of the collectors until they're running.
		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			ds, err := kubeClient.AppsV1().DaemonSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameCollector, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				return false, fmt.Errorf("getting collector DaemonSet failed: %w", err)
			}

			// Ensure prometheus container has expected args.
			for _, c := range ds.Spec.Template.Spec.Containers {
				if c.Name != operator.CollectorPrometheusContainerName {
					continue
				}

				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				wantArgs := []string{
					fmt.Sprintf("--export.label.project-id=%q", projectID),
					fmt.Sprintf("--export.label.location=%q", location),
					fmt.Sprintf("--export.label.cluster=%q", cluster),
					fmt.Sprintf("--export.match=%q", projectFilter),
					fmt.Sprintf("--export.match=%q", locationFilter),
					fmt.Sprintf("--export.match=%q", kubeletFilter)}
				gotArgs := getEnvVar(c.Env, "EXTRA_ARGS")
				for _, arg := range wantArgs {
					if !strings.Contains(gotArgs, arg) {
						return false, fmt.Errorf("expected arg %q not found in EXTRA_ARGS: %q", arg, gotArgs)
					}
				}

				return true, nil
			}
			return false, errors.New("no container with name prometheus found")
		})
		if err != nil {
			t.Fatalf("waiting for collector configuration failed: %s", err)
		}
	}
}

type statusFn func(*monitoringv1.ScrapeEndpointStatus) error

// testEnsurePodMonitoringReady sets up a PodMonitoring and ensures its status
// is successfully scraping targets.
func testEnsurePodMonitoringReady(ctx context.Context, opClient versioned.Interface, pm *monitoringv1.PodMonitoring) func(*testing.T) {
	return testEnsurePodMonitoringStatus(ctx, opClient, pm, isPodMonitoringScrapeEndpointSuccess)
}

// testEnsurePodMonitoringStatus sets up a PodMonitoring and runs validations against
// its status with the provided function.
func testEnsurePodMonitoringStatus(ctx context.Context, opClient versioned.Interface, pm *monitoringv1.PodMonitoring, validate statusFn) func(*testing.T) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	return func(t *testing.T) {
		t.Log("ensuring PodMonitoring is created and ready")

		_, err := opClient.MonitoringV1().PodMonitorings(pm.Namespace).Create(ctx, pm, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			pm, err := opClient.MonitoringV1().PodMonitorings(pm.Namespace).Get(ctx, pm.Name, metav1.GetOptions{})
			if err != nil {
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
				err = validate(&status)
				if err != nil {
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
func testEnsureClusterPodMonitoringReady(ctx context.Context, opClient versioned.Interface, cpm *monitoringv1.ClusterPodMonitoring) func(*testing.T) {
	return testEnsureClusterPodMonitoringStatus(ctx, opClient, cpm, isPodMonitoringScrapeEndpointSuccess)
}

// testEnsureClusterPodMonitoringStatus sets up a ClusterPodMonitoring and runs
// validations against its status with the provided function.
func testEnsureClusterPodMonitoringStatus(ctx context.Context, opClient versioned.Interface, cpm *monitoringv1.ClusterPodMonitoring, validate statusFn) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("ensuring ClusterPodMonitoring is created and ready")

		// The operator should configure the collector to scrape itself and its metrics
		// should show up in Cloud Monitoring shortly after.
		_, err := opClient.MonitoringV1().ClusterPodMonitorings().Create(ctx, cpm, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create collector ClusterPodMonitoring: %s", err)
		}

		err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
			cpm, err := opClient.MonitoringV1().ClusterPodMonitorings().Get(ctx, cpm.Name, metav1.GetOptions{})
			if err != nil {
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
				err = validate(&status)
				if err != nil {
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

func testEnableTargetStatus(ctx context.Context, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("enabling target status reporting")

		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Errorf("get operatorconfig: %s", err)
		}
		// Enable target status reporting.
		config.Features.TargetStatus = monitoringv1.TargetStatusSpec{
			Enabled: true,
		}
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Errorf("updating operatorconfig: %s", err)
		}
	}
}

func testEnableKubeletScraping(ctx context.Context, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("enabling kubelet scraping")

		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Errorf("get operatorconfig: %s", err)
		}
		// Enable kubelet scraping.
		// TODO(pintohutch): use NodeMonitoring instead once TLSInsecureSkipVerify is added there.
		// Since kubelet scraping wont work in kind clusters without this option.
		config.Collection.KubeletScraping = &monitoringv1.KubeletScraping{
			Interval:              "5s",
			TLSInsecureSkipVerify: true,
		}
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Errorf("updating operatorconfig: %s", err)
		}
	}
}

// testValidateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func testValidateCollectorUpMetrics(ctx context.Context, kubeClient kubernetes.Interface, job string) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking for metrics in Cloud Monitoring")

		// Wait for metric data to show up in Cloud Monitoring.
		metricClient, err := gcm.NewMetricClient(ctx)
		if err != nil {
			t.Fatalf("create metric client: %s", err)
		}
		defer metricClient.Close()

		pods, err := kubeClient.CoreV1().Pods(operator.DefaultOperatorNamespace).List(ctx, metav1.ListOptions{
			LabelSelector: fmt.Sprintf("%s=%s", operator.LabelAppName, operator.NameCollector),
		})
		if err != nil {
			t.Fatalf("list collector pods: %s", err)
		}

		// See whether the `up` metric is written for each pod/port combination. It is set to 1 by
		// Prometheus on successful scraping of the target. Thereby we validate service discovery
		// configuration, config reload handling, as well as data export are correct.
		//
		// Make a single query for each pod/port combo as this is simpler than untangling the result
		// of a single query.
		for _, pod := range pods.Items {
			for _, port := range []string{operator.CollectorPrometheusContainerPortName, operator.CollectorConfigReloaderContainerPortName} {
				t.Logf("poll 'up' metric for pod %q and port %q", pod.Name, port)

				err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
					now := time.Now()

					// Validate the majority of labels being set correctly by filtering along them.
					iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
						Name: fmt.Sprintf("projects/%s", projectID),
						Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.label.location = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.namespace = "%s" AND
				resource.labels.job = "%s" AND
				resource.labels.instance = "%s:%s" AND
				metric.type = "prometheus.googleapis.com/up/gauge"
				`, projectID, location, cluster, operator.DefaultOperatorNamespace, job, pod.Spec.NodeName, port),
						Interval: &gcmpb.TimeInterval{
							EndTime:   timestamppb.New(now),
							StartTime: timestamppb.New(now.Add(-10 * time.Second)),
						},
					})
					series, err := iter.Next()
					if err == iterator.Done {
						t.Log("no data in GCM, retrying...")
						return false, nil
					} else if err != nil {
						return false, fmt.Errorf("querying metrics failed: %w", err)
					}
					if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
						t.Logf("'up' still %v, retrying...", v)
						return false, nil
					}
					// We expect exactly one result.
					series, err = iter.Next()
					if err != iterator.Done {
						return false, fmt.Errorf("expected iterator to be done but got series %v: %w", series, err)
					}
					return true, nil
				})
				if err != nil {
					t.Fatalf("waiting for collector metrics to appear in GCM failed: %s", err)
				}
			}
		}
	}
}

// testCollectorScrapeKubelet verifies that kubelet metric endpoints are successfully scraped.
func testCollectorScrapeKubelet(ctx context.Context, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking for metrics in Cloud Monitoring")

		// Wait for metric data to show up in Cloud Monitoring.
		metricClient, err := gcm.NewMetricClient(ctx)
		if err != nil {
			t.Fatalf("create GCM metric client: %s", err)
		}
		defer metricClient.Close()

		nodes, err := kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			t.Fatalf("list nodes: %s", err)
		}

		for _, node := range nodes.Items {
			for _, port := range []string{"metrics", "cadvisor"} {
				t.Logf("poll 'up' metric for kubelet on node %q and port %q", node.Name, port)

				err = wait.PollUntilContextCancel(ctx, pollDuration, false, func(ctx context.Context) (bool, error) {
					now := time.Now()

					// Validate the majority of labels being set correctly by filtering along them.
					iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
						Name: fmt.Sprintf("projects/%s", projectID),
						Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.label.location = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.job = "kubelet" AND
				resource.labels.instance = "%s:%s" AND
				metric.type = "prometheus.googleapis.com/up/gauge" AND
				metric.labels.node = "%s"
				`,
							projectID, location, cluster, node.Name, port, node.Name,
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
					if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
						t.Logf("'up' still %v, retrying...", v)
						return false, nil
					}
					// We expect exactly one result.
					series, err = iter.Next()
					if err != iterator.Done {
						return false, fmt.Errorf("expected iterator to be done but got series %v: %w", series, err)
					}
					return true, nil
				})
				if err != nil {
					t.Fatalf("waiting for collector metrics to appear in GCM failed: %s", err)
				}
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

func getEnvVar(evs []corev1.EnvVar, key string) string {
	for _, ev := range evs {
		if ev.Name == key {
			return ev.Value
		}
	}
	return ""
}
