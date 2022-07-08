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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/go-cmp/cmp"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type FakeMetricServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	timeSeriesByProject     map[string][]*monitoringpb.TimeSeries
	maxTimeSeriesPerRequest int
}

func NewFakeMetricServer(maxTimeSeriesPerRequest int) *FakeMetricServer {
	return &FakeMetricServer{
		// initialize an empty map in the FakeMetricServer since Go does not let you add to a nil map
		timeSeriesByProject:     make(map[string][]*monitoringpb.TimeSeries),
		maxTimeSeriesPerRequest: maxTimeSeriesPerRequest,
	}
}

func (fms *FakeMetricServer) CreateTimeSeries(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if len(req.TimeSeries) < 1 {
		return nil, errors.New("there are no time series to add")
	}
	if len(req.TimeSeries) > fms.maxTimeSeriesPerRequest {
		return nil, errors.New("exceeded the max number of time series")
	}

	var timeSeriesToProcess []*monitoringpb.TimeSeries
	for _, timeSeries := range req.TimeSeries {
		if len(timeSeries.Points) == 1 {
			timeSeriesToProcess = append(timeSeriesToProcess, timeSeries)
		}
	}
	numErrors := len(req.TimeSeries) - len(timeSeriesToProcess)

	// this is pretty inefficient, but it is only used for testing purposes
	for _, singleTimeSeriesToAdd := range timeSeriesToProcess {
		// new project with a timeseries
		if fms.timeSeriesByProject[req.Name] == nil {
			fms.timeSeriesByProject[req.Name] = req.TimeSeries
		} else { // project already exists
			for i, singleTimeSeriesInMemory := range fms.timeSeriesByProject[req.Name] {
				inMemMetric := singleTimeSeriesInMemory.Metric
				toAddMetric := singleTimeSeriesToAdd.Metric
				inMemResource := singleTimeSeriesInMemory.Resource
				toAddResource := singleTimeSeriesToAdd.Resource

				// if this specific time series already exists, add it
				if inMemMetric.Type == toAddMetric.Type && cmp.Equal(inMemMetric.Labels, toAddMetric.Labels) &&
					inMemResource.Type == toAddResource.Type && cmp.Equal(inMemResource.Labels, toAddResource.Labels) {
					// only add this point if the start time of the point to add is greater than the end point latest in this time series
					if singleTimeSeriesToAdd.Points[0].Interval.StartTime.AsTime().After(singleTimeSeriesInMemory.Points[len(singleTimeSeriesInMemory.Points)-1].Interval.EndTime.AsTime()) {
						// add the new point to the beginning
						singleTimeSeriesInMemory.Points = append(singleTimeSeriesToAdd.Points, singleTimeSeriesInMemory.Points...)
					} else {
						numErrors++
					}
					break
					// if we make it into this else if block then we are adding a new time series for an existing project -- just append it.
				} else if i == len(fms.timeSeriesByProject[req.Name])-1 {
					fms.timeSeriesByProject[req.Name] = append(fms.timeSeriesByProject[req.Name], singleTimeSeriesToAdd)
				}
			}
		}
	}

	var err error
	var response *emptypb.Empty
	if numErrors > 0 {
		err = fmt.Errorf("there were %d time series that could not be added", numErrors)
	}
	if numErrors != len(req.TimeSeries) {
		response = &emptypb.Empty{}
	}
	return response, err
}

// ListTimeSeries only supports fetching raw time series data from the FakeMetricServer
// since we only use this function to verify data made its way in.
func (fms *FakeMetricServer) ListTimeSeries(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if req.Aggregation != nil || req.SecondaryAggregation != nil {
		return nil, errors.New("fake metric server does not support aggregation")
	}
	if req.Interval == nil {
		return nil, errors.New("time interval required")
	}
	if req.View == monitoringpb.ListTimeSeriesRequest_HEADERS {
		return nil, errors.New("header view is not supported")
	}

	// The filter string MUST be in the form:
	//		metric.type = <string>
	// return an error if it is not in this form
	filter := strings.Split(req.Filter, "=")
	if len(filter) != 2 || strings.ToLower(strings.TrimSpace(filter[0])) != "metric.type" {
		return nil, fmt.Errorf("filter string %q is malformed - only metric.type supported", req.Filter)
	}

	reqStartTime := req.Interval.StartTime.AsTime()
	var reqEndTime time.Time
	if req.Interval.EndTime != nil {
		reqEndTime = req.Interval.EndTime.AsTime()
	}

	var timeSeriesToReturn []*monitoringpb.TimeSeries
	for _, timeSeries := range fms.timeSeriesByProject[req.Name] {
		if timeSeries.Metric.Type == filter[1] {
			var pointsToReturn []*monitoringpb.Point
			for _, point := range timeSeries.Points {
				pointStartTime := point.Interval.StartTime.AsTime()
				pointEndTime := point.Interval.EndTime.AsTime()
				if pointStartTime.After(reqStartTime) && (req.Interval.EndTime == nil || pointEndTime.Before(reqEndTime) || pointEndTime.Equal(reqEndTime)) {
					pointsToReturn = append(pointsToReturn, point)
				}
			}
			newTimeSeries := &monitoringpb.TimeSeries{
				Metric:     timeSeries.Metric,
				Resource:   timeSeries.Resource,
				Metadata:   timeSeries.Metadata,
				MetricKind: timeSeries.MetricKind,
				ValueType:  timeSeries.ValueType,
				Points:     pointsToReturn,
				Unit:       timeSeries.Unit,
			}
			timeSeriesToReturn = append(timeSeriesToReturn, newTimeSeries)
		}
	}
	response := &monitoringpb.ListTimeSeriesResponse{
		TimeSeries: timeSeriesToReturn,
	}
	return response, nil
}
