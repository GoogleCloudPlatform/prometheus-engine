// Copyright 2020 Google LLC
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

package export

import (
	"fmt"
	"math"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/anypb"

	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	distribution_pb "google.golang.org/genproto/googleapis/api/distribution"
	metric_pb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	timestamp_pb "google.golang.org/protobuf/types/known/timestamppb"
)

type seriesMap map[storage.SeriesRef]labels.Labels
type metricMetadataMap map[string]MetricMetadata

func testMetadataFunc(metadata metricMetadataMap) MetadataFunc {
	return func(metric string) (MetricMetadata, bool) {
		md, ok := metadata[metric]
		md.Metric = metric
		return md, ok
	}
}

func wrapAsAny(any proto.Message) *anypb.Any {
	result, err := anypb.New(any)
	if err != nil {
		panic(err)
	}
	return result
}

func TestSampleBuilder(t *testing.T) {
	externalLabels := labels.FromMap(map[string]string{
		"project_id": "example-project",
		"location":   "europe",
		"cluster":    "foo-cluster",
	})

	cases := []struct {
		doc        string
		metadata   MetadataFunc
		series     seriesMap
		samples    [][]record.RefSample
		exemplars  []map[storage.SeriesRef]record.RefExemplar
		matchers   Matchers
		wantSeries []*monitoring_pb.TimeSeries
		wantFail   bool
	}{
		{
			doc: "convert gauge",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeGauge, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 123, T: 3000, V: 0.6}},
				{{Ref: 123, T: 4000, V: math.Inf(1)}},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/gauge",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 3},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 0.6},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/gauge",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: math.Inf(1)},
						},
					}},
				},
			},
		}, {
			doc: "convert untyped",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeUnknown, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 123, T: 3000, V: 0.6}},
				{{Ref: 123, T: 4000, V: 100}},
			},
			//
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/unknown",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 3},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 0.6},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/unknown",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 100},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/unknown:counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 3},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 99.4},
						},
					}},
				},
			},
		}, {
			doc: "convert counter (Prometheus format metadata key)",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1_total": {Type: textparse.MetricTypeCounter, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_total", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 123, T: 2000, V: 5.5}},
				{{Ref: 123, T: 3000, V: 8}},
				{{Ref: 123, T: 4000, V: 9}},
				{{Ref: 123, T: 5000, V: 7}},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// First sample skipped to initialize reset handling.
				// Subsequent samples are relative to the initial sample in value and timestamp.
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 2},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 3},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 2.5},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 2},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 3.5},
						},
					}},
				},
				// Reset in the Prometheus series. Start timestamp is set to 1ms
				// before end timestamp.
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 4, Nanos: 999000000},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 5},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 7},
						},
					}},
				},
			},
		}, {
			doc: "convert counter - skip duplicates (OpenMetrics format metadata key)",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeCounter, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_total", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 123, T: 2000, V: 5.5},
					{Ref: 123, T: 2000, V: 5.5}, // duplicate
				}, {
					{Ref: 123, T: 4000, V: 9},
				}, {
					{Ref: 123, T: 5000, V: 7},
					{Ref: 123, T: 5000, V: 7}, // duplicate
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// First sample skipped to initialize reset handling.
				// Second sample was a duplicate of the reset value, should be dropped.
				// Subsequent samples are relative to the initial sample in value and timestamp.
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 2},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 3.5},
						},
					}},
				},
				// Reset in the Prometheus series. Start timestamp is set to 1ms
				// before end timestamp.
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 4, Nanos: 999000000},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 5},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 7},
						},
					}},
				},
				// subsequent duplicates still get through.
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 4, Nanos: 999000000},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 5},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 7},
						},
					}},
				},
			},
		}, {
			doc: "convert counter - skip on previous timestamp",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1_total": {Type: textparse.MetricTypeCounter, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_total", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 123, T: 2000, V: 5.5}},
				{{Ref: 123, T: 1000, V: 5.5}}, // drop old timestamp.
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// First sample skipped to initialize reset handling.
				// Second sample occured before first, panic.
			},
		}, {
			doc: "convert summary",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeSummary, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "quantile", "0.5"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "quantile", "0.9"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 1, T: 2000, V: 1},
					{Ref: 2, T: 2000, V: 2},
				}, {
					{Ref: 1, T: 3000, V: 21},
					{Ref: 3, T: 3000, V: 3},
				}, {
					{Ref: 3, T: 4000, V: 4},
					{Ref: 4, T: 4000, V: 4},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/summary",
						Labels: map[string]string{"quantile": "0.5"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 2},
						},
					}},
				},
				// first metric1_count and metric1_sum dropped by reset handling.
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_sum/summary:counter",
						Labels: map[string]string{},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 2},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 3},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 20},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_count/summary",
						Labels: map[string]string{},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 3},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 1},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/summary",
						Labels: map[string]string{"quantile": "0.9"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 4},
						},
					}},
				},
			},
		}, {
			doc: "convert summary - skip counter duplicates",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeSummary, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "quantile", "0.5"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "quantile", "0.9"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 1, T: 2000, V: 1},
					{Ref: 2, T: 2000, V: 2},
				}, {
					{Ref: 3, T: 3000, V: 3},
					{Ref: 3, T: 3000, V: 3}, // duplicate
				}, {
					{Ref: 3, T: 4000, V: 4},
					{Ref: 4, T: 4000, V: 4},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/summary",
						Labels: map[string]string{"quantile": "0.5"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 2},
						},
					}},
				},
				// first metric1_count dropped by reset handling.
				// duplicate of initial reset sample.
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_count/summary",
						Labels: map[string]string{},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 3},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 1},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"location":   "europe",
							"project_id": "example-project",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job1",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/summary",
						Labels: map[string]string{"quantile": "0.9"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 4},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 4},
						},
					}},
				},
			},
		}, {
			doc: "convert histogram",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1":         {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
				"metric1_a_count": {Type: textparse.MetricTypeGauge, Help: "metric1_a_count help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.1"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.5"),
				5: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "1"),
				6: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "2.5"),
				7: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
				// Add another series that only deviates by having an extra label. We must properly detect a new histogram.
				// This is a discouraged but possible case of metric labeling.
				8:  labels.FromStrings("job", "job1", "instance", "instance1", "a", "b", "__name__", "metric1_sum"),
				9:  labels.FromStrings("job", "job1", "instance", "instance1", "a", "b", "__name__", "metric1_count"),
				10: labels.FromStrings("job", "job1", "instance", "instance1", "a", "b", "__name__", "metric1_bucket", "le", "2.5"),
				11: labels.FromStrings("job", "job1", "instance", "instance1", "a", "b", "__name__", "metric1_bucket", "le", "+Inf"),
				// An incomplete histogram.
				12: labels.FromStrings("job", "job1", "instance", "instance1", "a", "c", "__name__", "metric1_sum"),
				13: labels.FromStrings("job", "job1", "instance", "instance1", "a", "c", "__name__", "metric1_count"),
				// Metric with prefix and suffix matching the previous histograms but actually a distinct metric.
				14: labels.FromStrings("job", "job1", "instance", "instance1", "a", "b", "__name__", "metric1_a_count"),
			},
			samples: [][]record.RefSample{
				// First sample set, should be skipped by reset handling.
				// The buckets must be in ascending order for an individual histogram but otherwise
				// no order or grouping constraints apply for series of a given histogram metric.
				{
					{Ref: 8, T: 1000, V: 100},  // hist2, sum
					{Ref: 1, T: 1000, V: 55.1}, // hist1, sum
					{Ref: 3, T: 1000, V: 2},    // hist1, 0.1
					{Ref: 4, T: 1000, V: 5},    // hist1, 0.5
					{Ref: 5, T: 1000, V: 6},    // hist1, 1
					{Ref: 6, T: 1000, V: 8},    // hist1, 2.5
					{Ref: 7, T: 1000, V: 10},   // hist1, inf
					{Ref: 9, T: 1000, V: 10},   // hist2, count
					{Ref: 2, T: 1000, V: 10},   // hist1, count
					{Ref: 10, T: 1000, V: 10},  // hist2, 2.5
					{Ref: 11, T: 1000, V: 10},  // hist2, inf
					{Ref: 12, T: 1000, V: 10},  // hist3, sum
					{Ref: 13, T: 1000, V: 10},  // hist3, count
				},
				// Second sample set should actually be emitted.
				{
					// Second samples for histograms should produce a distribution.
					{Ref: 3, T: 2000, V: 4},     // hist1, 0.1
					{Ref: 2, T: 2000, V: 21},    // hist1, count
					{Ref: 1, T: 2000, V: 123.4}, // hist1, sum
					{Ref: 4, T: 2000, V: 9},     // hist1, 0.5
					{Ref: 5, T: 2000, V: 11},    // hist1, 1
					{Ref: 6, T: 2000, V: 15},    // hist1, 2.5
					{Ref: 7, T: 2000, V: 21},    // hist1, inf
					{Ref: 10, T: 2000, V: 10},   // hist2, 2.5
					{Ref: 11, T: 2000, V: 13},   // hist2, inf
					{Ref: 9, T: 2000, V: 12},    // hist2, count – less than inf, ignored
					{Ref: 8, T: 2000, V: 115},   // hist2, sum
					// Incomplete histogram should not produce a sample.
					{Ref: 12, T: 2000, V: 10}, // hist3, sum
					{Ref: 13, T: 2000, V: 10}, // hist3, count
					// Different metric with prefix common with previous histograms must be detected
					// as the gauge it is.
					{Ref: 14, T: 1000, V: 3},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// 0: skipped by reset handling.
				{ // 1
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/histogram",
						Labels: map[string]string{},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 11,
									Mean:                  6.20909090909091,
									SumOfSquaredDeviation: 270.301590909091,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{0.1, 0.5, 1, 2.5},
											},
										},
									},
									BucketCounts: []int64{2, 2, 1, 2, 4},
								},
							},
						},
					}},
				},
				// 2: skipped by reset handling
				{ // 3
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/histogram",
						Labels: map[string]string{"a": "b"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 3,
									Mean:                  5,
									SumOfSquaredDeviation: 18.75,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{2.5},
											},
										},
									},
									BucketCounts: []int64{0, 3},
								},
							},
						},
					}},
				},
				{ // 4
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_a_count/gauge",
						Labels: map[string]string{"a": "b"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 1},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 3},
						},
					}},
				},
			},
		}, {
			doc: "histogram with 0 buckets is ignored",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
			},
			samples: [][]record.RefSample{
				// Add two samples for each series as the first ones are discarded for reset
				// handling regardless of the zero bucket count.
				{
					{Ref: 1, T: 1000, V: 5},
					{Ref: 2, T: 1000, V: 2},
				}, {
					{Ref: 1, T: 2000, V: 5},
					{Ref: 2, T: 2000, V: 2},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// skipped by reset handling.
				// skipped due to zero buckets.
			},
		}, {
			doc: "histogram with only Inf buckets is ignored",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
			},
			samples: [][]record.RefSample{
				// Add two samples for each series as the first ones are discarded for reset
				// handling regardless of the zero bucket bounds count.
				{
					{Ref: 1, T: 1000, V: 5},
					{Ref: 2, T: 1000, V: 2},
					{Ref: 3, T: 1000, V: 2},
				}, {
					{Ref: 1, T: 2000, V: 5},
					{Ref: 2, T: 2000, V: 2},
					{Ref: 3, T: 2000, V: 2},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// skipped by reset handling.
				// skipped due to zero buckets.
			},
		}, {
			doc: "histogram NaN sum",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "1"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 1, T: 1000, V: math.Float64frombits(value.NormalNaN)},
					{Ref: 2, T: 1000, V: 2},
					{Ref: 3, T: 1000, V: 1},
					{Ref: 4, T: 1000, V: 2},
				}, {
					{Ref: 1, T: 2000, V: math.Float64frombits(value.NormalNaN)},
					{Ref: 2, T: 2000, V: 3},
					{Ref: 3, T: 2000, V: 2},
					{Ref: 4, T: 2000, V: 3},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type: "prometheus.googleapis.com/metric1/histogram",
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 1,
									Mean:                  0,
									SumOfSquaredDeviation: 0.25,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{1},
											},
										},
									},
									BucketCounts: []int64{1, 0},
								},
							},
						},
					}},
				},
			},
		}, {
			doc: "histogram NaN count",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "1"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 1, T: 1000, V: 10},
					{Ref: 2, T: 1000, V: math.Float64frombits(value.NormalNaN)},
					{Ref: 3, T: 1000, V: 1},
					{Ref: 4, T: 1000, V: 2},
				}, {
					{Ref: 1, T: 2000, V: 10},
					{Ref: 2, T: 2000, V: math.Float64frombits(value.NormalNaN)},
					{Ref: 3, T: 2000, V: 2},
					{Ref: 4, T: 2000, V: 12},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type: "prometheus.googleapis.com/metric1/histogram",
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 10,
									Mean:                  0,
									SumOfSquaredDeviation: 9.25,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{1},
											},
										},
									},
									BucketCounts: []int64{1, 9},
								},
							},
						},
					}},
				},
			},
		}, {
			doc:      "no metric metadata",
			metadata: testMetadataFunc(metricMetadataMap{}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 1, T: 1000, V: 1}},
			},
			wantSeries: []*monitoring_pb.TimeSeries{},
		}, {
			doc: "filter with matchers",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeGauge, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "k1", "v1"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1", "k1", "v2"),
				3: labels.FromStrings("job", "job2", "instance", "instance1", "__name__", "metric1", "k1", "v3"),
				4: labels.FromStrings("job", "job2", "instance", "instance1", "__name__", "metric1", "k1", "v4"),
			},
			samples: [][]record.RefSample{{
				{Ref: 1, T: 1000, V: 1},
				{Ref: 1, T: 2000, V: 2},
				{Ref: 2, T: 1000, V: 1},
				{Ref: 3, T: 1000, V: 1},
				{Ref: 4, T: 1000, V: 1},
			}},
			// Series must pass either of the matchers
			matchers: Matchers{
				labels.Selector{
					labels.MustNewMatcher(labels.MatchEqual, "k1", "v1"),
				},
				labels.Selector{
					labels.MustNewMatcher(labels.MatchRegexp, "job", ".+2"),
					labels.MustNewMatcher(labels.MatchNotEqual, "k1", "v3"),
				},
			},
			// If the metadata is nil we expect the series to be converted to a gauge as
			// metadata-less series are produced by rules and any processing result of type gauge.
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/gauge",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 1},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 1},
						},
					}},
				}, {
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/gauge",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 2},
						},
					}},
				},
				{
					Resource: &monitoredres_pb.MonitoredResource{
						Type: "prometheus_target",
						Labels: map[string]string{
							"project_id": "example-project",
							"location":   "europe",
							"cluster":    "foo-cluster",
							"namespace":  "",
							"job":        "job2",
							"instance":   "instance1",
						},
					},
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/gauge",
						Labels: map[string]string{"k1": "v4"},
					},
					MetricKind: metric_pb.MetricDescriptor_GAUGE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							EndTime: &timestamp_pb.Timestamp{Seconds: 1},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 1},
						},
					}},
				},
			},
		}, {
			doc: "histogram is not in-order",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1": {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "1"),
				5: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.5"),
				6: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.1"),
			},
			samples: [][]record.RefSample{
				{
					{Ref: 3, T: 1000, V: 10}, // +Inf
					{Ref: 4, T: 1000, V: 5},  // 1
					{Ref: 6, T: 1000, V: 0},  // 0.1
					{Ref: 5, T: 1000, V: 0},  // 0.5
					{Ref: 1, T: 1000, V: 10}, // count
					{Ref: 2, T: 1000, V: 3},  // sum
				}, {
					{Ref: 3, T: 2000, V: 13}, //+Inf
					{Ref: 4, T: 2000, V: 7},  // 1
					{Ref: 6, T: 2000, V: 0},  // 0.1
					{Ref: 5, T: 2000, V: 1},  // 0.5
					{Ref: 1, T: 2000, V: 13}, // count
					{Ref: 2, T: 2000, V: 12}, // sum
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type: "prometheus.googleapis.com/metric1/histogram",
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 3,
									Mean:                  1,
									SumOfSquaredDeviation: 0.5525,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{0.1, 0.5, 1},
											},
										},
									},
									BucketCounts: []int64{0, 1, 1, 1},
								},
							},
						},
					}},
				},
			},
		},
		{
			doc: "convert histogram with exemplars",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1":         {Type: textparse.MetricTypeHistogram, Help: "metric1 help text"},
				"metric1_a_count": {Type: textparse.MetricTypeGauge, Help: "metric1_a_count help text"},
			}),
			series: seriesMap{
				1: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_sum"),
				2: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_count"),
				3: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.1"),
				4: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "0.5"),
				5: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "1"),
				6: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "2.5"),
				7: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_bucket", "le", "+Inf"),
			},
			samples: [][]record.RefSample{
				// First sample set, should be skipped by reset handling.
				// The buckets must be in ascending order for an individual histogram but otherwise
				// no order or grouping constraints apply for series of a given histogram metric.
				{
					{Ref: 1, T: 1000, V: 55.1}, // hist1, sum
					{Ref: 3, T: 1000, V: 2},    // hist1, 0.1
					{Ref: 4, T: 1000, V: 5},    // hist1, 0.5
					{Ref: 5, T: 1000, V: 6},    // hist1, 1
					{Ref: 6, T: 1000, V: 8},    // hist1, 2.5
					{Ref: 7, T: 1000, V: 10},   // hist1, inf
					{Ref: 2, T: 1000, V: 10},   // hist1, count
				},
				// Second sample set should actually be emitted.
				{
					// Second samples for histograms should produce a distribution.
					{Ref: 3, T: 2000, V: 4},     // hist1, 0.1
					{Ref: 2, T: 2000, V: 21},    // hist1, count
					{Ref: 1, T: 2000, V: 123.4}, // hist1, sum
					{Ref: 4, T: 2000, V: 9},     // hist1, 0.5
					{Ref: 5, T: 2000, V: 11},    // hist1, 1
					{Ref: 6, T: 2000, V: 15},    // hist1, 2.5
					{Ref: 7, T: 2000, V: 21},    // hist1, inf
				},
			},
			exemplars: []map[storage.SeriesRef]record.RefExemplar{
				// first sample set is skipped by reset handling
				{},
				{
					// project_id, trace_id, and span_id should be in the span context
					// random should be in the dropped labels
					3: {Ref: 3, T: 1500, V: .099, Labels: labels.New(
						labels.Label{Name: "project_id", Value: "1"},
						labels.Label{Name: "trace_id", Value: "2"},
						labels.Label{Name: "span_id", Value: "3"},
						labels.Label{Name: "random", Value: "4"},
					)},
					// project_id and trace_id should both be in dropped labels
					// since we have no span_id to make a full span context
					4: {Ref: 4, T: 1500, V: .4, Labels: labels.New(
						labels.Label{Name: "project_id", Value: "1"},
						labels.Label{Name: "trace_id", Value: "2"},
					)},
					5: {Ref: 5, T: 1500, V: .99},
					6: {Ref: 6, T: 1500, V: 2},
					7: {Ref: 7, T: 1500, V: 11},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// 0: skipped by reset handling.
				{ // 1
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1/histogram",
						Labels: map[string]string{},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DISTRIBUTION,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 1},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 2},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DistributionValue{
								DistributionValue: &distribution_pb.Distribution{
									Count:                 11,
									Mean:                  6.20909090909091,
									SumOfSquaredDeviation: 270.301590909091,
									BucketOptions: &distribution_pb.Distribution_BucketOptions{
										Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
											ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
												Bounds: []float64{0.1, 0.5, 1, 2.5},
											},
										},
									},
									BucketCounts: []int64{2, 2, 1, 2, 4},
									Exemplars: []*distribution_pb.Distribution_Exemplar{
										{
											Value:     .099,
											Timestamp: &timestamp_pb.Timestamp{Seconds: 1, Nanos: 500000000},
											Attachments: []*anypb.Any{
												wrapAsAny(&monitoring_pb.SpanContext{
													SpanName: "projects/1/traces/2/spans/3",
												}),
												wrapAsAny(&monitoring_pb.DroppedLabels{
													Label: map[string]string{"random": "4"},
												}),
											},
										},
										{
											Value:     .4,
											Timestamp: &timestamp_pb.Timestamp{Seconds: 1, Nanos: 500000000},
											Attachments: []*anypb.Any{
												wrapAsAny(&monitoring_pb.DroppedLabels{
													Label: map[string]string{"trace_id": "2", "project_id": "1"},
												}),
											},
										},
										{
											Value:     .99,
											Timestamp: &timestamp_pb.Timestamp{Seconds: 1, Nanos: 500000000},
										},
										{
											Value:     2,
											Timestamp: &timestamp_pb.Timestamp{Seconds: 1, Nanos: 500000000},
										},
										{
											Value:     11,
											Timestamp: &timestamp_pb.Timestamp{Seconds: 1, Nanos: 500000000},
										},
									},
								},
							},
						},
					}},
				},
			},
		},
		{
			doc: "convert counter with exemplars (exemplars should be dropped)",
			metadata: testMetadataFunc(metricMetadataMap{
				"metric1_total": {Type: textparse.MetricTypeCounter, Help: "metric1 help text"},
			}),
			series: seriesMap{
				123: labels.FromStrings("job", "job1", "instance", "instance1", "__name__", "metric1_total", "k1", "v1"),
			},
			samples: [][]record.RefSample{
				{{Ref: 123, T: 2000, V: 5.5}},
				{{Ref: 123, T: 3000, V: 8}},
			},
			exemplars: []map[storage.SeriesRef]record.RefExemplar{
				// first sample set is skipped by reset handling
				{},
				{
					// project_id, trace_id, and span_id should be in the span context
					// random should be in the dropped labels
					123: {Ref: 123, T: 2500, V: 7},
				},
			},
			wantSeries: []*monitoring_pb.TimeSeries{
				// First sample skipped to initialize reset handling.
				// Subsequent samples are relative to the initial sample in value and timestamp.
				{
					Resource: &monitoredres_pb.MonitoredResource{
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
					Metric: &metric_pb.Metric{
						Type:   "prometheus.googleapis.com/metric1_total/counter",
						Labels: map[string]string{"k1": "v1"},
					},
					MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
					ValueType:  metric_pb.MetricDescriptor_DOUBLE,
					Points: []*monitoring_pb.Point{{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: &timestamp_pb.Timestamp{Seconds: 2},
							EndTime:   &timestamp_pb.Timestamp{Seconds: 3},
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{DoubleValue: 2.5},
						},
					}},
				},
			},
		},
	}

	for i, c := range cases {
		t.Run(fmt.Sprintf("%d: %s", i, c.doc), func(t *testing.T) {
			cache := newSeriesCache(nil, nil, MetricTypePrefix, c.matchers)
			// Fake lookup into TSDB.
			cache.getLabelsByRef = func(ref storage.SeriesRef) labels.Labels {
				return c.series[ref]
			}

			// Process entire input sample batch.
			var result []*monitoring_pb.TimeSeries

			for i, batch := range c.samples {
				b := newSampleBuilder(cache)

				for k := 0; len(batch) > 0; k++ {
					var exemplars map[storage.SeriesRef]record.RefExemplar
					if len(c.exemplars) > i {
						exemplars = c.exemplars[i]
					}
					out, tail, err := b.next(c.metadata, externalLabels, batch, exemplars)
					if err == nil && c.wantFail {
						t.Fatal("expected error but got none")
					}
					if err != nil && !c.wantFail {
						t.Fatalf("unexpected error: %s", err)
					}
					if err != nil {
						break
					}
					if len(tail) >= len(batch) {
						t.Fatalf("no sample was consumed")
					}
					for _, s := range out {
						result = append(result, s.proto)
					}
					batch = tail
				}
				b.close()
			}
			if diff := cmp.Diff(c.wantSeries, result, protocmp.Transform(), cmpopts.EquateEmpty()); diff != "" {
				t.Errorf("unexpected result (-want, +got): %v", diff)
			}
		})
	}
}
