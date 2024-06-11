// Copyright 2024 Google LLC
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
	"testing"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestParseFilter(t *testing.T) {
	point1 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 2},
			EndTime:   &timestamppb.Timestamp{Seconds: 4},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 9.8},
		},
	}

	point2 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 6},
			EndTime:   &timestamppb.Timestamp{Seconds: 8},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.5},
		},
	}

	timeSeries := &monitoringpb.TimeSeries{
		Resource: &monitoredrespb.MonitoredResource{
			Type: "prometheus_target",
			Labels: map[string]string{
				"project_id": "example-project",
				"location":   "europe",
				"cluster":    "foo-cluster",
				"namespace":  "",
				"job":        "job1",
				"instance":   "instance1",
			},
		},
		Metric: &metricpb.Metric{
			Type: "prometheus.googleapis.com/metric1/gauge",
			Labels: map[string]string{
				"foo": "bar",
			},
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points:     []*monitoringpb.Point{point1, point2},
	}

	testCases := []struct {
		name        string
		filter      string
		expected    []*monitoringpb.Point
		errExpected bool
	}{
		{
			name:        "empty",
			filter:      "",
			errExpected: true,
		},
		{
			name:        "missing quotes",
			filter:      "project = example-project",
			errExpected: true,
		},
		{
			name:        "invalid object",
			filter:      "project_id = example-project",
			errExpected: true,
		},
		{
			name:        "invalid operation",
			filter:      "project_id == example-project",
			errExpected: true,
		},
		{
			name:     "right project",
			filter:   `project = "example-project"`,
			expected: timeSeries.Points,
		},
		{
			name:   "wrong project",
			filter: `project = "example"`,
		},
		{
			name:     "right metric type",
			filter:   `metric.type = "prometheus.googleapis.com/metric1/gauge"`,
			expected: timeSeries.Points,
		},
		{
			name:   "wrong metric type",
			filter: `metric.type = "prometheus.googleapis.com/up/gauge"`,
		},
		{
			name:     "right resource type",
			filter:   `resource.type = "prometheus_target"`,
			expected: timeSeries.Points,
		},
		{
			name:   "wrong resource type",
			filter: `resource.type = "prometheus"`,
		},
		{
			name:     "right metric label value",
			filter:   `metric.labels.foo = "bar"`,
			expected: timeSeries.Points,
		},
		{
			name:   "wrong metric label value",
			filter: `metric.labels.foo = "baz"`,
		},
		{
			name:   "missing metric label",
			filter: `metric.labels.bar = "foo"`,
		},
		{
			name:     "right resource label value",
			filter:   `resource.labels.project_id = "example-project"`,
			expected: timeSeries.Points,
		},
		{
			name:   "wrong resource label value",
			filter: `resource.labels.project_id = "example"`,
		},
		{
			name:   "missing resource label",
			filter: `resource.labels.project = "example-project"`,
		},
		{
			name:     "and expression both true",
			filter:   `project = "example-project" AND resource.type = "prometheus_target"`,
			expected: timeSeries.Points,
		},
		{
			name:   "and expression left true",
			filter: `project = "example-project" AND resource.type = "prometheus"`,
		},
		{
			name:   "and expression right true",
			filter: `project = "example" AND resource.type = "prometheus_target"`,
		},
		{
			name:   "and expression none true",
			filter: `project = "example" AND resource.type = "prometheus"`,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f, err := parseFilter(tc.filter)
			if err != nil {
				if tc.errExpected {
					return
				}
				t.Fatal("parse filter failed", err)
			} else if tc.errExpected {
				t.Fatal("expected parse error", tc.filter)
			}
			var points []*monitoringpb.Point
			for _, p := range timeSeries.Points {
				if f.filter(timeSeries, p) {
					points = append(points, p)
				}
			}
			if diff := cmp.Diff(tc.expected, points, protocmp.Transform()); diff != "" {
				t.Fatalf("expected points (-want, +got) %s", diff)
			}
		})
	}
}
