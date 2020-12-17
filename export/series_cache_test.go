package export

import (
	"reflect"
	"testing"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
)

func TestSeriesCache_extractResource(t *testing.T) {
	cases := []struct {
		doc            string
		externalLabels labels.Labels
		seriesLabels   labels.Labels
		wantResource   *monitoredres_pb.MonitoredResource
		wantLabels     labels.Labels
		wantOk         bool
	}{
		{
			doc: "job label must be set",
			seriesLabels: labels.FromMap(map[string]string{
				"location":  "l1",
				"cluster":   "c1",
				"namespace": "n1",
				"instance":  "i1",
				"key":       "v1",
			}),
			wantOk: false,
		}, {
			doc: "instance label must be set",
			seriesLabels: labels.FromMap(map[string]string{
				"location":  "l1",
				"cluster":   "c1",
				"namespace": "n1",
				"job":       "j1",
				"key":       "v1",
			}),
			wantOk: false,
		}, {
			doc: "everything contained in series labels",
			seriesLabels: labels.FromMap(map[string]string{
				"location":  "l1",
				"cluster":   "c1",
				"namespace": "n1",
				"job":       "j1",
				"instance":  "i1",
				"key":       "v1",
			}),
			wantResource: &monitoredres_pb.MonitoredResource{
				Type: "prometheus_target",
				Labels: map[string]string{
					"location":  "l1",
					"cluster":   "c1",
					"namespace": "n1",
					"job":       "j1",
					"instance":  "i1",
				},
			},
			wantLabels: labels.FromStrings("key", "v1"),
			wantOk:     true,
		}, {
			doc: "some target and metric labels through external labels",
			externalLabels: labels.FromMap(map[string]string{
				"location": "l1",
				"cluster":  "c1",
				"key1":     "v1",
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
					"location":  "l1",
					"cluster":   "c2",
					"namespace": "n1",
					"job":       "j1",
					"instance":  "i1",
				},
			},
			wantLabels: labels.FromStrings("key1", "v1", "key2", "v2"),
			wantOk:     true,
		},
	}
	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			cache := newSeriesCache(nil, func() labels.Labels {
				return c.externalLabels
			})
			resource, lset, ok := cache.extractResource(c.seriesLabels)
			if c.wantOk != ok {
				t.Errorf("expected 'ok' to be %v but got %v", c.wantOk, ok)
			}
			if !reflect.DeepEqual(c.wantResource, resource) {
				t.Errorf("expected resource %+v but got %+v", c.wantResource, resource)
			}
			if !labels.Equal(c.wantLabels, lset) {
				t.Errorf("expected metric labels %q but got %q", c.wantLabels, lset)
			}
		})
	}
}

func TestSeriesCache_garbageCollect(t *testing.T) {
	cache := newSeriesCache(nil, nil)
	// Always return empty labels. This will cause cache entries to be added but not populated,
	// which we don't need to test garbage collection.
	cache.getLabelsByRef = func(uint64) labels.Labels { return nil }

	// Fake now second timestamp.
	now := int64(100000)
	cache.now = func() time.Time { return time.Unix(now, 0) }

	// Populate some cache entries. Timestamps are converted to milliseconds.
	cache.get(record.RefSample{Ref: 1, T: (now - 100) * 1000}, nil)
	cache.get(record.RefSample{Ref: 2, T: (now - 101) * 1000}, nil)

	cache.garbageCollect(100 * time.Second)

	// Entry for series 1 should remain while 2 got dropped.
	if len(cache.entries) != 1 {
		t.Errorf("Expected exactly one cache entry left, but cache is %v", cache.entries)
	}
	if _, ok := cache.entries[1]; !ok {
		t.Errorf("Expected cache entry for series 1 but cache is %v", cache.entries)
	}
}
