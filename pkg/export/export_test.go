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
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	timestamp_pb "google.golang.org/protobuf/types/known/timestamppb"
	"k8s.io/apimachinery/pkg/util/wait"
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
	exporterOpts := ExporterOpts{DisableAuth: true}
	exporterOpts.DefaultUnsetFields()
	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, exporterOpts, NopLease())
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

func (srv *testMetricService) CreateTimeSeries(_ context.Context, req *monitoring_pb.CreateTimeSeriesRequest, _ ...gax.CallOption) error {
	srv.samples = append(srv.samples, req.TimeSeries...)
	return nil
}

func (srv *testMetricService) Close() error {
	return nil
}

func (srv *testMetricService) clear() {
	srv.samples = []*monitoring_pb.TimeSeries{}
}

func TestExporter_drainBacklog(t *testing.T) {
	ctx := context.Background()

	exporterOpts := ExporterOpts{DisableAuth: true}
	exporterOpts.DefaultUnsetFields()
	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, exporterOpts, NopLease())
	if err != nil {
		t.Fatalf("Creating Exporter failed: %s", err)
	}
	metricServer := testMetricService{}
	e.metricClient = &metricServer

	e.SetLabelsByIDFunc(func(storage.SeriesRef) labels.Labels {
		return labels.FromStrings("project_id", "test", "location", "test")
	})

	// Fill a single shard with samples.
	wantSamples := 50
	for i := range wantSamples {
		e.Export(nil, []record.RefSample{
			{Ref: 1, T: int64(i), V: float64(i)},
		}, nil)
	}

	//nolint:errcheck
	go e.Run()
	// As our samples are all for the same series, each batch can only contain a single sample.
	// The exporter waits for the batch delay duration before sending it.
	// We sleep for an appropriate multiple of it to allow it to drain the shard.
	ctxTimeout, cancel := context.WithTimeout(ctx, 60*batchDelayMax)
	defer cancel()

	pollErr := wait.PollUntilContextCancel(ctxTimeout, batchDelayMax, false, func(_ context.Context) (bool, error) {
		// Check that we received all samples that went in.
		if got, want := len(metricServer.samples), wantSamples; got != want {
			err = fmt.Errorf("got %d, want %d", got, want)
			return false, nil
		}
		return true, nil
	})
	if pollErr != nil {
		if wait.Interrupted(pollErr) && err != nil {
			pollErr = err
		}
		t.Fatalf("did not get samples: %s", pollErr)
	}
}

func TestApplyConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*30)
	defer cancel()

	exporterOpts := ExporterOpts{DisableAuth: true}
	exporterOpts.DefaultUnsetFields()
	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, exporterOpts, NopLease())
	if err != nil {
		t.Fatalf("Create exporter: %s", err)
	}
	e.SetLabelsByIDFunc(func(storage.SeriesRef) labels.Labels {
		return labels.FromStrings("location", "us-central1-c")
	})

	metricServer := testMetricService{}
	e.newMetricClient = func(_ context.Context, _ ExporterOpts) (metricServiceClient, error) {
		return &metricServer, nil
	}
	// Sends a sample with no labels. The project label is automatically added by the
	// exporter.
	sendSample := func() {
		e.Export(nil, []record.RefSample{{Ref: 1, T: int64(0), V: float64(0)}}, nil)
	}
	// Tests all samples have the correct project ID label value.
	testSamples := func(expectedProjectID string, expectedSampleCount int) {
		var err error
		pollErr := wait.PollUntilContextCancel(ctx, batchDelayMax, false, func(_ context.Context) (bool, error) {
			switch len(metricServer.samples) {
			case 0:
				err = errors.New("no samples sent")
				return false, nil
			case expectedSampleCount:
				// Good.
			default:
				// Sometimes there's a small delay from the thread that sends the new
				// samples, so let's wait.
				err = fmt.Errorf("expected %d samples but got %d", expectedSampleCount, len(metricServer.samples))
				return false, nil
			}

			for _, sample := range metricServer.samples {
				projectID := sample.Resource.Labels[KeyProjectID]
				if projectID != expectedProjectID {
					err = fmt.Errorf("expected project ID %q but got %q", expectedProjectID, projectID)
					return false, nil
				}
			}

			return true, nil
		})
		if pollErr != nil {
			if wait.Interrupted(pollErr) && err != nil {
				pollErr = err
			}
			t.Fatalf("did not get samples: %s", pollErr)
		}
	}
	sendAndTestSamples := func(expectedProjectID string) {
		// Send two samples to ensure both have correct labels.
		sendSample()
		sendSample()
		sendSample()
		testSamples(expectedProjectID, 3)
		metricServer.clear()
	}

	// In our Prometheus fork, GCM is executed before the reloader in the run group.
	go func() {
		if err := e.Run(); err != nil {
			t.Errorf("Run exporter: %s", err)
		}
	}()

	opts := ExporterOpts{ProjectID: "project-test"}
	opts.DefaultUnsetFields()
	if err := e.ApplyConfig(&config.Config{}, &opts); err != nil {
		t.Fatalf("Initial apply: %s", err)
	}
	sendAndTestSamples("project-test")

	opts = ExporterOpts{ProjectID: "project-abc"}
	opts.DefaultUnsetFields()
	if err := e.ApplyConfig(&config.Config{}, &opts); err != nil {
		t.Fatalf("Initial apply: %s", err)
	}
	sendAndTestSamples("project-abc")

	opts = ExporterOpts{ProjectID: "project-xyz"}
	opts.DefaultUnsetFields()
	if err := e.ApplyConfig(&config.Config{}, &opts); err != nil {
		t.Fatalf("Initial apply: %s", err)
	}
	sendAndTestSamples("project-xyz")
}

func TestDisabledExporter(t *testing.T) {
	// Since on certain environments (e.g. Google-developer machines), we can't emulate a non-GCE
	// environment, we instead set invalid an invalid credential path to emulate no credentials.
	t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "does-not-exist.json")

	ctx := context.Background()
	exporterOpts := ExporterOpts{}
	exporterOpts.DefaultUnsetFields()

	// The default exporter will look for authentication.
	if _, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, exporterOpts, NopLease()); err == nil {
		t.Fatal("Expected error but got none")
	}

	// When we disable the exporter, it doesn't matter if we have credentials or not.
	exporterOpts.Disable = true
	e, err := New(ctx, log.NewJSONLogger(log.NewSyncWriter(os.Stderr)), nil, exporterOpts, NopLease())
	if err != nil {
		t.Fatalf("Run exporter: %s", err)
	}

	// In our Prometheus fork, GCM is executed before the reloader in the run group.
	go func() {
		if err := e.Run(); err != nil {
			t.Errorf("Run exporter: %s", err)
		}
	}()
	opts := ExporterOpts{
		Disable:   true,
		ProjectID: "project-test",
	}
	opts.DefaultUnsetFields()
	if err := e.ApplyConfig(&config.Config{}, &opts); err != nil {
		t.Fatalf("Initial apply: %s", err)
	}
	e.Export(nil, []record.RefSample{{Ref: 1, T: int64(0), V: float64(0)}}, nil)

	// Allow samples to be sent to the void. If we don't panic, we're good.
	time.Sleep(batchDelayMax)
}
