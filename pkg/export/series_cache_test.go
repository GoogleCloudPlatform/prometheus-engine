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
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestExtractResource(t *testing.T) {
	cases := []struct {
		doc            string
		externalLabels labels.Labels
		seriesLabels   labels.Labels
		wantResource   *monitoredres_pb.MonitoredResource
		wantLabels     labels.Labels
		wantOk         bool
	}{
		{
			doc: "everything contained in series labels",
			seriesLabels: labels.FromMap(map[string]string{
				"project_id": "p1",
				"location":   "l1",
				"cluster":    "c1",
				"namespace":  "n1",
				"job":        "j1",
				"instance":   "i1",
				"key":        "v1",
			}),
			wantResource: &monitoredres_pb.MonitoredResource{
				Type: "prometheus_target",
				Labels: map[string]string{
					"project_id": "p1",
					"location":   "l1",
					"cluster":    "c1",
					"namespace":  "n1",
					"job":        "j1",
					"instance":   "i1",
				},
			},
			wantLabels: labels.FromStrings("key", "v1"),
			wantOk:     true,
		},
		{
			doc: "partially contained in series labels",
			seriesLabels: labels.FromMap(map[string]string{
				"project_id": "p1",
				"location":   "l1",
				"namespace":  "n1",
				"instance":   "i1",
				"key":        "v1",
			}),
			wantResource: &monitoredres_pb.MonitoredResource{
				Type: "prometheus_target",
				Labels: map[string]string{
					"project_id": "p1",
					"location":   "l1",
					"cluster":    "",
					"namespace":  "n1",
					"job":        "",
					"instance":   "i1",
				},
			},
			wantLabels: labels.FromStrings("key", "v1"),
			wantOk:     true,
		}, {
			doc: "some target and metric labels through external labels",
			externalLabels: labels.FromMap(map[string]string{
				"project_id": "p1",
				"location":   "l1",
				"cluster":    "c1",
				"key1":       "v1",
			}),
			seriesLabels: labels.FromMap(map[string]string{
				"cluster":   "c2",
				"namespace": "n1",
				"job":       "j1",
				"instance":  "i1",
				"key2":      "v2",
			}),
			wantResource: &monitoredres_pb.MonitoredResource{
				Type: "prometheus_target",
				Labels: map[string]string{
					"project_id": "p1",
					"location":   "l1",
					"cluster":    "c2",
					"namespace":  "n1",
					"job":        "j1",
					"instance":   "i1",
				},
			},
			wantLabels: labels.FromStrings("key1", "v1", "key2", "v2"),
			wantOk:     true,
		}, {
			doc: "location must be set",
			seriesLabels: labels.FromMap(map[string]string{
				"project_id": "p1",
				"cluster":    "c1",
				"namespace":  "n1",
				"job":        "j1",
				"key":        "v1",
			}),
			wantOk: false,
		}, {
			doc: "project_id must be set",
			seriesLabels: labels.FromMap(map[string]string{
				"location":  "l1",
				"cluster":   "c1",
				"namespace": "n1",
				"job":       "j1",
				"key":       "v1",
			}),
			wantOk: false,
		},
	}
	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			resource, lset, err := extractResource(c.externalLabels, c.seriesLabels)
			if c.wantOk && err != nil {
				t.Errorf("expected no error but got: %s", err)
			}
			if !c.wantOk && err == nil {
				t.Errorf("expected error but got none")
			}
			if diff := cmp.Diff(c.wantResource, resource, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected resource (-want, +got): %s", diff)
			}
			if diff := cmp.Diff(c.wantLabels, lset); diff != "" {
				t.Errorf("unexpected labels (-want, +got): %s", diff)
			}
		})
	}
}

func TestSeriesCache_garbageCollect(t *testing.T) {
	cache := newSeriesCache(nil, nil, MetricTypePrefix, nil)
	// Always return empty labels. This will cause cache entries to be added but not populated,
	// which we don't need to test garbage collection.
	cache.getLabelsByRef = func(uint64) labels.Labels { return nil }

	// Fake now second timestamp.
	now := int64(100000)
	cache.now = func() time.Time { return time.Unix(now, 0) }

	// Populate some cache entries. Timestamps are converted to milliseconds.
	cache.get(record.RefSample{Ref: 1, T: (now - 100) * 1000}, nil, nil)
	cache.get(record.RefSample{Ref: 2, T: (now - 101) * 1000}, nil, nil)

	cache.garbageCollect(100 * time.Second)

	// Entry for series 1 should remain while 2 got dropped.
	if len(cache.entries) != 1 {
		t.Errorf("Expected exactly one cache entry left, but cache is %v", cache.entries)
	}
	if _, ok := cache.entries[1]; !ok {
		t.Errorf("Expected cache entry for series 1 but cache is %v", cache.entries)
	}
}
