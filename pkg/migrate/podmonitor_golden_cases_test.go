// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package migrate

import (
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

// Upstream Prometheus Operator Target Release Tag: v0.75.0
// prometheusOperatorPodMonitorTestCases maps exact upstream test scenarios from pkg/prometheus/promcfg_test.go.
var prometheusOperatorPodMonitorTestCases = []struct {
	name  string
	input *monitoringv1.PodMonitor
}{
	// Upstream Function: TestNamespaceSetCorrectlyForPodMonitor
	// Tests: NamespaceSelector scoping and AttachMetadata node configuration.
	{
		name: "podmonitor-namespace-selection",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				NamespaceSelector: monitoringv1.NamespaceSelector{MatchNames: []string{"test"}},
				AttachMetadata:    &monitoringv1.AttachMetadata{Node: ptr.To(true)},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{Port: "web", Interval: "30s"},
				},
			},
		},
	},

	// Upstream Function: TestSettingHonorTimestampsInPodMonitor
	// Tests: HonorTimestamps configuration flag mapping.
	{
		name: "podmonitor-setting-honor-timestamps",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						HonorTimestamps: ptr.To(false),
						Port:            "web",
						Interval:        "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestSettingTrackTimestampsStalenessInPodMonitor
	// Tests: TrackTimestampsStaleness configuration flag handling.
	{
		name: "podmonitor-track-timestamps-staleness",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TrackTimestampsStaleness: ptr.To(false),
						Port:                     "web",
						Interval:                 "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestSettingScrapeProtocolsInPodMonitor
	// Tests: ScrapeProtocols configuration options handling.
	{
		name: "podmonitor-scrape-protocols",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				ScrapeProtocols: []monitoringv1.ScrapeProtocol{
					monitoringv1.ScrapeProtocol("OpenMetricsText1.0.0"),
					monitoringv1.ScrapeProtocol("OpenMetricsText0.0.1"),
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TrackTimestampsStaleness: ptr.To(false),
						Port:                     "web",
						Interval:                 "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodTargetLabelsFromPodMonitor
	// Tests: Custom PodTargetLabels extraction.
	{
		name: "podmonitor-pod-target-labels",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodTargetLabels: []string{"example", "env"},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{Port: "web", Interval: "30s"},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorEndpointFollowRedirects
	// Tests: FollowRedirects configuration settings.
	{
		name: "podmonitor-follow-redirects",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "pod-monitor-ns",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{MatchLabels: map[string]string{"group": "group1"}},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:            "web",
						Interval:        "30s",
						FollowRedirects: ptr.To(false),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPhaseFilter
	// Tests: FilterRunning endpoint-level warning configuration.
	{
		name: "podmonitor-phase-filter",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "default",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						FilterRunning: ptr.To(false),
						Port:          "test",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorEndpointEnableHttp2
	// Tests: EnableHttp2 TLS configuration options.
	{
		name: "podmonitor-http2-enablement",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor1",
				Namespace: "pod-monitor-ns",
				Labels:    map[string]string{"group": "group1"},
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{MatchLabels: map[string]string{"group": "group1"}},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:        "web",
						Interval:    "30s",
						EnableHttp2: ptr.To(true),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with Pod Name.
	{
		name: "podmonitor-port-name",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-port-name",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:       "podname",
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with Pod Port Number.
	{
		name: "podmonitor-port-number-with-name",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-port-num-name",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with TargetPort Int.
	{
		name: "podmonitor-targetport-int",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-targetport-int",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromInt(10240)),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorPortNumber
	// Case: PodMonitor with TargetPort string.
	{
		name: "podmonitor-targetport-string",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "testpodmonitor-targetport-string",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						TargetPort: ptr.To(intstr.FromString("10240")),
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with Match Label Selector.
	{
		name: "podmonitor-selector-match-labels",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "group1",
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with Match Expression Selector.
	{
		name: "podmonitor-selector-match-expressions",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "group",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"group1"},
						},
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},

	// Upstream Function: TestPodMonitorSelectors
	// Case: PodMonitor with selector and match expression selector (multiple criteria).
	{
		name: "podmonitor-selector-match-labels-and-expressions",
		input: &monitoringv1.PodMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "defaultPodMonitor",
				Namespace: "default",
			},
			Spec: monitoringv1.PodMonitorSpec{
				Selector: metav1.LabelSelector{
					MatchLabels: map[string]string{
						"group": "group1",
					},
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "group",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"group2"},
						},
						{
							Key:      "groupb",
							Operator: metav1.LabelSelectorOpDoesNotExist,
						},
					},
				},
				PodMetricsEndpoints: []monitoringv1.PodMetricsEndpoint{
					{
						Port:     "web",
						Interval: "30s",
					},
				},
			},
		},
	},
}
