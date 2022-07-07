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
	testName                string
	createTimeSeriesRequest *monitoringpb.CreateTimeSeriesRequest
	timeSeriesIndexToCheck  int
	pointsIndexToCheck      int
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
		Name:       "PROJECT/1234",
		TimeSeries: timeSeries,
	}
	fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)

	// these are the subtests
	tests := []*createTimeSeriesTest{
		{testName: "TestNil"},
		{
			testName: "TestExceedMaxTimeSeriesPerRequest",
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
				Name:       "PROJECT/1234",
				TimeSeries: []*monitoringpb.TimeSeries{{}, {}},
			},
		},
		{
			testName: "TestNoTimeSeriesToAdd",
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
				Name: "PROJECT/1234",
			},
		},
		{
			testName: "TestNoPointInTimeSeriesToAdd",
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
				Name: "PROJECT/1234",
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
		{
			testName: "TestAddPointInPast",
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
				Name: "PROJECT/1234",
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
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			response, err := fms.CreateTimeSeries(context.TODO(), test.createTimeSeriesRequest)
			if err == nil && response != nil {
				t.Errorf("expected an error for %q", test.testName)
			}
		})
	}
}

func TestCreateTimeSeries(t *testing.T) {
	fms := NewFakeMetricServer(200)
	projectName := "PROJECT/1234"

	// these are the subtests
	tests := []*createTimeSeriesTest{
		{
			testName:               "TestNewTimeSeriesForNewProject",
			timeSeriesIndexToCheck: 0,
			pointsIndexToCheck:     0,
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
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
		{
			testName:               "TestNewTimeSeriesExistingProject",
			timeSeriesIndexToCheck: 1,
			pointsIndexToCheck:     0,
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
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
		},
		{
			testName:               "TestAddNewPointExistingTimeSeries",
			timeSeriesIndexToCheck: 1,
			pointsIndexToCheck:     1,
			createTimeSeriesRequest: &monitoringpb.CreateTimeSeriesRequest{
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
	}

	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			response, err := fms.CreateTimeSeries(context.TODO(), test.createTimeSeriesRequest)
			if err != nil || response == nil {
				t.Errorf("did not expect an error when running %q", test.testName)
			}
			if !timeSeriesEqualsExceptPoints(
				test.createTimeSeriesRequest.TimeSeries[0],
				fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck],
			) {
				t.Errorf(
					"expected %+v and got %+v. Note: the points were not compared",
					test.createTimeSeriesRequest.TimeSeries[0],
					fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck],
				)
			}
			if !reflect.DeepEqual(
				test.createTimeSeriesRequest.TimeSeries[0].Points[0],
				fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck].Points[test.pointsIndexToCheck],
			) {
				t.Errorf(
					"expected %+v and got %+v",
					test.createTimeSeriesRequest.TimeSeries[0].Points[0],
					fms.timeSeriesByProject[projectName][test.timeSeriesIndexToCheck].Points[test.pointsIndexToCheck],
				)
			}
		})
	}
}
