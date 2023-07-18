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
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	gcmpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

func TestCollector(t *testing.T) {
	tctx := newOperatorContext(t)

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("deployed", tctx.subtest(testCollectorDeployed))
	t.Run("self-podmonitoring", tctx.subtest(testCollectorSelfPodMonitoring))
	t.Run("self-clusterpodmonitoring", tctx.subtest(testCollectorSelfClusterPodMonitoring))
	t.Run("scrape-kubelet", tctx.subtest(testCollectorScrapeKubelet))
}

// testCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(ctx context.Context, t *OperatorContext) {
	// Create initial OperatorConfig to trigger deployment of resources.
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
	}
	if gcpServiceAccount != "" {
		opCfg.Collection.Credentials = &v1.SecretKeySelector{
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

	err = wait.Poll(3*time.Second, 3*time.Minute, func() (bool, error) {
		ds, err := t.kubeClient.AppsV1().DaemonSets(t.namespace).Get(ctx, operator.NameCollector, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return false, nil
		} else if err != nil {
			t.Log(fmt.Errorf("getting collector DaemonSet failed: %w", err))
			return false, fmt.Errorf("getting collector DaemonSet failed: %w", err)
		}
		// At first creation the DaemonSet may appear with 0 desired replicas. This should
		// change shortly after.
		if ds.Status.DesiredNumberScheduled == 0 {
			return false, nil
		}

		// TODO(pintohutch): run all tests without skipGCM by providing boilerplate
		// credentials for use in local testing and CI.
		//
		// This is necessary for any e2e tests that don't have access to GCP
		// credentials. We were getting away with this by running on networks
		// with access to the GCE metadata server IP to supply them:
		// https://github.com/googleapis/google-cloud-go/blob/56d81f123b5b4491aaf294042340c35ffcb224a7/compute/metadata/metadata.go#L39
		// However, running without this access (e.g. on Github Actions) causes
		// a failure from:
		// https://cs.opensource.google/go/x/oauth2/+/master:google/default.go;l=155;drc=9780585627b5122c8cc9c6a378ac9861507e7551
		if !skipGCM {
			if ds.Status.NumberReady != ds.Status.DesiredNumberScheduled {
				return false, nil
			}
		}

		// Assert we have the expected annotations.
		wantedAnnotations := map[string]string{
			"components.gke.io/component-name":               "managed_prometheus",
			"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
		}
		if diff := cmp.Diff(wantedAnnotations, ds.Spec.Template.Annotations); diff != "" {
			return false, fmt.Errorf("unexpected annotations (-want, +got): %s", diff)
		}

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
				`--export.match="{job='foo'}"`,
				`--export.match="{__name__=~'up'}"`,
			}
			if gcpServiceAccount != "" {
				wantArgs = append(wantArgs, fmt.Sprintf(`--export.credentials-file="/etc/secrets/secret_%s_user-gcp-service-account_key.json"`, t.pubNamespace))
			}

			if diff := cmp.Diff(strings.Join(wantArgs, " "), getEnvVar(c.Env, "EXTRA_ARGS")); diff != "" {
				t.Log(fmt.Errorf("unexpected flags (-want, +got): %s", diff))
				return false, fmt.Errorf("unexpected flags (-want, +got): %s", diff)
			}
			return true, nil
		}
		t.Log(errors.New("no container with name prometheus found"))
		return false, errors.New("no container with name prometheus found")
	})
	if err != nil {
		t.Fatalf("Waiting for DaemonSet deployment failed: %s", err)
	}
}

// testCollectorSelfPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfPodMonitoring(ctx context.Context, t *OperatorContext) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
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

	_, err := t.operatorClient.MonitoringV1().PodMonitorings(t.namespace).Create(ctx, podmon, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector PodMonitoring: %s", err)
	}
	t.Log("Waiting for PodMonitoring collector-podmon to be processed")

	var resVer = ""
	err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
		pm, err := t.operatorClient.MonitoringV1().PodMonitorings(t.namespace).Get(ctx, "collector-podmon", metav1.GetOptions{})
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
		t.Log("Waiting for up metrics for collector targets")
		validateCollectorUpMetrics(ctx, t, "collector-podmon")
	}
}

// testCollectorSelfClusterPodMonitoring sets up pod monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfClusterPodMonitoring(ctx context.Context, t *OperatorContext) {
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	podmon := &monitoringv1.ClusterPodMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "collector-cmon",
			OwnerReferences: t.ownerReferences,
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

	_, err := t.operatorClient.MonitoringV1().ClusterPodMonitorings().Create(ctx, podmon, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector ClusterPodMonitoring: %s", err)
	}
	t.Log("Waiting for PodMonitoring collector-podmon to be processed")

	var resVer = ""
	err = wait.Poll(time.Second, 1*time.Minute, func() (bool, error) {
		pm, err := t.operatorClient.MonitoringV1().ClusterPodMonitorings().Get(ctx, "collector-cmon", metav1.GetOptions{})
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
		validateCollectorUpMetrics(ctx, t, "collector-cmon")
	}
}

// validateCollectorUpMetrics checks whether the scrape-time up metrics for all collector
// pods can be queried from GCM.
func validateCollectorUpMetrics(ctx context.Context, t *OperatorContext, job string) {
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
		for _, port := range []string{operator.CollectorPrometheusContainerPortName, operator.CollectorConfigReloaderContainerPortName} {
			t.Logf("Poll up metric for pod %q and port %q", pod.Name, port)

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
				metric.type = "prometheus.googleapis.com/up/gauge" AND
				metric.labels.external_key = "external_val"
				`,
						projectID, location, cluster, t.namespace, job, pod.Spec.NodeName, port,
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
					return false, fmt.Errorf("querying metrics failed: %w", err)
				}
				if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
					t.Logf("Up still %v, retrying...", v)
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
				t.Fatalf("Waiting for collector metrics to appear in Cloud Monitoring failed: %s", err)
			}
		}
	}
}

// testCollectorScrapeKubelet verifies that kubelet metric endpoints are successfully scraped.
func testCollectorScrapeKubelet(ctx context.Context, t *OperatorContext) {
	if skipGCM {
		t.Log("Not validating scraping of kubelets when --skip-gcm is set")
		return
	}
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
		t.Fatalf("Create GCM metric client: %s", err)
	}
	defer metricClient.Close()

	nodes, err := t.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("List nodes: %s", err)
	}

	// See whether the `up` metric for both kubelet endpoints is 1 for each node on which
	// a collector pod is running.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	for _, node := range nodes.Items {
		for _, port := range []string{"metrics", "cadvisor"} {
			t.Logf("Poll up metric for kubelet on node %q and port %q", node.Name, port)

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
				metric.labels.external_key = "external_val"
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
					t.Logf("No data, retrying...")
					return false, nil
				} else if err != nil {
					return false, fmt.Errorf("querying metrics failed: %w", err)
				}
				if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
					t.Logf("Up still %v, retrying...", v)
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
				t.Fatalf("Waiting for collector metrics to appear in Cloud Monitoring failed: %s", err)
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
