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

package test

import (
	"context"
	"errors"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

type FakeMetricServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	timeSeriesByProject map[string][]*monitoringpb.TimeSeries
}

// initialize an empty map in the FakeMetricServer since go does not let you add to a nil map
func NewFakeMetricServer() *FakeMetricServer {
	return &FakeMetricServer{
		timeSeriesByProject: make(map[string][]*monitoringpb.TimeSeries),
	}
}

func (*FakeMetricServer) CreateTimeSeries(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	if len(req.TimeSeries) < 1 {
		return nil, errors.New("there are no points in the TimeSeries to add")
	}

	return &emptypb.Empty{}, nil
}

func (*FakeMetricServer) ListTimeSeries(ctx context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	response := &monitoringpb.ListTimeSeriesResponse{}

	return response, nil
}
