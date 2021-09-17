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
	"hash/fnv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var (
	poolIntern = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gcm_pool_intern_total",
			Help: "Time series memory intern operations.",
		},
	)
	poolRelease = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gcm_pool_release_total",
			Help: "Time series memory intern release operations.",
		},
	)
)

// pool holds interned strings and label sets to deduplicate memory across
// cached monitoring_pb.TimeSeries entries.
type pool struct {
	mtx     sync.Mutex
	strings map[string]*stringEntry
	labels  map[uint64]*labelsEntry
}

func newPool(reg prometheus.Registerer) *pool {
	if reg != nil {
		reg.MustRegister(poolIntern, poolRelease)
	}
	return &pool{
		strings: map[string]*stringEntry{},
		labels:  map[uint64]*labelsEntry{},
	}
}

type stringEntry struct {
	refs uint32
	s    string
}

type labelsEntry struct {
	refs   uint32
	labels map[string]string
}

// intern the strings and label sets in ts.
func (p *pool) intern(ts *monitoring_pb.TimeSeries) {
	if ts == nil {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()

	poolIntern.Inc()

	ts.Resource.Labels = p.internLabels(ts.Resource.Labels)
	ts.Metric.Type = p.internString(ts.Metric.Type)
	ts.Metric.Labels = p.internLabels(ts.Metric.Labels)
}

func (p *pool) internString(s string) string {
	e, ok := p.strings[s]
	if !ok {
		e = &stringEntry{s: s}
		p.strings[e.s] = e
	}
	e.refs++
	return e.s
}

func (p *pool) labelSum(lset map[string]string) uint64 {
	h := fnv.New64a()
	hashLabels(h, lset)
	return h.Sum64()
}

func (p *pool) internLabels(lset map[string]string) map[string]string {
	hsum := p.labelSum(lset)

	e, ok := p.labels[hsum]
	if !ok {
		for k, v := range lset {
			delete(lset, k)
			// In general Prometheus already optimizes label string allocations
			// and interning them won't do much in practice. But the overhead of doing it
			// is negligible and it decouples ous from Prometheus' implementation details
			// to be more robust to future changes.
			lset[p.internString(k)] = p.internString(v)
		}
		e = &labelsEntry{labels: lset}
		p.labels[hsum] = e
	}
	e.refs++
	return e.labels
}

// release pooled memory interned for ts.
func (p *pool) release(ts *monitoring_pb.TimeSeries) {
	// When first populating a cache entry, we may call this with an unset series.
	if ts == nil {
		return
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()

	poolRelease.Inc()

	p.releaseLabels(ts.Resource.Labels)
	p.releaseString(ts.Metric.Type)
	p.releaseLabels(ts.Metric.Labels)
}

func (p *pool) releaseString(s string) {
	e, ok := p.strings[s]
	if !ok {
		panic(fmt.Sprintf("release of non-interned string %q", s))
	}
	e.refs--
	if e.refs == 0 {
		delete(p.strings, s)
	}
}

func (p *pool) releaseLabels(lset map[string]string) {
	hsum := p.labelSum(lset)

	e, ok := p.labels[hsum]
	if !ok {
		panic(fmt.Sprintf("release of non-interned labels %s", lset))
	}
	e.refs--
	if e.refs == 0 {
		for k, v := range e.labels {
			p.releaseString(k)
			p.releaseString(v)
		}
		delete(p.labels, hsum)
	}
}
