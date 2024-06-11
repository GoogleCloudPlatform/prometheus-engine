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
	"context"
	"reflect"
	"testing"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Returns true if every field in TimeSeries a is deeply equal to TimeSeries b
// ignoring the Points field. False otherwise.
func timeSeriesEqualsExceptPoints(a *monitoringpb.TimeSeries, b *monitoringpb.TimeSeries) bool {
	tmp := a.Points
	a.Points = b.Points
	isEqual := reflect.DeepEqual(a, b)
	a.Points = tmp
	return isEqual
}

func TestCreateTimeSeriesBadInput(t *testing.T) {
	ctx := context.Background()
	db := NewMetricDatabase()
	fms := fakeMetricServer{
		maxTimeSeriesPerRequest: 1,
		db:                      db,
	}
	projectName := "projects/1234"
	// add a time series to the FakeMetricServer so that
	// TestAddPointInPast will fail as expected
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
	if _, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest); err != nil {
		t.Fatalf("create time series: %s", err)
	}

	// these are the subtests
	tests := []*struct {
		desc     string
		requests []*monitoringpb.CreateTimeSeriesRequest
	}{
		{
			desc:     "TestNil",
			requests: []*monitoringpb.CreateTimeSeriesRequest{nil},
		},
		{
			desc: "TestExceedMaxTimeSeriesPerRequest",
			requests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
					// Note: the TimeSeries are empty here since the check for exceeding
					// the max timeseries in a requet happens before we verify
					// data the data in the TimeSeries.
					TimeSeries: []*monitoringpb.TimeSeries{{}, {}},
				},
			},
		},
		{
			desc: "TestNoTimeSeriesToAdd",
			requests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
				},
			},
		},
		{
			desc: "TestNoPointInTimeSeriesToAdd",
			requests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
					TimeSeries: []*monitoringpb.TimeSeries{{
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
					}},
				},
			},
		},
		{
			desc: "TestAddPointInPast",
			requests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
					TimeSeries: []*monitoringpb.TimeSeries{{
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
					}},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			for _, request := range test.requests {
				_, err := fms.CreateTimeSeries(ctx, request)
				if err == nil {
					t.Errorf("expected an error for %q", test.desc)
				}
			}
		})
	}
}

func TestCreateTimeSeries(t *testing.T) {
	ctx := context.Background()
	db := NewMetricDatabase()
	fms := newFakeMetricServer(db)
	projectName := "projects/1234"

	// these are the subtests
	tests := []*struct {
		desc     string
		requests []*monitoringpb.CreateTimeSeriesRequest
		// index we expect the newly added timeseries to be in the fake metric server
		timeSeriesIndexToCheck []int
		// index we expect the newly added point to be in the fake metric server
		pointsIndexToCheck []int
	}{
		// This test adds a new time series with a new project id to the fake metric server.
		// It then adds a new time series to the same project.
		// It then adds a new point to the second time series.
		{
			desc:                   "TestCreateTimeSeries-NewProject-NewTimeSeries-NewPoint",
			timeSeriesIndexToCheck: []int{0, 1, 1},
			pointsIndexToCheck:     []int{0, 0, 0},
			requests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
					TimeSeries: []*monitoringpb.TimeSeries{{
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
					}},
				},
				{
					Name: projectName,
					TimeSeries: []*monitoringpb.TimeSeries{{
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
					}},
				},
				{
					Name: projectName,
					TimeSeries: []*monitoringpb.TimeSeries{{
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
					}},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			for i, request := range test.requests {
				response, err := fms.CreateTimeSeries(ctx, request)
				if err != nil || response == nil {
					t.Errorf("unexpected error: %s", err)
				}
				if !timeSeriesEqualsExceptPoints(
					request.TimeSeries[0],
					db.Get(projectName)[test.timeSeriesIndexToCheck[i]],
				) {
					t.Errorf(
						"expected %+v and got %+v. Note: the points were not compared",
						request.TimeSeries[0],
						db.Get(projectName)[test.timeSeriesIndexToCheck[i]],
					)
				}
				if !reflect.DeepEqual(
					request.TimeSeries[0].Points[0],
					db.Get(projectName)[test.timeSeriesIndexToCheck[i]].Points[test.pointsIndexToCheck[i]],
				) {
					t.Errorf(
						"expected %+v and got %+v",
						request.TimeSeries[0].Points[0],
						db.Get(projectName)[test.timeSeriesIndexToCheck[i]].Points[test.pointsIndexToCheck[i]],
					)
				}
			}
		})
	}
}

func TestCreateTimeSeriesTwoSeries(t *testing.T) {
	db := NewMetricDatabase()
	fms := newFakeMetricServer(db)
	projectName := "projects/1234"

	request := &monitoringpb.CreateTimeSeriesRequest{
		Name: projectName,
		TimeSeries: []*monitoringpb.TimeSeries{
			{
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
			},
			{
				Resource: &monitoredrespb.MonitoredResource{
					Type: "prometheus_target",
					Labels: map[string]string{
						"project_id": "example-project",
						"location":   "europe1",
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
			},
		},
	}
	response, err := fms.CreateTimeSeries(context.TODO(), request)
	if err != nil || response == nil {
		t.Errorf("unexpected error: %s", err)
	}
	for i := range request.TimeSeries {
		if !reflect.DeepEqual(request.TimeSeries[i], db.Get(projectName)[i]) {
			t.Errorf("expected %+v and got %+v", request.TimeSeries[i], db.Get(projectName)[i])
		}
	}
}

func TestListTimeSeriesBadInput(t *testing.T) {
	db := NewMetricDatabase()
	fms := newFakeMetricServer(db)
	projectName := "projects/1234"
	filter := "metric.type = \"prometheus.googleapis.com/metric1/gauge\""

	// these are the subtests
	tests := []*struct {
		desc    string
		request *monitoringpb.ListTimeSeriesRequest
	}{
		{
			desc:    "TestListTimeSeriesNilRequest",
			request: nil,
		},
		{},
		{
			desc: "TestListTimeSeriesAggregation",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:        projectName,
				Aggregation: &monitoringpb.Aggregation{},
				Filter:      filter,
			},
		},
		{
			desc: "TestListTimeSeriesNoInterval",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
			},
		},
		{
			desc: "TestListTimeSeriesHeadersView",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 1},
					EndTime:   &timestamppb.Timestamp{Seconds: 2},
				},
				View: monitoringpb.ListTimeSeriesRequest_HEADERS,
			},
		},
		{
			desc: "TestListTimeSeriesMalformedFilter",
			request: &monitoringpb.ListTimeSeriesRequest{

				Name:   projectName,
				Filter: "metric.type = \"prometheus-target\" AND metric.labels.location = ",
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 1},
					EndTime:   &timestamppb.Timestamp{Seconds: 2},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			response, err := fms.ListTimeSeries(context.TODO(), test.request)
			if err == nil || response != nil {
				t.Errorf("expected an error for %q", test.desc)
			}
		})
	}
}

func TestListTimeSeries(t *testing.T) {
	db := NewMetricDatabase()
	fms := newFakeMetricServer(db)
	projectName := "projects/1234"
	filter := "metric.type = \"prometheus.googleapis.com/metric1/gauge\" AND project = \"example-project\""

	point1 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 1},
			EndTime:   &timestamppb.Timestamp{Seconds: 2},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 0.6},
		},
	}

	point2 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 4},
			EndTime:   &timestamppb.Timestamp{Seconds: 5},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 9.8},
		},
	}

	point3 := &monitoringpb.Point{
		Interval: &monitoringpb.TimeInterval{
			StartTime: &timestamppb.Timestamp{Seconds: 11},
			EndTime:   &timestamppb.Timestamp{Seconds: 18},
		},
		Value: &monitoringpb.TypedValue{
			Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: 1.4},
		},
	}

	resource1 := &monitoredrespb.MonitoredResource{
		Type: "prometheus_target",
		Labels: map[string]string{
			"project_id": "example-project",
			"location":   "europe",
			"cluster":    "foo-cluster",
			"namespace":  "default",
			"job":        "job1",
			"instance":   "instance1",
		},
	}

	resource2 := &monitoredrespb.MonitoredResource{
		Type: "prometheus_target",
		Labels: map[string]string{
			"project_id": "example-project",
			"location":   "europe",
			"cluster":    "foo-cluster",
			"namespace":  "default",
			"job":        "job2",
			"instance":   "instance1",
		},
	}

	resource3 := &monitoredrespb.MonitoredResource{
		Type: "prometheus_target",
		Labels: map[string]string{
			"project_id": "example-project2",
			"location":   "europe",
			"cluster":    "foo-cluster",
			"namespace":  "default",
			"job":        "job1",
			"instance":   "instance1",
		},
	}

	metric := &metricpb.Metric{
		Type:   "prometheus.googleapis.com/metric1/gauge",
		Labels: map[string]string{"k1": "v1"},
	}

	timeSeriesJob1 := &monitoringpb.TimeSeries{
		Resource:   resource1,
		Metric:     metric,
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points:     []*monitoringpb.Point{point1},
	}

	timeSeriesJob2 := &monitoringpb.TimeSeries{
		Resource:   resource2,
		Metric:     metric,
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points:     []*monitoringpb.Point{point1},
	}

	timeSeriesJob3 := &monitoringpb.TimeSeries{
		Resource:   resource3,
		Metric:     metric,
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points:     []*monitoringpb.Point{point3},
	}

	timeSeries := []*monitoringpb.TimeSeries{timeSeriesJob1, timeSeriesJob2, timeSeriesJob3}
	createTimeSeriesRequest := &monitoringpb.CreateTimeSeriesRequest{
		Name:       projectName,
		TimeSeries: timeSeries,
	}
	if _, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest); err != nil {
		t.Fatalf("create time series: %s", err)
	}

	timeSeriesJob1Point2 := &monitoringpb.TimeSeries{
		Resource:   resource1,
		Metric:     metric,
		MetricKind: metricpb.MetricDescriptor_GAUGE,
		ValueType:  metricpb.MetricDescriptor_DOUBLE,
		Points:     []*monitoringpb.Point{point2},
	}

	createTimeSeriesRequest.TimeSeries = []*monitoringpb.TimeSeries{timeSeriesJob1Point2}
	if _, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest); err != nil {
		t.Fatalf("create time series: %s", err)
	}

	testCases := []*struct {
		desc     string
		request  *monitoringpb.ListTimeSeriesRequest
		expected *monitoringpb.ListTimeSeriesResponse
	}{
		{
			desc: "filter in range short interval",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 0},
					EndTime:   &timestamppb.Timestamp{Seconds: 3},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Resource:   resource1,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point1},
					},
					{
						Resource:   resource2,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point1},
					},
				},
			},
		},
		{
			desc: "filter in range wide interval",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 0},
					EndTime:   &timestamppb.Timestamp{Seconds: 6},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Resource:   resource1,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point2, point1},
					},
					{
						Resource:   resource2,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point1},
					},
				},
			},
		},
		{
			desc: "interval out of range",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 10},
					EndTime:   &timestamppb.Timestamp{Seconds: 11},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{},
			},
		},
		{
			desc: "filter within point interval",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: filter,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 12},
					EndTime:   &timestamppb.Timestamp{Seconds: 14},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Resource:   resource3,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point3},
					},
				},
			},
		},
		{
			desc: "filter project",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: "project = \"example-project\"",
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 0},
					EndTime:   &timestamppb.Timestamp{Seconds: 6},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Resource:   resource1,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point2, point1},
					},
					{
						Resource:   resource2,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point1},
					},
				},
			},
		},
		{
			desc: "complex filter",
			request: &monitoringpb.ListTimeSeriesRequest{
				Name:   projectName,
				Filter: `resource.type = "prometheus_target" AND resource.labels.project_id = "example-project" AND resource.labels.location = "europe" AND resource.labels.cluster = "foo-cluster" AND resource.labels.namespace = "default" AND metric.type = "prometheus.googleapis.com/metric1/gauge"`,
				Interval: &monitoringpb.TimeInterval{
					StartTime: &timestamppb.Timestamp{Seconds: 0},
					EndTime:   &timestamppb.Timestamp{Seconds: 6},
				},
			},
			expected: &monitoringpb.ListTimeSeriesResponse{
				TimeSeries: []*monitoringpb.TimeSeries{
					{
						Resource:   resource1,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point2, point1},
					},
					{
						Resource:   resource2,
						Metric:     metric,
						MetricKind: metricpb.MetricDescriptor_GAUGE,
						ValueType:  metricpb.MetricDescriptor_DOUBLE,
						Points:     []*monitoringpb.Point{point1},
					},
				},
			},
		},
	}

	ctx := context.Background()
	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			response, err := fms.ListTimeSeries(ctx, tc.request)
			if err != nil {
				t.Errorf("unexpected error: %s", err)
			}
			if diff := cmp.Diff(tc.expected, response, protocmp.Transform()); diff != "" {
				t.Errorf("expected response (-want, +got) %s", diff)
			}
		})
	}
}
