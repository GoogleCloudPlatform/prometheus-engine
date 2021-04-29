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
	"fmt"
	"sync"

	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// shard holds a queue of data for a subset of samples.
type shard struct {
	mtx     sync.Mutex
	queue   *queue
	pending bool

	// A cache of series IDs that have been added to the batch in fill already.
	// It's only part of the struct to not re-allocate on each call to fill.
	seen map[uint64]struct{}
}

func newShard(queueSize int) *shard {
	return &shard{
		queue: newQueue(queueSize),
		seen:  map[uint64]struct{}{},
	}
}

func (s *shard) enqueue(hash uint64, sample *monitoring_pb.TimeSeries) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	samplesExported.Inc()

	e := queueEntry{
		hash:   hash,
		sample: sample,
	}
	if !s.queue.add(e) {
		// TODO(freinartz): tail drop is not a great solution. Once we have the WAL buffer,
		// we can just block here when enqueueing from it.
		samplesDropped.Inc()
	}
}

// fill adds samples to the batch until its capacity is reached or the shard
// has no more samples for series that are not in the batch yet.
func (s *shard) fill(batch *[]*monitoring_pb.TimeSeries) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	shardProcess.Inc()

	if s.pending {
		shardProcessPending.Inc()
		return 0
	}
	n := 0

	for len(*batch) < cap(*batch) {
		e, ok := s.queue.peek()
		if !ok {
			break
		}
		// If we already added a sample for the same series to the batch, stop
		// the filling entirely.
		if _, ok := s.seen[e.hash]; ok {
			break
		}
		s.queue.remove()

		*batch = append(*batch, e.sample)
		s.seen[e.hash] = struct{}{}
		n++
	}

	if n > 0 {
		s.setPending(true)
		shardProcessSamplesTaken.Observe(float64(n))
	}
	// Clear seen cache. Because the shard is now pending, we won't add any more data
	// to the batch, even if fill was called again.
	for k := range s.seen {
		delete(s.seen, k)
	}
	return n
}

func (s *shard) setPending(b bool) {
	// This case should never happen in our usage of shards unless there is a bug.
	if s.pending == b {
		panic(fmt.Sprintf("pending set to %v while it already was", b))
	}
	s.pending = b
}

func (s *shard) notifyBatchDone() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.setPending(false)
}

type queue struct {
	buf        []queueEntry
	head, tail int
	len        int
}

type queueEntry struct {
	hash   uint64
	sample *monitoring_pb.TimeSeries
}

func newQueue(size int) *queue {
	return &queue{buf: make([]queueEntry, size)}
}

func (q *queue) length() int {
	return q.len
}

func (q *queue) add(e queueEntry) bool {
	if q.len == len(q.buf) {
		return false
	}
	q.buf[q.tail] = e
	q.tail = (q.tail + 1) % len(q.buf)
	q.len++

	return true
}

func (q *queue) peek() (queueEntry, bool) {
	if q.len < 1 {
		return queueEntry{}, false
	}
	return q.buf[q.head], true
}

func (q *queue) remove() bool {
	if q.len < 1 {
		return false
	}
	q.buf[q.head] = queueEntry{} // resetting makes debugging easier
	q.head = (q.head + 1) % len(q.buf)
	q.len--

	return true
}
