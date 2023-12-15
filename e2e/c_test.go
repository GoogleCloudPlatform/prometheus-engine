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

	// Blank import required to register GCP auth handlers to talk to GKE clusters.

	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
)

func TestCollector(t *testing.T) {
	kubeClient, opClient, err := newKubeContexts()
	if err != nil {
		t.Errorf("error instantiating clients. err: %s", err)
	}
	ctx := context.Background()

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("collector-running", testCollectorRunning(ctx, t, kubeClient))
	t.Run("collector-configured", testCollectorConfigured(ctx, t, kubeClient, opClient))
	t.Run("self-podmonitoring", testCollectorSelfPodMonitoring(ctx, t, kubeClient, opClient))
	t.Run("self-clusterpodmonitoring", testCollectorSelfClusterPodMonitoring(ctx, t, kubeClient, opClient))
	t.Run("scrape-kubelet", testCollectorScrapeKubelet(ctx, t, kubeClient, opClient))
}

// testCollectorRunning does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorRunning(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("checking collectors and configuration")

		// Keep checking the state of the collectors until they're running.
		err := wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
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

func testCollectorConfigured(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		t.Log("updating OperatorConfig fields")
		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}
		// Add some export filters.
		projectFilter := fmt.Sprintf("{project_id='%s'}", projectID)
		locationFilter := fmt.Sprintf("{location=~'%s$'}", location)
		// TODO(pintohutch): remove once we've fixed: https://github.com/GoogleCloudPlatform/prometheus-engine/issues/728.
		kubeletFilter := "{job='kubelet'}"
		config.Collection.Filter = monitoringv1.ExportFilters{
			MatchOneOf: []string{projectFilter, locationFilter, kubeletFilter},
		}
		// Enable kubelet scraping.
		config.Collection.KubeletScraping = &monitoringv1.KubeletScraping{
			Interval: "5s",
		}
		// Update OperatorConfig.
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

		// Keep checking the state of the collectors until they're running.
		err = wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
			ds, err := kubeClient.AppsV1().DaemonSets(operator.DefaultOperatorNamespace).Get(ctx, operator.NameCollector, metav1.GetOptions{})
			if apierrors.IsNotFound(err) {
				return false, nil
			} else if err != nil {
				t.Log(fmt.Errorf("getting collector DaemonSet failed: %w", err))
				return false, fmt.Errorf("getting collector DaemonSet failed: %w", err)
			}

			for _, c := range ds.Spec.Template.Spec.Containers {
				if c.Name != operator.CollectorPrometheusContainerName {
					continue
				}

				// We're mainly interested in the dynamic flags but checking the entire set including
				// the static ones is ultimately simpler.
				wantArgs := []string{projectFilter, locationFilter, kubeletFilter}
				gotArgs := getEnvVar(c.Env, "EXTRA_ARGS")
				for _, arg := range wantArgs {
					if !strings.Contains(gotArgs, arg) {
						return false, fmt.Errorf("expected arg %q not found in EXTRA_ARGS: %q", arg, gotArgs)
					}
				}

				return true, nil
			}
			t.Log(errors.New("no container with name prometheus found"))
			return false, errors.New("no container with name prometheus found")
		})
		if err != nil {
			t.Fatalf("waiting for collector configuration failed: %s", err)
		}
	}
}

// testCollectorSelfPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfPodMonitoring(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	return func(t *testing.T) {
		podmon := &monitoringv1.PodMonitoring{
			ObjectMeta: metav1.ObjectMeta{
				Name: "collector-podmon",
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

		_, err := opClient.MonitoringV1().PodMonitorings(operator.DefaultOperatorNamespace).Create(ctx, podmon, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create collector PodMonitoring: %s", err)
		}

		t.Log("waiting for PodMonitoring collector-podmon to be processed")
		var resVer = ""
		err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
			pm, err := opClient.MonitoringV1().PodMonitorings(operator.DefaultOperatorNamespace).Get(ctx, "collector-podmon", metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("getting PodMonitoring failed: %w", err)
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
				success := pm.Status.Conditions[0].Type == monitoringv1.ConfigurationCreateSuccess
				steadyVer := resVer == pm.ResourceVersion
				return success && steadyVer, nil
			} else if size > 1 {
				return false, fmt.Errorf("status conditions should be of length 1, but got: %d", size)
			}
			return false, nil
		})
		if err != nil {
			t.Errorf("unable to validate PodMonitoring status: %s", err)
		}

		if !skipGCM {
			validateCollectorUpMetrics(ctx, t, kubeClient, "collector-podmon")
		}
	}
}

// testCollectorSelfClusterPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfClusterPodMonitoring(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		// The operator should configure the collector to scrape itself and its metrics
		// should show up in Cloud Monitoring shortly after.
		podmon := &monitoringv1.ClusterPodMonitoring{
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

		_, err := opClient.MonitoringV1().ClusterPodMonitorings().Create(ctx, podmon, metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("create collector ClusterPodMonitoring: %s", err)
		}
		t.Log("Waiting for ClusterPodMonitoring collector-cmon to be processed")

		var resVer = ""
		err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
			pm, err := opClient.MonitoringV1().ClusterPodMonitorings().Get(ctx, "collector-cmon", metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("getting ClusterPodMonitoring failed: %w", err)
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
				success := pm.Status.Conditions[0].Type == monitoringv1.ConfigurationCreateSuccess
				steadyVer := resVer == pm.ResourceVersion
				return success && steadyVer, nil
			} else if size > 1 {
				return false, fmt.Errorf("status conditions should be of length 1, but got: %d", size)
			}
			return false, nil
		})
		if err != nil {
			t.Errorf("unable to validate ClusterPodMonitoring status: %s", err)
		}

		if !skipGCM {
			t.Log("Waiting for up metrics for collector targets")
			validateCollectorUpMetrics(ctx, t, kubeClient, "collector-cmon")
		}
	}
}

// validateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func validateCollectorUpMetrics(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, job string) {
	// The project, location, and cluster name in which we look for the metric data must
	// be provided by the user. Check this only in this test so tests that don't need these
	// flags can still be run without them.
	// They can be configured on the operator but our current test setup (targeting GKE)
	// relies on the operator inferring them from the environment.
	if projectID == "" {
		t.Fatalf("no project specified (--project-id flag)")
	}
	if location == "" {
		t.Fatalf("no location specified (--location flag)")
	}
	if cluster == "" {
		t.Fatalf("no cluster name specified (--cluster flag)")
	}

	// Wait for metric data to show up in Cloud Monitoring.
	metricClient, err := gcm.NewMetricClient(ctx)
	if err != nil {
		t.Fatalf("create GCM metric client: %s", err)
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
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	for _, pod := range pods.Items {
		for _, port := range []string{operator.CollectorPrometheusContainerPortName, operator.CollectorConfigReloaderContainerPortName} {
			t.Logf("poll up metric for pod %q and port %q", pod.Name, port)

			err = wait.PollImmediateUntil(3*time.Second, func() (bool, error) {
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
			}, ctx.Done())
			if err != nil {
				t.Fatalf("waiting for collector metrics to appear in GCM failed: %s", err)
			}
		}
	}
}

// testCollectorScrapeKubelet verifies that kubelet metric endpoints are successfully scraped.
func testCollectorScrapeKubelet(ctx context.Context, t *testing.T, kubeClient kubernetes.Interface, opClient versioned.Interface) func(*testing.T) {
	return func(t *testing.T) {
		if skipGCM {
			t.Log("not validating scraping of kubelets when --skip-gcm is set")
			return
		}

		t.Log("updating OperatorConfig fields")
		config, err := opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Get(ctx, operator.NameOperatorConfig, metav1.GetOptions{})
		if err != nil {
			t.Fatalf("get operatorconfig: %s", err)
		}
		// Enable kubelet scraping.
		config.Collection.KubeletScraping = &monitoringv1.KubeletScraping{
			Interval: "5s",
		}
		// Update OperatorConfig.
		_, err = opClient.MonitoringV1().OperatorConfigs(operator.DefaultPublicNamespace).Update(ctx, config, metav1.UpdateOptions{})
		if err != nil {
			t.Fatalf("update operatorconfig: %s", err)
		}

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

		// See whether the `up` metric for both kubelet endpoints is 1 for each node on which
		// a collector pod is running.
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		for _, node := range nodes.Items {
			for _, port := range []string{"metrics", "cadvisor"} {
				t.Logf("poll up metric for kubelet on node %q and port %q", node.Name, port)

				err = wait.PollImmediateUntil(3*time.Second, func() (bool, error) {
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
				}, ctx.Done())
				if err != nil {
					t.Fatalf("waiting for collector metrics to appear in GCM failed: %s", err)
				}
			}
		}
	}
}

func getEnvVar(evs []corev1.EnvVar, key string) string {
	for _, ev := range evs {
		if ev.Name == key {
			return ev.Value
		}
	}
	return ""
}
