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
	"testing"

	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

func TestAddTimeSeriesNil(t *testing.T) {
	fms := NewFakeMetricServer(200)
	_, err := fms.CreateTimeSeries(context.TODO(), nil)
	if err == nil {
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
	_, err := fms.CreateTimeSeries(context.TODO(), createTimeSeriesRequest)
	if err == nil {
		t.Error("expected an error when sending more time series than the server allows")
	}
}
