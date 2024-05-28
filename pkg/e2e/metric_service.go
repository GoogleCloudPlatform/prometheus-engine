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
	"errors"
	"fmt"
	"maps"
	"strings"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/protobuf/types/known/emptypb"
)

// https://cloud.google.com/monitoring/api/ref_v3/rest/v3/projects.timeSeries/create
const defaultMaxTimeSeriesPerRequest = 200

func NewFakeMetricServer(db *MetricDatabase) monitoringpb.MetricServiceServer {
	return newFakeMetricServer(db)
}

func newFakeMetricServer(db *MetricDatabase) *fakeMetricServer {
	return &fakeMetricServer{
		db:                      db,
		maxTimeSeriesPerRequest: defaultMaxTimeSeriesPerRequest,
	}
}

type fakeMetricServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	db                      *MetricDatabase
	maxTimeSeriesPerRequest int
}

func (fms *fakeMetricServer) CreateTimeSeries(_ context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, errors.New("nil request")
	}
	if len(req.GetTimeSeries()) < 1 {
		return nil, errors.New("there are no time series to add")
	}
	if amount := len(req.GetTimeSeries()); amount > fms.maxTimeSeriesPerRequest {
		return nil, fmt.Errorf("exceeded the max number of time series, %d vs %d", amount, fms.maxTimeSeriesPerRequest)
	}
	if !strings.HasPrefix(req.GetName(), "projects/") {
		return nil, fmt.Errorf("only projects are supported, found %q", req.GetName())
	}

	err := fms.db.Insert(req.GetName(), req.GetTimeSeries())
	return &emptypb.Empty{}, err
}

func isTimeSeriesSame(left, right *monitoringpb.TimeSeries) bool {
	if left == nil && right == nil {
		return true
	}
	if left == nil || right == nil {
		return false
	}

	leftMetric := left.GetMetric()
	rightMetric := right.GetMetric()
	if leftMetric.GetType() != rightMetric.GetType() || !maps.Equal(leftMetric.GetLabels(), leftMetric.GetLabels()) {
		return false
	}

	leftResource := left.GetResource()
	rightResource := right.GetResource()
	return leftResource.GetType() == rightResource.GetType() && maps.Equal(leftResource.GetLabels(), rightResource.GetLabels())
}

func (fms *fakeMetricServer) ListTimeSeries(_ context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
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

	expressionFilter, err := parseFilter(req.Filter)
	if err != nil {
		return nil, err
	}

	filter := andExpression{
		left:  newIntervalFilter(req.Interval),
		right: expressionFilter,
	}

	timeSeriesToReturn := runFilter(fms.db.Get(req.Name), &filter)
	return &monitoringpb.ListTimeSeriesResponse{
		TimeSeries: timeSeriesToReturn,
	}, nil
}
