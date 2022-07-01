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
	"reflect"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type FakeMetricServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	timeSeriesByProject     map[string][]*monitoringpb.TimeSeries
	maxTimeSeriesPerRequest int
}

// initialize an empty map in the FakeMetricServer since Go does not let you add to a nil map
func NewFakeMetricServer(maxTimeSeriesPerRequest int) *FakeMetricServer {
	return &FakeMetricServer{
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
		for _, singleTimeSeriesInMemory := range fms.timeSeriesByProject[req.Name] {
			inMemMetric := singleTimeSeriesInMemory.Metric
			toAddMetric := singleTimeSeriesToAdd.Metric
			inMemResource := singleTimeSeriesInMemory.Resource
			toAddResource := singleTimeSeriesToAdd.Resource

			if inMemMetric.Type == toAddMetric.Type && reflect.DeepEqual(inMemMetric.Labels, toAddMetric.Labels) &&
				inMemResource.Type == toAddResource.Type && reflect.DeepEqual(inMemResource.Labels, toAddResource.Labels) {
				// only add this point if the start time of the point to add is greater than the end point latest in this time series
				if singleTimeSeriesToAdd.Points[0].Interval.StartTime.AsTime().After(
					singleTimeSeriesInMemory.Points[len(singleTimeSeriesInMemory.Points)-1].Interval.EndTime.AsTime()) {
					singleTimeSeriesInMemory.Points = append(singleTimeSeriesInMemory.Points, singleTimeSeriesToAdd.Points...)
				} else {
					numErrors++
				}
				break
			}
		}
	}

	var err error
	if numErrors > 0 {
		err = fmt.Errorf("there were %d time series that could not be added", numErrors)
	}
	return &emptypb.Empty{}, err
}

func (fms *FakeMetricServer) ListTimeSeries(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	response := &monitoringpb.ListTimeSeriesResponse{}

	return response, nil
}
