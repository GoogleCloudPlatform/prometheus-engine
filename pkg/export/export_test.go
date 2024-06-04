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
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"google.golang.org/api/option"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	empty_pb "google.golang.org/protobuf/types/known/emptypb"
	timestamp_pb "google.golang.org/protobuf/types/known/timestamppb"
)

func TestBatchAdd(t *testing.T) {
	b := newBatch(nil, DefaultShardCount, 100)

	if !b.empty() {
		t.Fatalf("batch unexpectedly not empty")
	}
	// Add 99 samples per project across 10 projects. The batch should not be full at
	// any point and never be empty after adding the first sample.
	for i := range 10 {
		for range 99 {
			if b.full() {
				t.Fatalf("batch unexpectedly full")
			}
			b.add(&monitoring_pb.TimeSeries{
				Resource: &monitoredres_pb.MonitoredResource{
					Labels: map[string]string{
						KeyProjectID: fmt.Sprintf("project-%d", i),
					},
				},
			})
			if b.empty() {
				t.Fatalf("batch unexpectedly empty")
			}
		}
	}
	if b.full() {
		t.Fatalf("batch unexpectedly full")
	}

	// Adding one more sample to one of the projects should make the batch be full.
	b.add(&monitoring_pb.TimeSeries{
		Resource: &monitoredres_pb.MonitoredResource{
			Labels: map[string]string{
				KeyProjectID: fmt.Sprintf("project-%d", 5),
			},
		},
	})
	if !b.full() {
		t.Fatalf("batch unexpectedly not full")
	}
}

func TestBatchFillFromShardsAndSend(t *testing.T) {
	// Fill the batch from 100 shards with samples across 100 projects.
	var shards []*shard
	for range 100 {
		shards = append(shards, newShard(10000))
	}
	for i := range 10000 {
		shards[i%100].enqueue(uint64(i), &monitoring_pb.TimeSeries{
			Resource: &monitoredres_pb.MonitoredResource{
				Labels: map[string]string{
					KeyProjectID: fmt.Sprintf("project-%d", i%100),
				},
			},
		})
	}

	b := newBatch(nil, DefaultShardCount, 101)

	for _, s := range shards {
		s.fill(b)

		if !s.pending {
			t.Fatalf("shard unexpectedly not pending after fill")
		}
	}

	var mtx sync.Mutex
	receivedSamples := 0

	// When sending the batch we should see the right number of samples and all shards we pass should
	// be notified at the end.
	sendOne := func(_ context.Context, req *monitoring_pb.CreateTimeSeriesRequest, _ ...gax.CallOption) error {
		mtx.Lock()
		receivedSamples += len(req.TimeSeries)
		mtx.Unlock()
		return nil
	}
	b.send(context.Background(), sendOne)

	if want := 10000; receivedSamples != want {
		t.Fatalf("unexpected number of received samples (want=%d, got=%d)", want, receivedSamples)
	}
	for _, s := range shards {
		if s.pending {
			t.Fatalf("shard unexpectedtly pending after send")
		}
	}
}

func TestSampleInRange(t *testing.T) {
	cases := []struct {
		interval   monitoring_pb.TimeInterval
		start, end time.Time
		want       bool
	}{
		{
			interval: monitoring_pb.TimeInterval{
				EndTime: &timestamp_pb.Timestamp{Seconds: 100},
			},
			start: time.Unix(100, 0),
			end:   time.Unix(100, 0),
			want:  true,
		}, {
			interval: monitoring_pb.TimeInterval{
				EndTime: &timestamp_pb.Timestamp{Seconds: 100},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  true,
		}, {
			interval: monitoring_pb.TimeInterval{
				EndTime: &timestamp_pb.Timestamp{Seconds: 101},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  false,
		}, {
			interval: monitoring_pb.TimeInterval{
				StartTime: &timestamp_pb.Timestamp{Seconds: 90},
				EndTime:   &timestamp_pb.Timestamp{Seconds: 100},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  true,
		}, {
			interval: monitoring_pb.TimeInterval{
				StartTime: &timestamp_pb.Timestamp{Seconds: 89},
				EndTime:   &timestamp_pb.Timestamp{Seconds: 100},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  false,
		}, {
			interval: monitoring_pb.TimeInterval{
				StartTime: &timestamp_pb.Timestamp{Seconds: 90},
				EndTime:   &timestamp_pb.Timestamp{Seconds: 101},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  false,
		}, {
			interval: monitoring_pb.TimeInterval{
				StartTime: &timestamp_pb.Timestamp{Seconds: 89},
				EndTime:   &timestamp_pb.Timestamp{Seconds: 101},
			},
			start: time.Unix(90, 0),
			end:   time.Unix(100, 0),
			want:  false,
		},
	}
	//nolint:govet
	for _, c := range cases {
		p := &monitoring_pb.TimeSeries{
			Points: []*monitoring_pb.Point{
				{Interval: &c.interval},
			},
		}
		if ok := sampleInRange(p, c.start, c.end); ok != c.want {
			t.Errorf("expected sample in range %v, got %v", c.want, ok)
		}
	}
}

func TestExporter_wrapMetadata(t *testing.T) {
	cases := []struct {
		desc   string
		mf     MetadataFunc
		metric string
		want   MetricMetadata
		wantOK bool
	}{
		{
			desc:   "nil MetadataFunc always defaults to gauge",
			mf:     nil,
			metric: "some_metric",
			want:   MetricMetadata{Metric: "some_metric", Type: textparse.MetricTypeGauge},
			wantOK: true,
		}, {
			desc:   "nil MetadataFunc preserves synthetic metric metadata",
			mf:     nil,
			metric: "up",
			want: MetricMetadata{
				Metric: "up",
				Type:   textparse.MetricTypeGauge,
				Help:   "Up indicates whether the last target scrape was successful.",
			},
			wantOK: true,
		}, {
			desc: "synthetic metric metadata precedence",
			mf: func(string) (MetricMetadata, bool) {
				return MetricMetadata{
					Metric: "up",
					Type:   textparse.MetricTypeCounter,
				}, false
			},
			metric: "up",
			want: MetricMetadata{
				Metric: "up",
				Type:   textparse.MetricTypeGauge,
				Help:   "Up indicates whether the last target scrape was successful.",
			},
			wantOK: true,
		}, {
			desc: "regular metadata is returned as is",
			mf: func(string) (MetricMetadata, bool) {
				return MetricMetadata{
					Metric: "some_metric",
					Type:   textparse.MetricTypeCounter,
					Help:   "useful help",
				}, true
			},
			metric: "some_metric",
			want: MetricMetadata{
				Metric: "some_metric",
				Type:   textparse.MetricTypeCounter,
				Help:   "useful help",
			},
			wantOK: true,
		}, {
			desc: "not found metadata defaults to untyped",
			mf: func(string) (MetricMetadata, bool) {
				return MetricMetadata{}, false
			},
			metric: "some_metric",
			want: MetricMetadata{
				Metric: "some_metric",
				Type:   textparse.MetricTypeUnknown,
			},
			wantOK: true,
		}, {
			desc: "not found metadata returns false if base name has metadata (_sum)",
			mf: func(m string) (MetricMetadata, bool) {
				if m == "foo" {
					return MetricMetadata{Metric: "foo", Type: textparse.MetricTypeSummary}, true
				}
				return MetricMetadata{}, false
			},
			metric: "foo_sum",
			want:   MetricMetadata{},
			wantOK: false,
		}, {
			desc: "not found metadata returns false if base name has metadata (_bucket)",
			mf: func(m string) (MetricMetadata, bool) {
				if m == "foo" {
					return MetricMetadata{Metric: "foo", Type: textparse.MetricTypeSummary}, true
				}
				return MetricMetadata{}, false
			},
			metric: "foo_bucket",
			want:   MetricMetadata{},
			wantOK: false,
		}, {
			desc: "not found metadata returns false if base name has metadata (_count)",
			mf: func(m string) (MetricMetadata, bool) {
				if m == "foo" {
					return MetricMetadata{Metric: "foo", Type: textparse.MetricTypeSummary}, true
				}
				return MetricMetadata{}, false
			},
			metric: "foo_count",
			want:   MetricMetadata{},
			wantOK: false,
		},
	}

	ctx := context.Background()
	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, ExporterOpts{DisableAuth: true}, NopLease())
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range cases {
		t.Run(c.desc, func(t *testing.T) {
			got, ok := e.wrapMetadata(c.mf)(c.metric)
			if ok != c.wantOK {
				t.Fatalf("MetadataFunc unexpectedly ok=%v, want ok=%v", ok, c.wantOK)
			}
			if diff := cmp.Diff(c.want, got); diff != "" {
				t.Fatalf("unexpected metadata (-want,+got): %s", diff)
			}
		})
	}
}

type testMetricService struct {
	monitoring_pb.MetricServiceServer // Inherit all interface methods
	samples                           []*monitoring_pb.TimeSeries
}

func (srv *testMetricService) CreateTimeSeries(_ context.Context, req *monitoring_pb.CreateTimeSeriesRequest) (*empty_pb.Empty, error) {
	srv.samples = append(srv.samples, req.TimeSeries...)
	return &empty_pb.Empty{}, nil
}

func TestExporter_drainBacklog(t *testing.T) {
	var (
		srv          = grpc.NewServer()
		listener     = bufconn.Listen(1e6)
		metricServer = &testMetricService{}
	)
	monitoring_pb.RegisterMetricServiceServer(srv, metricServer)

	//nolint:errcheck
	go srv.Serve(listener)
	defer srv.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bufDialer := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	metricClient, err := monitoring.NewMetricClient(ctx,
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithContextDialer(bufDialer)),
	)
	if err != nil {
		t.Fatalf("Creating metric client failed: %s", err)
	}

	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, ExporterOpts{DisableAuth: true}, NopLease())
	if err != nil {
		t.Fatalf("Creating Exporter failed: %s", err)
	}
	e.metricClient = metricClient

	e.SetLabelsByIDFunc(func(storage.SeriesRef) labels.Labels {
		return labels.FromStrings("project_id", "test", "location", "test")
	})

	// Fill a single shard with samples.
	for i := range 50 {
		e.Export(nil, []record.RefSample{
			{Ref: 1, T: int64(i), V: float64(i)},
		}, nil)
	}

	//nolint:errcheck
	go e.Run(ctx)
	// As our samples are all for the same series, each batch can only contain a single sample.
	// The exporter waits for the batch delay duration before sending it.
	// We sleep for an appropriate multiple of it to allow it to drain the shard.
	time.Sleep(55 * batchDelayMax)

	// Check that we received all samples that went in.
	if got, want := len(metricServer.samples), 50; got != want {
		t.Fatalf("got %d, want %d", got, want)
	}
}
