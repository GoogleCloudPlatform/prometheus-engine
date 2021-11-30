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
	"strconv"
	"strings"
	"sync"
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

	dists map[uint64]*distribution
}

func newSampleBuilder(c *seriesCache) *sampleBuilder {
	return &sampleBuilder{
		series: c,
		dists:  make(map[uint64]*distribution, 128),
	}
}

func (b *sampleBuilder) close() {
	for _, d := range b.dists {
		putDistribution(d)
	}
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

// Metrics Prometheus writes at scrape time for which no metadata is exposed.
var internalMetricMetadata = map[string]MetricMetadata{
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

// withScrapeMetricMetadata wraps a MetadataFunc and additionally returns metadata
// about Prometheues's synthetic scrape-time metrics.
func withScrapeMetricMetadata(f MetadataFunc) MetadataFunc {
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
		md, ok := internalMetricMetadata[metric]
		if ok {
			return md, true
		}
		return f(metric)
	}
}

// next extracts the next sample from the input sample batch and returns
// the remainder of the input.
// Returns a nil time series for samples that couldn't be converted.
func (b *sampleBuilder) next(metadata MetadataFunc, samples []record.RefSample) ([]hashedSeries, []record.RefSample, error) {
	sample := samples[0]
	tailSamples := samples[1:]
	metadata = withScrapeMetricMetadata(metadata)

	// Staleness markers are currently not supported by Cloud Monitoring.
	if value.IsStaleNaN(sample.V) {
		prometheusSamplesDiscarded.WithLabelValues("staleness-marker").Inc()
		return nil, tailSamples, nil
	}

	entry, ok := b.series.get(sample, metadata)
	if !ok {
		prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
		return nil, tailSamples, nil
	}
	if entry.dropped {
		return nil, tailSamples, nil
	}

	result := make([]hashedSeries, 0, 2)

	// Shallow copy the cached series protos and populate them with a point. Only histograms
	// need a special case, for other Prometheus types we apply generic gauge/cumulative logic
	// based on the type determined in the series cache.
	// If both are set, we double-write the series as a gauge and a cumulative.
	if g := entry.protos.gauge; g.proto != nil {
		ts := *g.proto

		ts.Points = []*monitoring_pb.Point{{
			Interval: &monitoring_pb.TimeInterval{
				EndTime: getTimestamp(sample.T),
			},
			Value: &monitoring_pb.TypedValue{
				Value: &monitoring_pb.TypedValue_DoubleValue{sample.V},
			},
		}}
		result = append(result, hashedSeries{hash: g.hash, proto: &ts})
	}
	if c := entry.protos.cumulative; c.proto != nil {
		var (
			value          *monitoring_pb.TypedValue
			resetTimestamp int64
		)
		if entry.metadata.Type == textparse.MetricTypeHistogram {
			// Consume a set of series as a single distribution sample.

			// We pass in the original lset for matching since Prometheus's target label must
			// be the same as well.
			var v *distribution_pb.Distribution
			var err error
			v, resetTimestamp, tailSamples, err = b.buildDistribution(entry.metadata.Metric, entry.lset, samples, metadata)
			if err != nil {
				return nil, tailSamples, err
			}
			if v != nil {
				value = &monitoring_pb.TypedValue{
					Value: &monitoring_pb.TypedValue_DistributionValue{v},
				}
			}
		} else {
			// A regular counter series.
			var v float64
			resetTimestamp, v, ok = b.series.getResetAdjusted(sample.Ref, sample.T, sample.V)
			if ok {
				value = &monitoring_pb.TypedValue{
					Value: &monitoring_pb.TypedValue_DoubleValue{v},
				}
			}
		}
		// We may not have produced a value if:
		//
		//   1. It was the first sample of a cumulative and we only initialized  the reset timestamp with it.
		//   2. We could not observe all necessary series to build a full distribution sample.
		if value != nil {
			ts := *c.proto

			ts.Points = []*monitoring_pb.Point{{
				Interval: &monitoring_pb.TimeInterval{
					StartTime: getTimestamp(resetTimestamp),
					EndTime:   getTimestamp(sample.T),
				},
				Value: value,
			}}
			result = append(result, hashedSeries{hash: c.hash, proto: &ts})
		}
	}
	return result, tailSamples, nil
}

// getTimestamp converts a millisecond timestamp into a protobuf timestamp.
func getTimestamp(t int64) *timestamp_pb.Timestamp {
	return &timestamp_pb.Timestamp{
		Seconds: t / 1000,
		Nanos:   int32((t % 1000) * int64(time.Millisecond)),
	}
}

// A memory pool for distributions.
var distributionPool = sync.Pool{
	New: func() interface{} {
		return &distribution{}
	},
}

func getDistribution() *distribution {
	return distributionPool.Get().(*distribution)
}

func putDistribution(d *distribution) {
	d.reset()
	distributionPool.Put(d)
}

type distribution struct {
	bounds         []float64
	values         []int64
	sum            float64
	count          int64
	timestamp      int64
	resetTimestamp int64
	// If all three are true, we can be sure to have observed all series for the
	// distribution as buckets must be specified in ascending order.
	hasSum, hasCount, hasInfBucket bool
	// Whether to not emit a sample.
	skip bool
}

func (d *distribution) reset() {
	d.bounds = d.bounds[:0]
	d.values = d.values[:0]
	d.sum, d.count = 0, 0
	d.hasSum, d.hasCount, d.hasInfBucket = false, false, false
	d.timestamp, d.resetTimestamp = 0, 0
	d.skip = false
}

func (d *distribution) inputSampleCount() (c int) {
	if d.hasSum {
		c += 1
	}
	if d.hasCount {
		c += 1
	}
	return c + len(d.values)
}

func (d *distribution) complete() bool {
	// We can be sure to have accumulated all series if sum, count, and infinity bucket have been populated.
	return !d.skip && d.hasSum && d.hasCount && d.hasInfBucket
}

func (d *distribution) build(lset labels.Labels) (*distribution_pb.Distribution, error) {
	// Reuse slices we already populated to build final bounds and values.
	var (
		bounds               = d.bounds[:0]
		values               = d.values[:0]
		prevBound, dev, mean float64
		prevVal              int64
	)
	if d.count > 0 {
		mean = d.sum / float64(d.count)
	}
	for i, bound := range d.bounds {
		if math.IsInf(bound, 1) {
			bound = prevBound
		} else {
			bounds = append(bounds, bound)
		}

		val := d.values[i] - prevVal
		// val should never be negative and it most likely indicates a bug or a data race in a scraped
		// metrics endpoint.
		// It's a possible caused of the zero-count issue below so we catch it here early.
		if val < 0 {
			prometheusSamplesDiscarded.WithLabelValues("negative-bucket-count").Add(float64(d.inputSampleCount()))
			err := errors.Errorf("invalid bucket with negative count %s: count=%d, sum=%f, dev=%f, index=%d, bucketVal=%d, bucketPrevVal=%d",
				lset, d.count, d.sum, dev, i, d.values[i], prevVal)
			return nil, err
		}
		x := (prevBound + bound) / 2
		dev += float64(val) * (x - mean) * (x - mean)

		prevBound = bound
		prevVal = d.values[i]
		values = append(values, val)
	}
	// Catch distributions which are rejected by the CreateTimeSeries API and potentially
	// make the entire batch fail.
	if len(bounds) == 0 {
		prometheusSamplesDiscarded.WithLabelValues("zero-buckets-bounds").Add(float64(d.inputSampleCount()))
		return nil, nil
	}
	// Deviation and mean must be 0 if count is 0. We've got reports about samples with a negative
	// deviation and 0 count being sent.
	// Return an error to allow debugging this as it shouldn't happen under normal circumstances:
	// Deviation can only become negative if one histogram bucket has a lower value than the previous
	// one, which violates histogram's invariant.
	if d.count == 0 && (mean != 0 || dev != 0) {
		prometheusSamplesDiscarded.WithLabelValues("zero-count-violation").Add(float64(d.inputSampleCount()))
		err := errors.Errorf("invalid histogram with 0 count for %s: count=%d, sum=%f, dev=%f",
			lset, d.count, d.sum, dev)
		return nil, err
	}
	dp := &distribution_pb.Distribution{
		Count:                 d.count,
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
	return dp, nil
}

func isHistogramSeries(metric, name string) bool {
	if !strings.HasPrefix(name, metric) {
		return false
	}
	s := metricSuffix(name[len(metric):])
	return s == metricSuffixBucket || s == metricSuffixSum || s == metricSuffixCount
}

// buildDistribution consumes series from the input slice and populates the histogram cache with it.
// It returns when a series is consumed which completes a full distribution.
// Once all series for a single distribution have been observed, it returns it.
// It returns the reset timestamp along with the distrubution and the remaining samples.
func (b *sampleBuilder) buildDistribution(
	metric string,
	matchLset labels.Labels,
	samples []record.RefSample,
	metadata MetadataFunc,
) (*distribution_pb.Distribution, int64, []record.RefSample, error) {
	// The Prometheus/OpenMetrics exposition format does not require all histogram series for a single distribution
	// to be grouped together. But it does require that all series for a histogram metric in generall are grouped
	// together and that buckets for a single histogram are specified in order.
	// Thus, we build a cache and conclude a histogram complete once we've seen it's _sum series and its +Inf bucket
	// series. We return for the first histogram where this condition is fulfilled.
	consumed := 0
Loop:
	for _, s := range samples {
		e, ok := b.series.get(s, metadata)
		if !ok {
			consumed++
			prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
			continue
		}
		name := e.lset.Get(labels.MetricName)
		// Abort if the series is not for the intended histogram metric. All series for it must be grouped
		// together so we can rely on no further relevant series are in the batch.
		if !isHistogramSeries(metric, name) {
			break
		}
		consumed++

		// Create or update the cached distribution for the given histogram series
		dist, ok := b.dists[e.protos.cumulative.hash]
		if !ok {
			dist = getDistribution()
			dist.timestamp = s.T
			b.dists[e.protos.cumulative.hash] = dist
		}
		// If there are diverging timestamps within a single batch, the histogram is not valid.
		if s.T != dist.timestamp {
			dist.skip = true
			prometheusSamplesDiscarded.WithLabelValues("mismatching-histogram-timestamps").Inc()
			continue
		}

		rt, v, ok := b.series.getResetAdjusted(s.Ref, s.T, s.V)
		// If a series appeared for the first time, we won't get a valid reset timestamp yet.
		// This may happen if the histogram is entirely new or if new series appeared through bucket changes.
		// We skip the entire distribution sample in this case.
		if !ok {
			dist.skip = true
			continue
		}

		switch metricSuffix(name[len(metric):]) {
		case metricSuffixSum:
			dist.hasSum, dist.sum = true, v

		case metricSuffixCount:
			dist.hasCount, dist.count = true, int64(v)
			// We take the count series as the authoritative source for the overall reset timestamp.
			dist.resetTimestamp = rt

		case metricSuffixBucket:
			bound, err := strconv.ParseFloat(e.lset.Get(labels.BucketLabel), 64)
			if err != nil {
				prometheusSamplesDiscarded.WithLabelValues("malformed-bucket-le-label").Inc()
				continue
			}
			dist.hasInfBucket = math.IsInf(bound, 1)
			dist.bounds = append(dist.bounds, bound)
			dist.values = append(dist.values, int64(v))

		default:
			break Loop
		}

		if !dist.complete() {
			continue
		}
		dp, err := dist.build(e.lset)
		if err != nil {
			return nil, 0, samples[consumed:], err
		}
		return dp, dist.resetTimestamp, samples[consumed:], nil
	}
	if consumed == 0 {
		prometheusSamplesDiscarded.WithLabelValues("zero-histogram-samples-processed").Inc()
		return nil, 0, samples[1:], errors.New("no sample consumed for histogram")
	}
	// Batch ended without completing a further distribution
	return nil, 0, samples[consumed:], nil
}
