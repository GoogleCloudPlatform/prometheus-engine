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
	"testing"

	metric_pb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

func TestPool(t *testing.T) {
	p := newPool(nil)

	ts1 := &monitoring_pb.TimeSeries{
		Resource: &monitoredres_pb.MonitoredResource{
			Labels: map[string]string{
				"resouce_k1":  "resource_v1",
				"resource_k2": "resource_v2",
			},
		},
		Metric: &metric_pb.Metric{
			Type: "metric1",
			Labels: map[string]string{
				"metric_k1": "metric_v1",
				"metric_k2": "metric_v2",
			},
		},
	}
	ts2 := &monitoring_pb.TimeSeries{
		Resource: &monitoredres_pb.MonitoredResource{
			Labels: map[string]string{
				"resource_k2": "resource_v2",
				"resource_k3": "resource_v3",
			},
		},
		Metric: &metric_pb.Metric{
			Type: "metric2",
			Labels: map[string]string{
				"metric_k1": "metric_v1",
				"metric_k2": "metric_v2",
			},
		},
	}

	// Intern two series with partially repeated strings and label sets.
	p.intern(ts1)
	p.intern(ts2)

	if want, got := 12, len(p.strings); want != got {
		t.Errorf("Expected %d unique strings to be interned but got %d. All entries: %v", want, got, p.strings)
	}
	if want, got := 3, len(p.labels); want != got {
		t.Errorf("Expected %d unique label sets to be interned but got %d. All entries: %v", want, got, p.labels)
	}

	// Releasing must only drop instances where the ref count went to 0.
	// This should be ts1's resource labels and strings metric1, resource_k1, resource_v2.
	p.release(ts1)

	if want, got := 9, len(p.strings); want != got {
		t.Errorf("Expected %d unique strings to be interned but got %d. All entries: %v", want, got, p.strings)
	}
	if want, got := 2, len(p.labels); want != got {
		t.Errorf("Expected %d unique label sets to be interned but got %d. All entries: %v", want, got, p.labels)
	}

	// Pool should be completely empty after removing last series.
	p.release(ts2)

	if want, got := 0, len(p.strings); want != got {
		t.Errorf("Expected %d unique strings to be interned but got %d. All entries: %v", want, got, p.strings)
	}
	if want, got := 0, len(p.labels); want != got {
		t.Errorf("Expected %d unique label sets to be interned but got %d. All entries: %v", want, got, p.labels)
	}
}
