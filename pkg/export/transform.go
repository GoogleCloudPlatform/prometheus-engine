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
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/pkg/value"
	"github.com/prometheus/prometheus/tsdb/record"

	timestamp_pb "github.com/golang/protobuf/ptypes/timestamp"
	distribution_pb "google.golang.org/genproto/googleapis/api/distribution"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

var (
	prometheusSamplesDiscarded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcm_prometheus_samples_discarded_total",
			Help: "Samples that were discarded during data model conversion.",
		},
		[]string{"reason"},
	)
)

type sampleBuilder struct {
	series *seriesCache
}

// MetricMetadata is a copy of MetricMetadata in Prometheus's scrape package.
// It is copied to break a dependency cycle.
type MetricMetadata struct {
	Metric string
	Type   textparse.MetricType
	Help   string
	Unit   string
}

// MetadataFunc gets metadata for a specific metric name.
type MetadataFunc func(metric string) (MetricMetadata, bool)

// gaugeMetadata is a MetadataFunc that always returns the gauge type.
// Help and Unit are left empty.
func gaugeMetadata(metric string) (MetricMetadata, bool) {
	return MetricMetadata{
		Metric: metric,
		Type:   textparse.MetricTypeGauge,
	}, true
}

// withScrapeMetricMetadata wraps a MetadataFunc and additionally returns metadata
// about Prometheues's synthetic scrape-time metrics.
func withScrapeMetricMetadata(f MetadataFunc) MetadataFunc {
	// Metrics Prometheus writes at scrape time for which no metadata is exposed.
	internalMetrics := map[string]MetricMetadata{
		"up": {
			Metric: "up",
			Type:   textparse.MetricTypeGauge,
			Help:   "Up indicates whether the last target scrape was successful.",
		},
		"scrape_samples_scraped": {
			Metric: "scrape_samples_scraped",
			Type:   textparse.MetricTypeGauge,
			Help:   "How many samples were scraped during the last successful scrape.",
		},
		"scrape_duration_seconds": {
			Metric: "scrape_duration_seconds",
			Type:   textparse.MetricTypeGauge,
			Help:   "Duration of the last scrape.",
		},
		"scrape_samples_post_metric_relabeling": {
			Metric: "scrape_samples_post_metric_relabeling",
			Type:   textparse.MetricTypeGauge,
			Help:   "How many samples were ingested after relabeling.",
		},
		"scrape_series_added": {
			Metric: "scrape_series_added",
			Type:   textparse.MetricTypeGauge,
			Help:   "Number of new series added in the last scrape.",
		},
	}
	// Metadata is nil for metrics ingested through recording or alerting rules.
	// Unless the rule literally does no processing at all, this always means the
	// resulting data is a gauge.
	// This makes it safe to assume a gauge type here in the absence of any other
	// metadata.
	// In the future we might want to propagate the rule definition and add it as
	// help text here to easily understand what produced the metric.
	if f == nil {
		f = gaugeMetadata
	}
	return func(metric string) (MetricMetadata, bool) {
		md, ok := internalMetrics[metric]
		if ok {
			return md, true
		}
		return f(metric)
	}
}

// next extracts the next sample from the input sample batch and returns
// the remainder of the input.
// Returns a nil time series for samples that couldn't be converted.
func (b *sampleBuilder) next(metadata MetadataFunc, samples []record.RefSample) (*monitoring_pb.TimeSeries, uint64, []record.RefSample, error) {
	sample := samples[0]
	tailSamples := samples[1:]
	metadata = withScrapeMetricMetadata(metadata)

	// Staleness markers are currently not supported by Cloud Monitoring.
	if value.IsStaleNaN(sample.V) {
		prometheusSamplesDiscarded.WithLabelValues("staleness-marker").Inc()
		return nil, 0, tailSamples, nil
	}

	entry, ok := b.series.get(sample, metadata)
	if !ok {
		prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
		return nil, 0, tailSamples, nil
	}

	// Get a shallow copy of the proto so we can overwrite the point field
	// and safely send it into the remote queues.
	ts := *entry.proto

	point := &monitoring_pb.Point{
		Interval: &monitoring_pb.TimeInterval{
			EndTime: getTimestamp(sample.T),
		},
		Value: &monitoring_pb.TypedValue{},
	}
	ts.Points = append(ts.Points, point)

	var resetTimestamp int64

	switch entry.metadata.Type {
	case textparse.MetricTypeCounter:
		var v float64
		resetTimestamp, v, ok = b.series.getResetAdjusted(sample.Ref, sample.T, sample.V)
		if !ok {
			return nil, 0, tailSamples, nil
		}
		point.Interval.StartTime = getTimestamp(resetTimestamp)
		point.Value.Value = &monitoring_pb.TypedValue_DoubleValue{v}

	case textparse.MetricTypeGauge, textparse.MetricTypeUnknown:
		point.Value.Value = &monitoring_pb.TypedValue_DoubleValue{sample.V}

	case textparse.MetricTypeSummary:
		switch entry.suffix {
		case metricSuffixSum, metricSuffixNone:
			// Quantiles and sum. The sum may actually go up and down if observations are negative.
			point.Value.Value = &monitoring_pb.TypedValue_DoubleValue{sample.V}
		case metricSuffixCount:
			var v float64
			resetTimestamp, v, ok = b.series.getResetAdjusted(sample.Ref, sample.T, sample.V)
			if !ok {
				return nil, 0, tailSamples, nil
			}
			point.Interval.StartTime = getTimestamp(resetTimestamp)
			point.Value.Value = &monitoring_pb.TypedValue_DoubleValue{v}
		default:
			return nil, 0, tailSamples, errors.Errorf("unexpected metric name suffix %q", entry.suffix)
		}

	case textparse.MetricTypeHistogram:
		// We pass in the original lset for matching since Prometheus's target label must
		// be the same as well.
		var v *distribution_pb.Distribution
		var err error
		v, resetTimestamp, tailSamples, err = b.buildDistribution(entry.metadata.Metric, entry.lset, samples, metadata)
		if v == nil || err != nil {
			return nil, 0, tailSamples, err
		}
		point.Interval.StartTime = getTimestamp(resetTimestamp)
		point.Value.Value = &monitoring_pb.TypedValue_DistributionValue{v}

	default:
		return nil, 0, samples[1:], errors.Errorf("unexpected metric type %s", entry.metadata.Type)
	}

	return &ts, entry.hash, tailSamples, nil
}

// getTimestamp converts a millisecond timestamp into a protobuf timestamp.
func getTimestamp(t int64) *timestamp_pb.Timestamp {
	return &timestamp_pb.Timestamp{
		Seconds: t / 1000,
		Nanos:   int32((t % 1000) * int64(time.Millisecond)),
	}
}

type distribution struct {
	bounds []float64
	values []int64
}

func (d *distribution) Len() int {
	return len(d.bounds)
}

func (d *distribution) Less(i, j int) bool {
	return d.bounds[i] < d.bounds[j]
}

func (d *distribution) Swap(i, j int) {
	d.bounds[i], d.bounds[j] = d.bounds[j], d.bounds[i]
	d.values[i], d.values[j] = d.values[j], d.values[i]
}

// buildDistribution consumes series from the beginning of the input slice that belong to a histogram
// with the given metric name and label set.
// It returns the reset timestamp along with the distrubution.
func (b *sampleBuilder) buildDistribution(
	baseName string,
	matchLset labels.Labels,
	samples []record.RefSample,
	metadata MetadataFunc,
) (*distribution_pb.Distribution, int64, []record.RefSample, error) {
	var (
		consumed       int
		count, sum     float64
		resetTimestamp int64
		lastTimestamp  int64
		dist           = distribution{bounds: make([]float64, 0, 16), values: make([]int64, 0, 16)}
		skip           = false
	)
	// We assume that all series belonging to the histogram are sequential. Consume series
	// until we hit a new metric.
Loop:
	for i, s := range samples {
		e, ok := b.series.get(s, metadata)
		if !ok {
			consumed++
			prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
			continue
		}
		name := e.lset.Get("__name__")
		// The series matches if it has the same base name, the remainder is a valid histogram suffix,
		// and the labels aside from the le and __name__ label match up.
		if !strings.HasPrefix(name, baseName) || !histogramLabelsEqual(e.lset, matchLset) {
			break
		}
		// In general, a scrape cannot contain the same (set of) series repeatedlty but for different timestamps.
		// It could still happen with bad clients though and we are doing it in tests for simplicity.
		// If we detect the same series as before but for a different timestamp, return the histogram up to this
		// series and leave the duplicate time series untouched on the input.
		if i > 0 && s.T != lastTimestamp {
			break
		}
		lastTimestamp = s.T

		rt, v, ok := b.series.getResetAdjusted(s.Ref, s.T, s.V)

		switch metricSuffix(name[len(baseName):]) {
		case metricSuffixSum:
			sum = v

		case metricSuffixCount:
			count = v
			// We take the count series as the authoritative source for the overall reset timestamp.
			resetTimestamp = rt

		case metricSuffixBucket:
			upper, err := strconv.ParseFloat(e.lset.Get("le"), 64)
			if err != nil {
				consumed++
				prometheusSamplesDiscarded.WithLabelValues("malformed-bucket-le-label").Inc()
				continue
			}
			dist.bounds = append(dist.bounds, upper)
			dist.values = append(dist.values, int64(v))

		default:
			break Loop
		}
		// If a series appeared for the first time, we won't get a valid reset timestamp yet.
		// This may happen if the histogram is entirely new or if new series appeared through bucket changes.
		// We skip the entire histogram sample in this case.
		if !ok {
			skip = true
		}
		consumed++
	}
	// If no sample was consumed at all, the input was wrong and we consume at least
	// one sample to not get stuck in a loop.
	if consumed == 0 {
		prometheusSamplesDiscarded.WithLabelValues("zero-histogram-samples-processed").Inc()
		return nil, 0, samples[1:], errors.New("no sample consumed for histogram")
	}
	// Don't emit a sample if we explicitly skip it or no reset timestamp was set because the
	// count series was missing.
	if skip || resetTimestamp == 0 {
		return nil, 0, samples[consumed:], nil
	}
	// We do not assume that the buckets in the sample batch are in order, so we sort them again here.
	// The code below relies on this to convert between Prometheus's and GCM's bucketing approaches.
	sort.Sort(&dist)
	// Reuse slices we already populated to build final bounds and values.
	var (
		bounds           = dist.bounds[:0]
		values           = dist.values[:0]
		mean, dev, lower float64
		prevVal          int64
	)
	if count > 0 {
		mean = sum / count
	}
	for i, upper := range dist.bounds {
		if math.IsInf(upper, 1) {
			upper = lower
		} else {
			bounds = append(bounds, upper)
		}

		val := dist.values[i] - prevVal
		// val should never be negative and it most likely indicates a bug or a data race in a scraped
		// metrics endpoint.
		// It's a possible caused of the zero-count issue below so we catch it here early.
		if val < 0 {
			prometheusSamplesDiscarded.WithLabelValues("negative-bucket-count").Add(float64(consumed))
			err := errors.Errorf("invalid bucket with negative count: count=%f, sum=%f, dev=%f, index=%d buckets=%v", count, sum, dev, i, dist)
			return nil, 0, samples[consumed:], err
		}
		x := (lower + upper) / 2
		dev += float64(val) * (x - mean) * (x - mean)

		lower = upper
		prevVal = dist.values[i]
		values = append(values, val)
	}
	// Catch distributions which are rejected by the CreateTimeSeries API and potentially
	// make the entire batch fail.
	if len(bounds) == 0 {
		prometheusSamplesDiscarded.WithLabelValues("zero-buckets-bounds").Add(float64(consumed))
		return nil, 0, samples[consumed:], nil
	}
	// Deviation and mean must be 0 if count is 0. We've got reports about samples with a negative
	// deviation and 0 count being sent.
	// Return an error to allow debugging this as it shouldn't happen under normal circumstances:
	// Deviation can only become negative if one histogram bucket has a lower value than the previous
	// one, which violates histogram's invariant.
	if count == 0 && (mean != 0 || dev != 0) {
		prometheusSamplesDiscarded.WithLabelValues("zero-count-violation").Add(float64(consumed))
		err := errors.Errorf("invalid histogram with 0 count: count=%f, sum=%f, dev=%f, buckets=%v", count, sum, dev, dist)
		return nil, 0, samples[consumed:], err
	}
	d := &distribution_pb.Distribution{
		Count:                 int64(count),
		Mean:                  mean,
		SumOfSquaredDeviation: dev,
		BucketOptions: &distribution_pb.Distribution_BucketOptions{
			Options: &distribution_pb.Distribution_BucketOptions_ExplicitBuckets{
				ExplicitBuckets: &distribution_pb.Distribution_BucketOptions_Explicit{
					Bounds: bounds,
				},
			},
		},
		BucketCounts: values,
	}
	return d, resetTimestamp, samples[consumed:], nil
}

// histogramLabelsEqual checks whether two label sets for a histogram series are equal aside from their
// le and __name__ labels.
func histogramLabelsEqual(a, b labels.Labels) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if a[i].Name == "le" || a[i].Name == "__name__" {
			i++
			continue
		}
		if b[j].Name == "le" || b[j].Name == "__name__" {
			j++
			continue
		}
		if a[i] != b[j] {
			return false
		}
		i++
		j++
	}
	// Consume trailing le and __name__ labels so the check below passes correctly.
	for i < len(a) {
		if a[i].Name == "le" || a[i].Name == "__name__" {
			i++
			continue
		}
		break
	}
	for j < len(b) {
		if b[j].Name == "le" || b[j].Name == "__name__" {
			j++
			continue
		}
		break
	}
	// If one label set still has labels left, they are not equal.
	return i == len(a) && j == len(b)
}
