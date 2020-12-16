package export

import (
	"reflect"
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
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
