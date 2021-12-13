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
	"sync"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
)

// Storage provides a stateful wrapper around an Exporter that implements
// Prometheus's storage interface (Appendable).
//
// For performance reasons Exporter is optimized to be tightly integrate with
// Prometheus's storage. This makes it rely on external state (series ID to label
// mapping).
// For use cases where a full Prometheus storage engine is not present (e.g. rule
// evaluation service), Storage acts as a simple drop-in replacement that directly
// manages the state required by Exporter.
type Storage struct {
	exporter *Exporter

	mtx    sync.Mutex
	labels map[uint64]labels.Labels
}

// NewStorage returns a new Prometheus storage that's exporting data via the exporter.
func NewStorage(exporter *Exporter) *Storage {
	s := &Storage{
		exporter: exporter,
		labels:   map[uint64]labels.Labels{},
	}
	exporter.SetLabelsByIDFunc(s.labelsByID)

	return s
}

// ApplyConfig applies the new configuration to the storage.
func (s *Storage) ApplyConfig(cfg *config.Config) error {
	return s.exporter.ApplyConfig(cfg)
}

// Run background processing of the storage.
func (s *Storage) Run(ctx context.Context) error {
	return s.exporter.Run(ctx)
}

func (s *Storage) labelsByID(id uint64) labels.Labels {
	s.mtx.Lock()
	lset := s.labels[id]
	s.mtx.Unlock()
	return lset
}

func (s *Storage) setLabels(lset labels.Labels) uint64 {
	h := lset.Hash()
	s.mtx.Lock()
	s.labels[h] = lset
	s.mtx.Unlock()
	return h
}

func (s *Storage) clearLabels(samples []record.RefSample) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	for _, sample := range samples {
		delete(s.labels, sample.Ref)
	}
}

// Appender returns a new Appender.
func (s *Storage) Appender(ctx context.Context) storage.Appender {
	return &storageAppender{
		storage: s,
		samples: make([]record.RefSample, 0, 64),
	}
}

type storageAppender struct {
	// Make sure all Appender methods are implemented at compile time. Panics
	// are expected and intended if a method is used unexpectedly.
	storage.Appender

	storage *Storage
	samples []record.RefSample
}

func (a *storageAppender) Append(_ uint64, lset labels.Labels, t int64, v float64) (uint64, error) {
	if lset == nil {
		return 0, errors.Errorf("label set is nil")
	}
	a.samples = append(a.samples, record.RefSample{
		Ref: a.storage.setLabels(lset),
		T:   t,
		V:   v,
	})
	// Return 0 ID to indicate that we don't support fast path appending.
	return 0, nil
}

func (a *storageAppender) Commit() error {
	// This method is used to export rule results. It's generally safe to assume that
	// they are of type gauge. Thus we pass in a metadata func that always returns the
	// gauge type.
	// In the future we may want to populate the help text with information on the rule
	// that produced the metric.
	a.storage.exporter.Export(gaugeMetadata, a.samples)

	// After export is complete, we can clear the labels again.
	a.storage.clearLabels(a.samples)

	return nil
}
