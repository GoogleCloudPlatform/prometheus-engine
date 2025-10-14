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
	match string
}

var (
	stateEmpty = filterState{}
	stateA     = filterState{match: "{__name__='go_goroutines',container='prometheus'}"}
	stateB     = filterState{match: "{__name__='go_goroutines',container='config-reloader'}"}
)

func (f filterState) expectedForkConfigEntry(t testing.TB) string {
	switch f {
	case stateEmpty:
		return ""
	case stateA:
		return `
        match:
            - '{__name__=''go_goroutines'',container=''prometheus''}'`
	case stateB:
		return `
        match:
            - '{__name__=''go_goroutines'',container=''config-reloader''}'`
	default:
		t.Fatalf("invalid filter state: %s", f)
		return ""
	}
}

func (f filterState) filters(t testing.TB) []string {
	switch f {
	case stateEmpty:
		return nil
	case stateA, stateB:
		return []string{f.match}
	default:
		t.Fatalf("invalid filter state: %s", f)
		return nil
	}
}

// testValidateApplied fails the test if the current filtering state is not applied to "f"
// within the context deadline. This test assumes:
// * collectors are running.
// * collectorPodMonitoring is applied.
// * prometheus and config-reloader expose 'go_goroutines' metric.
// * OperatorConfig as "external_key"=$externalKey label configured (as well as default ones like project, etc.).
func (f filterState) testValidateApplied(ctx context.Context, kubeClient client.Client, externalKey string) func(*testing.T) {
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
				var promMatch, configReloaderMatch bool
				switch f {
				case stateEmpty:
					promMatch = true
					configReloaderMatch = true
				case stateA:
					promMatch = true
					configReloaderMatch = false
				case stateB:
					promMatch = false
					configReloaderMatch = true
				default:
					t.Fatalf("invalid filter state: %s", f)
				}

				t.Run("prometheus", testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:  "prometheus.googleapis.com/go_goroutines/gauge",
					job:         collectorPodMonitoring.Name,
					instance:    fmt.Sprintf("%s:%s", pod.Spec.NodeName, operator.CollectorPrometheusContainerPortName),
					pod:         pod.Name,
					container:   "prometheus",
					externalKey: externalKey,
					namespace:   operator.DefaultOperatorNamespace,
				}, metricExpectation{isQueryable: promMatch}))

				t.Run("config-reloader", testValidateGCMMetric(ctx, metricClient, listTimeSeriesFilter{
					metricType:  "prometheus.googleapis.com/go_goroutines/gauge",
					job:         collectorPodMonitoring.Name,
					instance:    fmt.Sprintf("%s:%s", pod.Spec.NodeName, operator.CollectorConfigReloaderContainerPortName),
					pod:         pod.Name,
					container:   "config-reloader",
					externalKey: externalKey,
					namespace:   operator.DefaultOperatorNamespace,
				}, metricExpectation{isQueryable: configReloaderMatch}))
			})
		}
	}
}

type filterCase struct {
	filter         filterState
	expectedFilter filterState // What we expect to be applied.
}

// Regression tests against go/gmp:matchstuck.
// See go/gmp:matchstuck for 0, A, B, C case definition.
func TestCollectorMatch0toACase(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}
	testCollectorMatch(t, stateEmpty, []filterCase{
		// 0
		{
			filter:         stateEmpty,
			expectedFilter: stateEmpty,
		},
		// A
		{
			filter: stateA,
			// Given the go/gmp:matchstuck we expect the noop behaviour.
			expectedFilter: stateEmpty, // TODO: Add fix, so it's stateA (when forced).
		},
		{
			filter: stateB,
			// Given the go/gmp:matchstuck we expect the noop behaviour.
			expectedFilter: stateEmpty, // TODO: Add fix, so it's stateB (when forced).
		},
		{
			filter:         stateEmpty,
			expectedFilter: stateEmpty,
		},
	})
}

// Regression tests against go/gmp:matchstuck.
// See go/gmp:matchstuck for 0, A, B, C case definition.
func TestCollectorMatchBtoCCase(t *testing.T) {
	if skipGCM {
		t.Skip("this test requires GCM integration")
	}
	testCollectorMatch(t, stateB, []filterCase{
		{
			filter: stateA,
			// Given the go/gmp:matchstuck we expect the orphaned setting applied.
			expectedFilter: stateB, // TODO: Add fix, so it's stateA (when forced).
		},
		// B-2
		{
			filter: stateEmpty,
			// Given the go/gmp:matchstuck we expect the orphaned setting applied.
			expectedFilter: stateB, // TODO: Add fix, `so it's stateEmpty (when forced).
		},
		// C
		{
			filter:         stateB,
			expectedFilter: stateB,
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
		dOpts = append(dOpts, deploy.WithExplicitCollectorFilter(explicitFilter.match))
	}
	kubeClient, restConfig, err := setupCluster(ctx, t, dOpts...)
	if err != nil {
		t.Fatalf("error instantiating clients. err: %s", err)
	}

	t.Run("collector-deployed", testCollectorDeployed(ctx, restConfig, kubeClient))
	t.Run("enable-target-status", testEnableTargetStatus(ctx, kubeClient))
	t.Run("self-podmonitoring-ready", testEnsurePodMonitoringReady(ctx, kubeClient, collectorPodMonitoring))

	for i, fcase := range filterCases {
		// Ensure a unique external label value so we are sure the existence checks are accurate.
		externalKey := fmt.Sprintf("filter%d", i)

		// Setup OperatorConfig with an intput filtering state (filter.matchOneOf).
		t.Run("collector-operatorconfig", testCollectorOperatorConfigWithParams(
			ctx,
			kubeClient,
			externalKey,
			fcase.filter,
			true, // Trim scrapeConfigs from diff chceck.
		))
		t.Run("filter-applied-gcm", fcase.expectedFilter.testValidateApplied(ctx, kubeClient, externalKey))
	}
}
