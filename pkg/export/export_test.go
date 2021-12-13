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
	"sync"
	"testing"
	"time"

	timestamp_pb "github.com/golang/protobuf/ptypes/timestamp"
	gax "github.com/googleapis/gax-go/v2"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

func TestBatchAdd(t *testing.T) {
	b := newBatch(nil, 100)

	if !b.empty() {
		t.Fatalf("batch unexpectedly not empty")
	}
	// Add 99 samples per project across 10 projects. The batch should not be full at
	// any point and never be empty after adding the first sample.
	for i := 0; i < 10; i++ {
		for j := 0; j < 99; j++ {
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
	for i := 0; i < 100; i++ {
		shards = append(shards, newShard(10000))
	}
	for i := 0; i < 10000; i++ {
		shards[i%100].enqueue(uint64(i), &monitoring_pb.TimeSeries{
			Resource: &monitoredres_pb.MonitoredResource{
				Labels: map[string]string{
					KeyProjectID: fmt.Sprintf("project-%d", i%100),
				},
			},
		})
	}

	b := newBatch(nil, 101)

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
	sendOne := func(ctx context.Context, req *monitoring_pb.CreateTimeSeriesRequest, opts ...gax.CallOption) error {
		mtx.Lock()
		receivedSamples += len(req.TimeSeries)
		mtx.Unlock()
		return nil
	}
	b.send(context.Background(), shards, sendOne)

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
