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

package v1alpha1

import (
	"reflect"
	"strings"
	"testing"

	prommodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/relabel"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestValidatePodMonitoring(t *testing.T) {
	cases := []struct {
		desc        string
		pm          PodMonitoringSpec
		fail        bool
		errContains string
	}{
		{
			desc: "OK",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
					},
					{
						Port:     intstr.FromInt(8080),
						Interval: "1000ms",
						Timeout:  "5s",
					},
				},
				TargetLabels: TargetLabels{
					FromPod: []LabelMapping{
						{From: "key1", To: "key2"},
						{From: "key3"},
					},
				},
			},
		}, {
			desc: "port missing",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{Interval: "10s"},
				},
			},
			fail:        true,
			errContains: "port must be set",
		}, {
			desc: "scrape interval missing",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{Port: intstr.FromString("web")},
				},
			},
			fail:        true,
			errContains: "invalid scrape interval: empty duration string",
		}, {
			desc: "scrape interval malformed",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "foo",
					},
				},
			},
			fail:        true,
			errContains: "invalid scrape interval: not a valid duration string",
		}, {
			desc: "scrape timeout malformed",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "1s",
						Timeout:  "_",
					},
				},
			},
			fail:        true,
			errContains: "invalid scrape timeout: not a valid duration string",
		}, {
			desc: "remapping onto prometheus_target label",
			pm: PodMonitoringSpec{
				Endpoints: []ScrapeEndpoint{
					{
						Port:     intstr.FromString("web"),
						Interval: "10s",
					},
				},
				TargetLabels: TargetLabels{
					FromPod: []LabelMapping{
						{From: "key1", To: "cluster"},
					},
				},
			},
			fail:        true,
			errContains: "conflicts with GMP target schema",
		},
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			pm := &PodMonitoring{
				Spec: c.pm,
			}
			err := pm.ValidateCreate()
			t.Log(err)

			if err == nil && c.fail {
				t.Fatalf("expected failure but passed")
			}
			if err != nil && !c.fail {
				t.Fatalf("unexpected failure: %s", err)
			}
			if err != nil && c.fail && !strings.Contains(err.Error(), c.errContains) {
				t.Fatalf("expected error to contain %q but got %q", c.errContains, err)
			}
		})
	}
}

func TestLabelMappingRelabelConfigs(t *testing.T) {
	cases := []struct {
		doc      string
		mappings []LabelMapping
		expected []*relabel.Config
		expErr   bool
	}{
		{
			doc:      "good podmonitoring relabel",
			mappings: []LabelMapping{{From: "from", To: "to"}},
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_from"},
				TargetLabel:  "to",
			}},
			expErr: false,
		},
		{
			doc:      "colliding podmonitoring relabel",
			mappings: []LabelMapping{{From: "from-instance", To: "instance"}},
			expected: nil,
			expErr:   true,
		},
		{
			doc: "both good and colliding podmonitoring relabel",
			mappings: []LabelMapping{
				{From: "from", To: "to"},
				{From: "from-instance", To: "instance"}},
			expected: nil,
			expErr:   true,
		},
		{
			doc:      "empty to podmonitoring relabel",
			mappings: []LabelMapping{{From: "from"}},
			expected: []*relabel.Config{{
				Action:       relabel.Replace,
				SourceLabels: prommodel.LabelNames{"__meta_kubernetes_pod_label_from"},
				TargetLabel:  "from",
			}},
			expErr: false,
		},
	}

	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			// If we get an error when we don't expect, fail test.
			actual, err := labelMappingRelabelConfigs(c.mappings, "__meta_kubernetes_pod_label_")
			if err != nil && !c.expErr {
				t.Errorf("returned unexpected error: %s", err)
			}
			if err == nil && c.expErr {
				t.Errorf("should have returned an error")
			}
			if !reflect.DeepEqual(c.expected, actual) {
				t.Errorf("returned unexpected config")
			}
		})
	}
}
