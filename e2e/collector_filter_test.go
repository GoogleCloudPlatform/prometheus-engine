// Copyright 2025 Google LLC
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
	"fmt"
	"testing"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deploy"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"google.golang.org/protobuf/proto"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var collectorPodMonitoring = &monitoringv1.PodMonitoring{
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

type filterState struct {
	container string
}

var (
	stateEmpty = filterState{}
	stateA     = filterState{container: "prometheus"}
	stateB     = filterState{container: "config-reloader"}
)

// expectedCollectorExportConfigEntry returns expected prometheus config.GoogleCloudExportConfig
// serialized entry for the given filter state.
func (f filterState) expectedCollectorExportConfigEntry(enabled *bool) string {
	var entry string
	switch {
	case enabled == nil:
		return ""
	case *enabled:
		entry += `
        enable_match: true`
	case !*enabled:
		entry += `
        enable_match: false`
	default:
		panic("unexpected enabled state")
	}

	// We only add matchers if it's enabled, no point doing this otherwise.
	if *enabled && f.container != "" {
		entry += fmt.Sprintf(`
        match:
            - '{__name__=''go_goroutines'',container=''%s''}'`, f.container)
	}
	return entry
}

func (f filterState) toMatcher() string {
	if f.container != "" {
		return fmt.Sprintf("{__name__='go_goroutines',container='%s'}", f.container)
	}
	return ""
}

// testFiltering fails the test if the current filtering state is applied by
// querying GCM expecting only f.container metrics to be present. If f.container
// is empty, it means GCM should see both container's metrics.
//
// This test assumes:
// * collectors are running.
// * collectorPodMonitoring is applied.
// * prometheus and config-reloader expose 'go_goroutines' metric.
// * OperatorConfig as "external_key"=$externalValue label configured (on top default labels like project, etc.).
func (f filterState) testFiltering(ctx context.Context, kubeClient client.Client, externalValue string) func(*testing.T) {
	return func(t *testing.T) {
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

		for _, pod := range pods.Items {
			t.Run(pod.Name, func(t *testing.T) {
				t.Run("prometheus", testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:    "prometheus.googleapis.com/go_goroutines/gauge",
					job:           collectorPodMonitoring.Name,
					instance:      fmt.Sprintf("%s:%s", pod.Spec.NodeName, operator.CollectorPrometheusContainerPortName),
					pod:           pod.Name,
					container:     "prometheus",
					externalValue: externalValue,
					namespace:     operator.DefaultOperatorNamespace,
				}, metricExpectation{noPoints: f.container != "" && f.container != "prometheus"}))

				t.Run("config-reloader", testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:    "prometheus.googleapis.com/go_goroutines/gauge",
					job:           collectorPodMonitoring.Name,
					instance:      fmt.Sprintf("%s:%s", pod.Spec.NodeName, operator.CollectorConfigReloaderContainerPortName),
					pod:           pod.Name,
					container:     "config-reloader",
					externalValue: externalValue,
					namespace:     operator.DefaultOperatorNamespace,
				}, metricExpectation{noPoints: f.container != "" && f.container != "config-reloader"}))
			})
		}
	}
}

type filterCase struct {
	name             string
	filter           filterState // OperatorConfig.collection.filter.matchOneOf.
	enableMatchOneOf *bool       // Opt in flag, so OperatorConfig.collection.filter.enableMatchOneOf.

	expectedFilter filterState // What we expect to be applied.
}

// Regression tests against go/gmp:matchstuck.
// NOTE: TestCollectorMatch_NoFiltering takes ~1m per case, add sequential cases carefully.
func TestCollectorMatch_NoFiltering(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}
	testCollectorMatch(t, stateEmpty, []filterCase{
		{
			name:   "no filtering",
			filter: stateEmpty,

			expectedFilter: stateEmpty,
		},
		{
			name:             "no filtering/enable=true",
			filter:           stateEmpty,
			enableMatchOneOf: proto.Bool(true),

			expectedFilter: stateEmpty,
		},
		{
			name:             "no filtering/enable=false",
			filter:           stateEmpty,
			enableMatchOneOf: proto.Bool(false),

			expectedFilter: stateEmpty,
		},
	})
}

// Regression tests against go/gmp:matchstuck.
// NOTE: TestCollectorMatch_NewFilter takes ~1m per case, add sequential cases carefully.
func TestCollectorMatch_NewFilter(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}
	testCollectorMatch(t, stateEmpty, []filterCase{
		{
			name:   "filtering stuck",
			filter: stateA,

			// Given the go/gmp:matchstuck we expect the noop behaviour, without opt-in.
			expectedFilter: stateEmpty,
		},
		{
			name:             "filtering/enable=true",
			filter:           stateA,
			enableMatchOneOf: proto.Bool(true),

			expectedFilter: stateA,
		},
		{
			name:             "filtering/enable=false",
			filter:           stateA,
			enableMatchOneOf: proto.Bool(false),

			expectedFilter: stateEmpty,
		},
	})
}

// Regression tests against go/gmp:matchstuck.
// NOTE: TestCollectorMatch_NewFilter_ThenRemoved takes ~1m per case, add sequential cases carefully.
func TestCollectorMatch_NewFilter_ThenRemoved(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}
	testCollectorMatch(t, stateEmpty, []filterCase{
		{
			name:             "filtering/enable=true",
			filter:           stateA,
			enableMatchOneOf: proto.Bool(true),

			expectedFilter: stateA,
		},
		{
			name:   "filtering stuck again",
			filter: stateA,

			// Given the go/gmp:matchstuck we expect the noop behaviour.
			expectedFilter: stateEmpty,
		},
		{
			name:           "no filtering again",
			filter:         stateEmpty,
			expectedFilter: stateEmpty,
		},
	})
}

// Regression tests against go/gmp:matchstuck.
// NOTE: TestCollectorMatch_StuckFilter takes some time per case, add cases carefully.
func TestCollectorMatch_StuckFilter(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}

	// --export.match=stateB.
	testCollectorMatch(t, stateB, []filterCase{
		{
			name:   "filtering stuck",
			filter: stateA,

			// Given the go/gmp:matchstuck we expect the orphaned setting applied.
			expectedFilter: stateB,
		},
		{
			name:             "filtering/enable=true",
			filter:           stateA,
			enableMatchOneOf: proto.Bool(true),

			expectedFilter: stateA,
		},
		{
			name:             "filtering/enable=disabled",
			filter:           stateA,
			enableMatchOneOf: proto.Bool(false),

			expectedFilter: stateEmpty,
		},
	})
}

// testCollectorMatch allows testing OperatorConfig.collection.filter.matchOneOf setting.
// NOTE: This test does not intend to test detailed collector match filtering cases, those should
// be tested on collector side. What we test here is if filtering is applied correctly
// in general and in the event of the orphaned extra args.
func testCollectorMatch(t *testing.T, explicitFilter filterState, filterCases []filterCase) {
	ctx := contextWithDeadline(t)

	var dOpts []deploy.DeployOption
	if explicitFilter != stateEmpty {
		dOpts = append(dOpts, deploy.WithExplicitCollectorFilter(explicitFilter.toMatcher()))
	}
	kubeClient, restConfig, err := setupCluster(ctx, t, dOpts...)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("collector-deployed", testCollectorDeployed(ctx, restConfig, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, kubeClient))
	t.Run("self-podmonitoring-ready", testEnsurePodMonitoringReady(ctx, kubeClient, collectorPodMonitoring))

	for i, fcase := range filterCases {
		t.Run(fcase.name, func(t *testing.T) {
			// Ensure a unique external label value so we are sure the existence checks are accurate.
			externalValue := fmt.Sprintf("filter%d", i)

			// Setup OperatorConfig with an input filtering state (filter.matchOneOf).
			t.Run("collector-operatorconfig", testCollectorOperatorConfigWithParams(
				ctx,
				kubeClient,
				externalValue,
				fcase.filter,
				fcase.enableMatchOneOf,
			))
			t.Run("filter-applied-gcm", fcase.expectedFilter.testFiltering(ctx, kubeClient, externalValue))
		})
	}
}
