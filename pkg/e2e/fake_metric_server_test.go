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

type createTimeSeriesTest struct {
	testName                 string
	createTimeSeriesRequests []*monitoringpb.CreateTimeSeriesRequest
	// index we expect the newly added timeseries to be in the fake metric server
	timeSeriesIndexToCheck []int
	// index we expect the newly added point to be in the fake metric server
	pointsIndexToCheck []int
}

type listTimeSeriesTest struct {
	testName                       string
	createTimeSeriesRequests       []*monitoringpb.CreateTimeSeriesRequest
	listTimeSeriesRequests         []*monitoringpb.ListTimeSeriesRequest
	expectedListTimeSeriesResponse []*monitoringpb.ListTimeSeriesResponse
}

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
	fms := NewFakeMetricServer(1)
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
	fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)

	// these are the subtests
	tests := []*createTimeSeriesTest{
		{
			testName:                 "TestNil",
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{nil},
		},
		{
			testName: "TestExceedMaxTimeSeriesPerRequest",
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{
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
			testName: "TestNoTimeSeriesToAdd",
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{
				{
					Name: projectName,
				},
			},
		},
		{
			testName: "TestNoPointInTimeSeriesToAdd",
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{
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
			testName: "TestAddPointInPast",
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{
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
		t.Run(test.testName, func(t *testing.T) {
			for _, request := range test.createTimeSeriesRequests {
				response, err := fms.CreateTimeSeries(context.TODO(), request)
				if err == nil && response != nil {
					t.Errorf("expected an error for %q", test.testName)
				}
			}
		})
	}
}

func TestCreateTimeSeries(t *testing.T) {
	fms := NewFakeMetricServer(200)
	projectName := "projects/1234"

	// these are the subtests
	tests := []*createTimeSeriesTest{
		// This test adds a new time series with a new project id to the fake metric server.
		// It then adds a new time series to the same project.
		// It then adds a new point to the second time series.
		{
			testName:               "TestCreateTimeSeries-NewProject-NewTimeSeries-NewPoint",
			timeSeriesIndexToCheck: []int{0, 1, 1},
			pointsIndexToCheck:     []int{0, 0, 0},
			createTimeSeriesRequests: []*monitoringpb.CreateTimeSeriesRequest{
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
		t.Run(test.testName, func(t *testing.T) {
			for i, request := range test.createTimeSeriesRequests {
				response, err := fms.CreateTimeSeries(context.TODO(), request)
				if err != nil || response == nil {
					t.Errorf("did not expect an error when running %q", test.testName)
				}
				if !timeSeriesEqualsExceptPoints(
					request.TimeSeries[0],
					fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck[i]],
				) {
					t.Errorf(
						"expected %+v and got %+v. Note: the points were not compared",
						request.TimeSeries[0],
						fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck[i]],
					)
				}
				if !reflect.DeepEqual(
					request.TimeSeries[0].Points[0],
					fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck[i]].Points[test.pointsIndexToCheck[i]],
				) {
					t.Errorf(
						"expected %+v and got %+v",
						request.TimeSeries[0].Points[0],
						fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck[i]].Points[test.pointsIndexToCheck[i]],
					)
				}
			}
		})
	}
}

func TestListTimeSeriesBadInput(t *testing.T) {
	fms := NewFakeMetricServer(200)
	projectName := "projects/1234"
	filter := "metric.type = prometheus_target"

	// these are the subtests
	tests := []*listTimeSeriesTest{
		{
			testName:               "TestListTimeSeriesNilRequest",
			listTimeSeriesRequests: []*monitoringpb.ListTimeSeriesRequest{nil},
		},
		{},
		{
			testName: "TestListTimeSeriesAggregation",
			listTimeSeriesRequests: []*monitoringpb.ListTimeSeriesRequest{
				{
					Name:        projectName,
					Aggregation: &monitoringpb.Aggregation{},
					Filter:      filter,
				},
			},
		},
		{
			testName: "TestListTimeSeriesNoInterval",
			listTimeSeriesRequests: []*monitoringpb.ListTimeSeriesRequest{
				{
					Name:   projectName,
					Filter: filter,
				},
			},
		},
		{
			testName: "TestListTimeSeriesHeadersView",
			listTimeSeriesRequests: []*monitoringpb.ListTimeSeriesRequest{
				{
					Name:   projectName,
					Filter: filter,
					Interval: &monitoringpb.TimeInterval{
						StartTime: &timestamppb.Timestamp{Seconds: 1},
						EndTime:   &timestamppb.Timestamp{Seconds: 2},
					},
					View: monitoringpb.ListTimeSeriesRequest_HEADERS,
				},
			},
		},
		{
			testName: "TestListTimeSeriesMalformedFilter",
			listTimeSeriesRequests: []*monitoringpb.ListTimeSeriesRequest{
				{
					Name:   projectName,
					Filter: "metric.type = prometheus-target AND metric.labels.location = europe",
					Interval: &monitoringpb.TimeInterval{
						StartTime: &timestamppb.Timestamp{Seconds: 1},
						EndTime:   &timestamppb.Timestamp{Seconds: 2},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			for _, request := range test.listTimeSeriesRequests {
				response, err := fms.ListTimeSeries(context.TODO(), request)
				if err == nil && response != nil {
					t.Errorf("expected an error for %q", test.testName)
				}
			}
		})
	}
}
