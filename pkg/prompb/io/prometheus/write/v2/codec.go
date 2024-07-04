// Copyright 2024 Google LLC
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

// Copyright 2024 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package writev2

import (
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

// NOTE(bwplotka): This file's code is tested in /prompb/rwcommon.

// ToLabels return model labels.Labels from timeseries' remote labels.
func (m *TimeSeries) ToLabels(b *labels.ScratchBuilder, symbols []string) labels.Labels {
	return desymbolizeLabels(b, m.GetLabelsRefs(), symbols)
}

// IsFloatHistogram returns true if the histogram is float.
func (h *Histogram) IsFloatHistogram() bool {
	_, ok := h.GetCount().(*Histogram_CountFloat)
	return ok
}

// ToIntHistogram returns integer Prometheus histogram from the remote implementation
// of integer histogram. If it's a float histogram, the method returns nil.
// TODO(bwplotka): Add support for incoming NHCB.
func (h *Histogram) ToIntHistogram() *histogram.Histogram {
	if h.IsFloatHistogram() {
		return nil
	}
	return &histogram.Histogram{
		CounterResetHint: histogram.CounterResetHint(h.ResetHint),
		Schema:           h.Schema,
		ZeroThreshold:    h.ZeroThreshold,
		ZeroCount:        h.GetZeroCountInt(),
		Count:            h.GetCountInt(),
		Sum:              h.Sum,
		PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
		PositiveBuckets:  h.GetPositiveDeltas(),
		NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
		NegativeBuckets:  h.GetNegativeDeltas(),
	}
}

// ToFloatHistogram returns float Prometheus histogram from the remote implementation
// of float histogram. If the underlying implementation is an integer histogram, a
// conversion is performed.
// TODO(bwplotka): Add support for incoming NHCB.
func (h *Histogram) ToFloatHistogram() *histogram.FloatHistogram {
	if h.IsFloatHistogram() {
		return &histogram.FloatHistogram{
			CounterResetHint: histogram.CounterResetHint(h.ResetHint),
			Schema:           h.Schema,
			ZeroThreshold:    h.ZeroThreshold,
			ZeroCount:        h.GetZeroCountFloat(),
			Count:            h.GetCountFloat(),
			Sum:              h.Sum,
			PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
			PositiveBuckets:  h.GetPositiveCounts(),
			NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
			NegativeBuckets:  h.GetNegativeCounts(),
		}
	}
	// Conversion from integer histogram.
	return &histogram.FloatHistogram{
		CounterResetHint: histogram.CounterResetHint(h.ResetHint),
		Schema:           h.Schema,
		ZeroThreshold:    h.ZeroThreshold,
		ZeroCount:        float64(h.GetZeroCountInt()),
		Count:            float64(h.GetCountInt()),
		Sum:              h.Sum,
		PositiveSpans:    spansProtoToSpans(h.GetPositiveSpans()),
		PositiveBuckets:  deltasToCounts(h.GetPositiveDeltas()),
		NegativeSpans:    spansProtoToSpans(h.GetNegativeSpans()),
		NegativeBuckets:  deltasToCounts(h.GetNegativeDeltas()),
	}
}

func spansProtoToSpans(s []*BucketSpan) []histogram.Span {
	spans := make([]histogram.Span, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = histogram.Span{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func deltasToCounts(deltas []int64) []float64 {
	counts := make([]float64, len(deltas))
	var cur float64
	for i, d := range deltas {
		cur += float64(d)
		counts[i] = cur
	}
	return counts
}

// FromIntHistogram returns remote Histogram from the integer Histogram.
func FromIntHistogram(timestamp int64, h *histogram.Histogram) Histogram {
	return Histogram{
		Count:          &Histogram_CountInt{CountInt: h.Count},
		Sum:            h.Sum,
		Schema:         h.Schema,
		ZeroThreshold:  h.ZeroThreshold,
		ZeroCount:      &Histogram_ZeroCountInt{ZeroCountInt: h.ZeroCount},
		NegativeSpans:  spansToSpansProto(h.NegativeSpans),
		NegativeDeltas: h.NegativeBuckets,
		PositiveSpans:  spansToSpansProto(h.PositiveSpans),
		PositiveDeltas: h.PositiveBuckets,
		ResetHint:      Histogram_ResetHint(h.CounterResetHint),
		Timestamp:      timestamp,
	}
}

// FromFloatHistogram returns remote Histogram from the float Histogram.
func FromFloatHistogram(timestamp int64, fh *histogram.FloatHistogram) Histogram {
	return Histogram{
		Count:          &Histogram_CountFloat{CountFloat: fh.Count},
		Sum:            fh.Sum,
		Schema:         fh.Schema,
		ZeroThreshold:  fh.ZeroThreshold,
		ZeroCount:      &Histogram_ZeroCountFloat{ZeroCountFloat: fh.ZeroCount},
		NegativeSpans:  spansToSpansProto(fh.NegativeSpans),
		NegativeCounts: fh.NegativeBuckets,
		PositiveSpans:  spansToSpansProto(fh.PositiveSpans),
		PositiveCounts: fh.PositiveBuckets,
		ResetHint:      Histogram_ResetHint(fh.CounterResetHint),
		Timestamp:      timestamp,
	}
}

func spansToSpansProto(s []histogram.Span) []*BucketSpan {
	spans := make([]*BucketSpan, len(s))
	for i := 0; i < len(s); i++ {
		spans[i] = &BucketSpan{Offset: s[i].Offset, Length: s[i].Length}
	}

	return spans
}

func (m *Exemplar) ToExemplar(b *labels.ScratchBuilder, symbols []string) exemplar.Exemplar {
	timestamp := m.Timestamp

	return exemplar.Exemplar{
		Labels: desymbolizeLabels(b, m.LabelsRefs, symbols),
		Value:  m.Value,
		Ts:     timestamp,
		HasTs:  timestamp != 0,
	}
}
