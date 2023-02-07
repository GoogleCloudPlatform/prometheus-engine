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
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"

	timestamp_pb "github.com/golang/protobuf/ptypes/timestamp"
	distribution_pb "google.golang.org/genproto/googleapis/api/distribution"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

const (
	projectIDLabel    = "project_id"
	traceIDLabel      = "trace_id"
	spanIDLabel       = "span_id"
	spanContextFormat = "projects/%s/traces/%s/spans/%s"
)

var (
	prometheusSamplesDiscarded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcm_prometheus_samples_discarded_total",
			Help: "Samples that were discarded during data model conversion.",
		},
		[]string{"reason"},
	)
	prometheusExemplarsDiscarded = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gcm_prometheus_exemplars_discarded_total",
			Help: "Exemplars that were discarded during data model conversion.",
		},
		[]string{"reason"},
	)
)

// discardExemplarIncIfExists increments the counter prometheusExemplarsDiscarded
// if an exemplar exists for the given storage.SeriesRef.
func discardExemplarIncIfExists(series storage.SeriesRef, exemplars map[storage.SeriesRef]record.RefExemplar, reason string) {
	if _, ok := exemplars[storage.SeriesRef(series)]; ok {
		prometheusExemplarsDiscarded.WithLabelValues(reason).Inc()
	}
}

type sampleBuilder struct {
	series *seriesCache
	dists  map[uint64]*distribution
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

// next extracts the next sample from the input sample batch and returns
// the remainder of the input. It also attaches valid exemplars if applicable.
// Returns a nil time series for samples that couldn't be converted.
func (b *sampleBuilder) next(metadata MetadataFunc, externalLabels labels.Labels, samples []record.RefSample, exemplars map[storage.SeriesRef]record.RefExemplar) ([]hashedSeries, []record.RefSample, error) {
	sample := samples[0]
	tailSamples := samples[1:]

	// Staleness markers are currently not supported by Cloud Monitoring.
	if value.IsStaleNaN(sample.V) {
		prometheusSamplesDiscarded.WithLabelValues("staleness-marker").Inc()
		discardExemplarIncIfExists(storage.SeriesRef(sample.Ref), exemplars, "staleness-marker")
		return nil, tailSamples, nil
	}

	entry, ok := b.series.get(sample, externalLabels, metadata)
	if !ok {
		prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
		discardExemplarIncIfExists(storage.SeriesRef(sample.Ref), exemplars, "no-cache-series-found")
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
			v, resetTimestamp, tailSamples, err = b.buildDistribution(
				entry.metadata.Metric,
				entry.lset,
				samples,
				exemplars,
				externalLabels,
				metadata,
			)
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
			resetTimestamp, v, ok = b.series.getResetAdjusted(storage.SeriesRef(sample.Ref), sample.T, sample.V)
			if ok {
				value = &monitoring_pb.TypedValue{
					Value: &monitoring_pb.TypedValue_DoubleValue{v},
				}
				discardExemplarIncIfExists(storage.SeriesRef(sample.Ref), exemplars, "counters-unsupported")
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

// If adding fields to this object, be sure to reset them to their
// zero value in the reset method below.
type distribution struct {
	bounds         []float64
	values         []int64
	sum            float64
	count          float64
	timestamp      int64
	resetTimestamp int64
	exemplars      []record.RefExemplar
	// If all three are true, we can be sure to have observed all series for the
	// distribution as buckets must be specified in ascending order.
	hasSum, hasCount, hasInfBucket bool
	// Whether to not emit a sample.
	skip bool
}

// TODO: create a unit test that makes sure distribution objects
// are reset properly, especially when adding new fields.
func (d *distribution) reset() {
	d.bounds = d.bounds[:0]
	d.values = d.values[:0]
	d.sum, d.count = 0, 0
	d.hasSum, d.hasCount, d.hasInfBucket = false, false, false
	d.timestamp, d.resetTimestamp = 0, 0
	d.skip = false
	d.exemplars = d.exemplars[:0]
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

func (d *distribution) build(lset labels.Labels) (*distribution_pb.Distribution, error) {
	// The exposition format in general requires buckets to be in-order but we observed
	// some cases in the wild where this was not the case.
	// Ensure sorting here to gracefully handle those cases sometimes. This cannot handle
	// all cases. Specifically, if buckets are out-of-order distribution.complete() may
	// return true before all buckets have been read. Then we will send a distribution
	// with only a subset of buckets.
	sort.Sort(d)

	// Populate new values and bounds slices for the final proto as d will be returned to
	// the memory pool while the proto will be enqueued for sending.
	var (
		bounds               = make([]float64, 0, len(d.bounds))
		values               = make([]int64, 0, len(d.values))
		prevBound, dev, mean float64
		prevVal              int64
	)
	// Some client libraries have race conditions causing a mismatch in counts across buckets and count
	// series. The most common case seems to be count mismatching while the buckets are consistent.
	// We handle this here by always picking the inf bucket value.
	// This help ingesting samples that would otherwise be dropped.
	d.count = float64(d.values[len(d.bounds)-1])

	// In principle, the count and sum series could theoretically be NaN.
	// For the sum series this has been observed in the wild.
	// As NaN is not a permitted mean value in Cloud Monitoring, we leave it at the default 0 in this case.
	// For the count we overrode it with the inf bucket value anyway and thus don't need special handling.
	if !math.IsNaN(d.sum) && d.count > 0 {
		mean = d.sum / d.count
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
			err := errors.Errorf("invalid bucket with negative count %s: count=%f, sum=%f, dev=%f, index=%d, bucketVal=%d, bucketPrevVal=%d",
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
		err := errors.Errorf("invalid histogram with 0 count for %s: count=%f, sum=%f, dev=%f",
			lset, d.count, d.sum, dev)
		return nil, err
	}
	dp := &distribution_pb.Distribution{
		Count:                 int64(d.count),
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
		Exemplars:    buildExemplars(d.exemplars),
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
	exemplars map[storage.SeriesRef]record.RefExemplar,
	externalLabels labels.Labels,
	metadata MetadataFunc,
) (*distribution_pb.Distribution, int64, []record.RefSample, error) {
	// The Prometheus/OpenMetrics exposition format does not require all histogram series for a single distribution
	// to be grouped together. But it does require that all series for a histogram metric in general are grouped
	// together and that buckets for a single histogram are specified in order.
	// Thus, we build a cache and conclude a histogram complete once we've seen it's _sum series and its +Inf bucket
	// series. We return for the first histogram where this condition is fulfilled.
	consumed := 0
Loop:
	for _, s := range samples {
		e, ok := b.series.get(s, externalLabels, metadata)
		if !ok {
			consumed++
			prometheusSamplesDiscarded.WithLabelValues("no-cache-series-found").Inc()
			discardExemplarIncIfExists(storage.SeriesRef(s.Ref), exemplars, "no-cache-series-found")
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
			discardExemplarIncIfExists(storage.SeriesRef(s.Ref), exemplars, "mismatching-histogram-timestamps")
			continue
		}

		rt, v, ok := b.series.getResetAdjusted(storage.SeriesRef(s.Ref), s.T, s.V)
		// If a series appeared for the first time, we won't get a valid reset timestamp yet.
		// This may happen if the histogram is entirely new or if new series appeared through bucket changes.
		// We skip the entire distribution sample in this case.
		if !ok {
			dist.skip = true
			continue
		}

		// All series can in principle have a NaN value (staleness NaNs already filtered).
		// We permit this for sum and count as we handle it explicitly when building the distribution.
		// For buckets there's not sensible way to handle it however and we discard those bucket samples.
		switch metricSuffix(name[len(metric):]) {
		case metricSuffixSum:
			dist.hasSum, dist.sum = true, v

		case metricSuffixCount:
			dist.hasCount, dist.count = true, v
			// We take the count series as the authoritative source for the overall reset timestamp.
			dist.resetTimestamp = rt

		case metricSuffixBucket:
			bound, err := strconv.ParseFloat(e.lset.Get(labels.BucketLabel), 64)
			if err != nil {
				prometheusSamplesDiscarded.WithLabelValues("malformed-bucket-le-label").Inc()
				discardExemplarIncIfExists(storage.SeriesRef(s.Ref), exemplars, "malformed-bucket-le-label")
				continue
			}
			if math.IsNaN(v) {
				prometheusSamplesDiscarded.WithLabelValues("NaN-bucket-value").Inc()
				discardExemplarIncIfExists(storage.SeriesRef(s.Ref), exemplars, "NaN-bucket-value")
				continue
			}
			// Handle cases where +Inf bucket is out-of-order by not overwriting on the last-consumed bucket.
			if !dist.hasInfBucket {
				dist.hasInfBucket = math.IsInf(bound, 1)
			}
			dist.bounds = append(dist.bounds, bound)
			dist.values = append(dist.values, int64(v))
			if exemplar, ok := exemplars[storage.SeriesRef(s.Ref)]; ok {
				dist.exemplars = append(dist.exemplars, exemplar)
			}

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
		discardExemplarIncIfExists(storage.SeriesRef(samples[0].Ref), exemplars, "zero-histogram-samples-processed")
		return nil, 0, samples[1:], errors.New("no sample consumed for histogram")
	}
	// Batch ended without completing a further distribution
	return nil, 0, samples[consumed:], nil
}

func buildExemplars(exemplars []record.RefExemplar) []*distribution_pb.Distribution_Exemplar {
	// The exemplars field of a distribution value field must be in increasing order of value
	// (https://cloud.google.com/monitoring/api/ref_v3/rpc/google.api#distribution) -- let's sort them.
	sort.Slice(exemplars, func(i, j int) bool {
		return exemplars[i].V < exemplars[j].V
	})
	var result []*distribution_pb.Distribution_Exemplar
	for _, pex := range exemplars {
		attachments := buildExemplarAttachments(pex.Labels)
		result = append(result, &distribution_pb.Distribution_Exemplar{
			Value:       pex.V,
			Timestamp:   getTimestamp(pex.T),
			Attachments: attachments,
		})
	}
	return result
}

// buildExemplarAttachments transforms the prometheus LabelSet into a GCM exemplar attachment.
// If the following three fields are present in the LabelSet, then we will build a SpanContext:
//  1. project_id
//  2. span_id
//  3. trace_id
//
// The rest of the LabelSet will go into the DroppedLabels attachment. If one of the above
// fields is missing, we will put the entire LabelSet into a DroppedLabels attachment.
// This is to maintain comptability with CloudTrace.
// Note that the project_id needs to be the project_id where the span was written.
// This may not necessarily be the same project_id where the metric was written.
func buildExemplarAttachments(lset labels.Labels) []*anypb.Any {
	var projectID, spanID, traceID string
	var attachments []*anypb.Any
	droppedLabels := make(map[string]string)
	for _, label := range lset {
		if label.Name == projectIDLabel {
			projectID = label.Value
		} else if label.Name == spanIDLabel {
			spanID = label.Value
		} else if label.Name == traceIDLabel {
			traceID = label.Value
		} else {
			droppedLabels[label.Name] = label.Value
		}
	}
	if projectID != "" && spanID != "" && traceID != "" {
		spanCtx, err := anypb.New(&monitoring_pb.SpanContext{
			SpanName: fmt.Sprintf(spanContextFormat, projectID, traceID, spanID),
		})
		if err != nil {
			prometheusExemplarsDiscarded.WithLabelValues("error-creating-span-context").Inc()
		} else {
			attachments = append(attachments, spanCtx)
		}
	} else {
		if projectID != "" {
			droppedLabels[projectIDLabel] = projectID
		}
		if spanID != "" {
			droppedLabels[spanIDLabel] = spanID
		}
		if traceID != "" {
			droppedLabels[traceIDLabel] = traceID
		}
	}
	if len(droppedLabels) > 0 {
		droppedLabels, err := anypb.New(&monitoring_pb.DroppedLabels{
			Label: droppedLabels,
		})
		if err != nil {
			prometheusExemplarsDiscarded.WithLabelValues("error-creating-dropped-labels").Inc()
		} else {
			attachments = append(attachments, droppedLabels)
		}
	}
	return attachments
}
