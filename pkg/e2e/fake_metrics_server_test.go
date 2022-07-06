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
	"reflect"
	"testing"

	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	timestamppb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestAddTimeSeriesNil(t *testing.T) {
	fms := NewFakeMetricServer(200)
	response, err := fms.CreateTimeSeries(context.TODO(), nil)
	if err == nil && response != nil {
		t.Error("expected an error when calling CreateTimeSeries with all nil")
	}
}

func TestMaxTimeSeriesPerRequest(t *testing.T) {
	fms := NewFakeMetricServer(1)
	timeSeries := []*monitoringpb.TimeSeries{
		{},
		{},
	}
	createTimeSeriesRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name:       "PROJECT/1234",
		TimeSeries: timeSeries,
	}
	response, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)
	if err == nil && response != nil {
		t.Error("expected an error when sending more time series than the server allows")
	}
}

func TestNoTimeSeriesToAdd(t *testing.T) {
	fms := NewFakeMetricServer(200)
	timeSeries := []*monitoringpb.TimeSeries{}
	createTimeSeriesRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name:       "PROJECT/1234",
		TimeSeries: timeSeries,
	}
	response, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)
	if err == nil && response != nil {
		t.Error("expected an error when sending no time series")
	}
}

func TestTimeSeriesAddTimeSeries(t *testing.T) {
	fms := NewFakeMetricServer(200)
	projectName := "PROJECT/1234"

	// add a mew time series for a project that does not have one yet
	timeSeries := []*monitoringpb.TimeSeries{{
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
			Type:   "prometheus.googleapis.com/metric1/gauge",
			Labels: map[string]string{"k1": "v1"},
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: 1},
				EndTime:   &timestamppb.Timestamp{Seconds: 2},
			},
			Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
			},
		}},
	}}
	createTimeSeriesRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name:       projectName,
		TimeSeries: timeSeries,
	}
	response, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)
	if err != nil || response == nil {
		t.Error("did not expect an error when adding a new project with a time series")
	}
	if !reflect.DeepEqual(fms.timeSeriesByProject[projectName][0], timeSeries[0]) {
		t.Error("expected the new project's timeseries to be saved to the fake metric server")
	}

	// add a new time series for a project that already has a time series
	timeSeries2 := []*monitoringpb.TimeSeries{{
		Resource: &monitoredrespb.MonitoredResource{
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
		Metric: &metricpb.Metric{
			Type:   "prometheus.googleapis.com/metric1/gauge",
			Labels: map[string]string{"k1": "v1"},
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: 1},
				EndTime:   &timestamppb.Timestamp{Seconds: 2},
			},
			Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
			},
		}},
	}}
	createTimeSeriesRequest2 := &monitoringpb.CreateTimeSeriesRequest{
		Name:       projectName,
		TimeSeries: timeSeries2,
	}
	response2, err2 := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest2)
	if err2 != nil || response2 == nil {
		t.Error("did not expect an error when adding a mew time series")
	}
	if !reflect.DeepEqual(fms.timeSeriesByProject[projectName][1], timeSeries2[0]) {
		t.Error("expected the second time series to be the newly created one")
	}
	if len(fms.timeSeriesByProject[projectName]) != 2 {
		t.Error("two time series for this project")
	}

	// add a new point to an existing time series
	timeSeries3 := []*monitoringpb.TimeSeries{{
		Resource: &monitoredrespb.MonitoredResource{
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
		Metric: &metricpb.Metric{
			Type:   "prometheus.googleapis.com/metric1/gauge",
			Labels: map[string]string{"k1": "v1"},
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: 3},
				EndTime:   &timestamppb.Timestamp{Seconds: 4},
			},
			Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
			},
		}},
	}}
	createTimeSeriesRequest3 := &monitoringpb.CreateTimeSeriesRequest{
		Name:       projectName,
		TimeSeries: timeSeries3,
	}
	response3, err3 := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest3)
	if err3 != nil || response3 == nil {
		t.Error("did not expect an error when adding a mew time series")
	}
	if len(fms.timeSeriesByProject[projectName][1].Points) != 2 {
		t.Error("expected the new data point to be added to the second time series")
	}
	if len(fms.timeSeriesByProject[projectName]) != 2 {
		t.Error("expected two time series")
	}

	// reject addition to a time series if the point occurs before the last point
	timeSeries4 := []*monitoringpb.TimeSeries{{
		Resource: &monitoredrespb.MonitoredResource{
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
		Metric: &metricpb.Metric{
			Type:   "prometheus.googleapis.com/metric1/gauge",
			Labels: map[string]string{"k1": "v1"},
		},
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points: []*monitoringpb.Point{{
			Interval: &monitoringpb.TimeInterval{
				StartTime: &timestamppb.Timestamp{Seconds: 1},
				EndTime:   &timestamppb.Timestamp{Seconds: 2},
			},
			Value: &monitoringpb.TypedValue{
				Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
			},
		}},
	}}
	createTimeSeriesRequest4 := &monitoringpb.CreateTimeSeriesRequest{
		Name:       projectName,
		TimeSeries: timeSeries4,
	}
	response4, err4 := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest4)
	if err4 == nil || response4 != nil {
		t.Error("did not expect an error when adding a mew time series")
	}
}
