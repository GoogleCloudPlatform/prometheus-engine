package exportv2

import (
	"testing"

	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	writev2 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/model/labels"
)

func TestExportV2TimeSeries(t *testing.T) {
	s := writev2.NewSymbolTable()
	for _, tc := range []struct {
		ts             *writev2.TimeSeries
		expectedPoints []*monitoring_pb.TimeSeries
	}{
		{
			ts: &writev2.TimeSeries{
				LabelsRefs: s.SymbolizeLabels(labels.FromStrings("__name__", "test_gauge1", "foo", "bar1"), nil),
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			var gotPoints []*monitoring_pb.TimeSeries
			exportSelfContainedTimeSeries(tc.ts, func(p *monitoring_pb.TimeSeries) {
				gotPoints = append(gotPoints, p)
			})
		})
	}
}
