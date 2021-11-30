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
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb/record"

	metric_pb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// seriesCache holds a mapping from series reference to label set.
// It can garbage collect obsolete entries based on the most recent WAL checkpoint.
// Implements seriesGetter.
type seriesCache struct {
	logger log.Logger
	now    func() time.Time
	pool   *pool

	// Guards access to the entries and intervals maps and the lastRefresh
	// field of individual cache entries.
	mtx sync.Mutex
	// Map from series reference to various cached information about it.
	entries map[uint64]*seriesCacheEntry

	// Function to retrieve a label set for a series reference number.
	// Returns nil if the reference is no longer valid.
	getLabelsByRef func(uint64) labels.Labels

	// Function to retrieve external labels for the instance.
	getExternalLabels func() labels.Labels

	// A list of metric selectors. Exported Prometheus are discarded if they
	// don't match at least one of the matchers.
	// If the matchers are empty, all series pass.
	matchers Matchers

	// Prefix under which metrics are written to GCM.
	metricTypePrefix string
}

type seriesCacheEntry struct {
	// The uniquely identifying set of labels for the series.
	lset labels.Labels

	// Metadata for the metric of the series.
	metadata MetricMetadata
	// A pre-populated time protobuf to be sent to the GCM API. It can
	// be shallow-copied and populated with point values to avoid excessive
	// allocations for each datapoint exported for the series.
	protos cachedProtos
	// The well-known Prometheus metric name suffix if any.
	suffix metricSuffix
	// Timestamp after which to refresh the cached state.
	nextRefresh int64
	// Unix timestamp at which the we last used the entry.
	lastUsed int64
	// Whether the series is dropped from exporting.
	dropped bool

	// Tracked counter reset state for conversion to GCM cumulatives.
	hasReset       bool
	resetValue     float64
	lastValue      float64
	resetTimestamp int64
}

type hashedSeries struct {
	hash  uint64
	proto *monitoring_pb.TimeSeries
}

type cachedProtos struct {
	gauge, cumulative hashedSeries
}

func (cp *cachedProtos) empty() bool {
	return cp.gauge.proto == nil && cp.cumulative.proto == nil
}

const (
	refreshInterval = 10 * time.Minute
	refreshJitter   = 10 * time.Minute
)

// valid returns true if the Prometheus series can be converted to a GCM series.
func (e *seriesCacheEntry) valid() bool {
	return e.lset != nil && (e.dropped || !e.protos.empty())
}

// shouldRefresh returns true if the cached state should be refreshed.
func (e *seriesCacheEntry) shouldRefresh() bool {
	// Matchers cannot be changed at runtime and are applied to the local time series labels
	// without external labels. Thus the dropped status can never change at runtime and thus
	// no refresh is required.
	return !e.dropped && time.Now().Unix() > e.nextRefresh
}

// setNextRefresh determines a timestamp for the next refresh.
func (e *seriesCacheEntry) setNextRefresh() {
	// Randomly offset the timestamp around the targeted average so a bulk of simultaniously
	// created series are not invalidated all at once, causing potential CPU and allocation
	// spikes.
	jitter := time.Duration((rand.Float64() - 0.5) * float64(refreshJitter))
	e.nextRefresh = time.Now().Add(refreshInterval).Add(jitter).Unix()
}

func newSeriesCache(
	logger log.Logger,
	reg prometheus.Registerer,
	metricTypePrefix string,
	getExternalLabels func() labels.Labels,
	matchers Matchers,
) *seriesCache {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	if getExternalLabels == nil {
		getExternalLabels = func() labels.Labels { return nil }
	}
	return &seriesCache{
		logger:            logger,
		now:               time.Now,
		pool:              newPool(reg),
		entries:           map[uint64]*seriesCacheEntry{},
		getExternalLabels: getExternalLabels,
		matchers:          matchers,
		metricTypePrefix:  metricTypePrefix,
	}
}

func (c *seriesCache) run(ctx context.Context) {
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if err := c.garbageCollect(10 * time.Minute); err != nil {
				level.Error(c.logger).Log("msg", "garbage collection failed", "err", err)
			}
		}
	}
}

// invalidateAll invalidates all cache entries.
func (c *seriesCache) invalidateAll() {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	// Set next refresh to the zero timestamp to trigger a refresh.
	for _, e := range c.entries {
		e.nextRefresh = 0
	}
}

// garbageCollect drops obsolete cache entries that have not been updated for
// the given delay duration.
func (c *seriesCache) garbageCollect(delay time.Duration) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	start := c.now()

	// Drop all series that haven't been used in 10 minutes.
	//
	// Alternatively, we could call getLabelsByRef on each series and discard it if the
	// result is nil. While more reliable in only evicting entries that will never come back
	// it may mean stale entries sit around for up to 3 hours.
	// Since we can always re-populate cache entries, this is not worth it as it may blow
	// up our memory usage in high-churn environments.
	deleteBefore := start.Add(-delay).Unix()

	for ref, entry := range c.entries {
		if entry.lastUsed >= deleteBefore {
			continue
		}
		c.pool.release(entry.protos.gauge.proto)
		c.pool.release(entry.protos.cumulative.proto)
		delete(c.entries, ref)
	}
	level.Info(c.logger).Log("msg", "garbage collection completed", "took", time.Since(start))

	return nil
}

// get a cache entry for the given series reference. The passed timestamp indicates when data was
// last seen for the entry.
// If the series cannot be converted the returned boolean is false.
func (c *seriesCache) get(s record.RefSample, metadata MetadataFunc) (*seriesCacheEntry, bool) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	e, ok := c.entries[s.Ref]
	if !ok {
		e = &seriesCacheEntry{}
		c.entries[s.Ref] = e
	}
	if e.shouldRefresh() {
		if err := c.populate(s.Ref, e, metadata); err != nil {
			level.Debug(c.logger).Log("msg", "populating series failed", "ref", s.Ref, "err", err)
		}
		e.setNextRefresh()
	}
	// Store millisecond sample timestamp in seconds.
	e.lastUsed = s.T / 1000
	return e, e.valid()
}

// getResetAdjusted takes a sample for a referenced series and returns
// its reset timestamp and adjusted value.
// If the last return argument is false, the sample should be dropped.
func (c *seriesCache) getResetAdjusted(ref uint64, t int64, v float64) (int64, float64, bool) {
	c.mtx.Lock()
	e, ok := c.entries[ref]
	c.mtx.Unlock()
	if !ok {
		return 0, 0, false
	}
	hasReset := e.hasReset
	e.hasReset = true
	if !hasReset {
		e.resetTimestamp = t
		e.resetValue = v
		// If we just initialized the reset timestamp, this sample should be skipped.
		// We don't know the window over which the current cumulative value was built up over.
		// The next sample for will be considered from this point onwards.
		return 0, 0, false
	} else if t <= e.resetTimestamp {
		// Otherwise if the current sample's time was already processed, drop sample.
		// Keeping the sample is not desirable because it results in:
		// - (at best) performing excessive API write calls with redundant data
		// - sending API bad requests in the form of zero-ranged sample intervals
		// - attempting to update a previous point, resulting in an error response.
		//
		// Note: this will only omit duplicates of the initial "reset" sample.
		// Omitting duplicates of all incoming samples would require
		// more sophisticated state management.
		return 0, 0, false
	}
	if v < e.lastValue {
		// If the series was reset, set the reset timestamp to be one millisecond
		// before the timestamp of the current sample.
		// We don't know the true reset time but this ensures the range is non-zero
		// while unlikely to conflict with any previous sample.
		e.resetValue = 0
		e.resetTimestamp = t - 1
	}
	e.lastValue = v

	return e.resetTimestamp, v - e.resetValue, true
}

// getMetricType creates a GCM metric type from the Prometheus metric name and a type suffix.
// Optionally, a secondary type suffix may be provided for series for which a Prometheus type
// may be written as different GCM series.
// The general rule is that if the primary suffix is ambigious about whether the specific series
// is to be treated as a counter or gauge at query time, the secondarySuffix is set to "counter"
// for the counter variant, and left empty for the gauge variant.
func (c *seriesCache) getMetricType(name string, suffix, secondarySuffix gcmMetricSuffix) string {
	if secondarySuffix == gcmMetricSuffixNone {
		return fmt.Sprintf("%s/%s/%s", c.metricTypePrefix, name, suffix)
	}
	return fmt.Sprintf("%s/%s/%s:%s", c.metricTypePrefix, name, suffix, secondarySuffix)
}

// Metric name suffixes used by various Prometheus metric types.
type metricSuffix string

const (
	metricSuffixNone   metricSuffix = ""
	metricSuffixTotal  metricSuffix = "_total"
	metricSuffixBucket metricSuffix = "_bucket"
	metricSuffixSum    metricSuffix = "_sum"
	metricSuffixCount  metricSuffix = "_count"
)

// Suffixes appended to GCM metric types. They are equivalent to the respective
// Prometheus types but we redfine them here to ensure they don't unexpectedly change
// by updating a Prometheus library.
type gcmMetricSuffix string

const (
	gcmMetricSuffixNone      gcmMetricSuffix = ""
	gcmMetricSuffixUnknown   gcmMetricSuffix = "unknown"
	gcmMetricSuffixGauge     gcmMetricSuffix = "gauge"
	gcmMetricSuffixCounter   gcmMetricSuffix = "counter"
	gcmMetricSuffixHistogram gcmMetricSuffix = "histogram"
	gcmMetricSuffixSummary   gcmMetricSuffix = "summary"
)

// Maximum number of labels allowed on GCM series.
const maxLabelCount = 100

// populate cached state for the given entry.
func (c *seriesCache) populate(ref uint64, entry *seriesCacheEntry, getMetadata MetadataFunc) error {
	if entry.lset == nil {
		entry.lset = c.getLabelsByRef(ref)
		if entry.lset == nil {
			return errors.New("series reference invalid")
		}
		entry.dropped = !c.matchers.Matches(entry.lset)
	}
	if entry.dropped {
		return nil
	}
	// Break the series into resource and metric labels.
	resource, metricLabels, err := c.extractResource(entry.lset)
	if err != nil {
		return errors.Wrapf(err, "extracting resource for series %s failed", entry.lset)
	}

	// Remove the __name__ label as it becomes the metric type in the GCM time series.
	for i, l := range metricLabels {
		if l.Name == "__name__" {
			metricLabels = append(metricLabels[:i], metricLabels[i+1:]...)
			break
		}
	}
	// Drop series with too many labels.
	// TODO: remove once field limit is lifted in the GCM API.
	if len(metricLabels) > maxLabelCount {
		return errors.Errorf("metric labels %s exceed the limit of %d", metricLabels, maxLabelCount)
	}

	var (
		metricName     = entry.lset.Get("__name__")
		baseMetricName = metricName
		suffix         metricSuffix
	)
	metadata, ok := getMetadata(metricName)
	if !ok {
		// The full name didn't turn anything up. Check again in case it's a summary
		// or histogram without the metric name suffix. If the underlying target
		// returned the OpenMetrics format, counter metadata is also stored with the
		// _total suffix stripped.
		var ok bool
		if baseMetricName, suffix, ok = splitMetricSuffix(metricName); ok {
			metadata, ok = getMetadata(baseMetricName)
		}
		if !ok {
			return errors.Errorf("no metadata found for metric name %q", metricName)
		}
	}
	// Handle label modifications for histograms early so we don't build the label map twice.
	// We have to remove the 'le' label which defines the bucket boundary.
	if metadata.Type == textparse.MetricTypeHistogram {
		for i, l := range metricLabels {
			if l.Name == "le" {
				metricLabels = append(metricLabels[:i], metricLabels[i+1:]...)
				break
			}
		}
	}

	newSeries := func(mtype string, kind metric_pb.MetricDescriptor_MetricKind, vtype metric_pb.MetricDescriptor_ValueType) hashedSeries {
		s := &monitoring_pb.TimeSeries{
			Resource:   resource,
			Metric:     &metric_pb.Metric{Type: mtype, Labels: metricLabels.Map()},
			MetricKind: kind,
			ValueType:  vtype,
		}
		return hashedSeries{hash: hashSeries(s), proto: s}
	}
	var protos cachedProtos

	switch metadata.Type {
	case textparse.MetricTypeCounter:
		protos.cumulative = newSeries(
			c.getMetricType(metricName, gcmMetricSuffixCounter, gcmMetricSuffixNone),
			metric_pb.MetricDescriptor_CUMULATIVE,
			metric_pb.MetricDescriptor_DOUBLE)

	case textparse.MetricTypeGauge:
		protos.gauge = newSeries(
			c.getMetricType(metricName, gcmMetricSuffixGauge, gcmMetricSuffixNone),
			metric_pb.MetricDescriptor_GAUGE,
			metric_pb.MetricDescriptor_DOUBLE)

	case textparse.MetricTypeUnknown:
		protos.gauge = newSeries(
			c.getMetricType(metricName, gcmMetricSuffixUnknown, gcmMetricSuffixNone),
			metric_pb.MetricDescriptor_GAUGE,
			metric_pb.MetricDescriptor_DOUBLE)
		protos.cumulative = newSeries(
			c.getMetricType(metricName, gcmMetricSuffixUnknown, gcmMetricSuffixCounter),
			metric_pb.MetricDescriptor_CUMULATIVE,
			metric_pb.MetricDescriptor_DOUBLE)

	case textparse.MetricTypeSummary:
		switch suffix {
		case metricSuffixSum:
			protos.cumulative = newSeries(
				c.getMetricType(metricName, gcmMetricSuffixSummary, gcmMetricSuffixCounter),
				metric_pb.MetricDescriptor_CUMULATIVE,
				metric_pb.MetricDescriptor_DOUBLE)

		case metricSuffixCount:
			protos.cumulative = newSeries(
				c.getMetricType(metricName, gcmMetricSuffixSummary, gcmMetricSuffixNone),
				metric_pb.MetricDescriptor_CUMULATIVE,
				metric_pb.MetricDescriptor_DOUBLE)

		case metricSuffixNone: // Actual quantiles.
			protos.gauge = newSeries(
				c.getMetricType(metricName, gcmMetricSuffixSummary, gcmMetricSuffixNone),
				metric_pb.MetricDescriptor_GAUGE,
				metric_pb.MetricDescriptor_DOUBLE)

		default:
			return errors.Errorf("unexpected metric name suffix %q for metric %q", suffix, metricName)
		}

	case textparse.MetricTypeHistogram:
		protos.cumulative = newSeries(
			c.getMetricType(baseMetricName, gcmMetricSuffixHistogram, gcmMetricSuffixNone),
			metric_pb.MetricDescriptor_CUMULATIVE,
			metric_pb.MetricDescriptor_DISTRIBUTION)

	default:
		return errors.Errorf("unexpected metric type %s for metric %q", metadata.Type, metricName)
	}

	c.pool.release(entry.protos.gauge.proto)
	c.pool.release(entry.protos.cumulative.proto)
	c.pool.intern(protos.gauge.proto)
	c.pool.intern(protos.cumulative.proto)

	entry.protos = protos
	entry.metadata = metadata
	entry.suffix = suffix

	return nil
}

// extractResource returns the monitored resource, the entry labels, and whether the operation succeeded.
// The returned entry labels are a subset of `lset` without the labels that were used as resource labels.
func (c *seriesCache) extractResource(lset labels.Labels) (*monitoredres_pb.MonitoredResource, labels.Labels, error) {
	// Prometheus allows to configure external labels, which are attached when exporting data out of
	// the instance to disambiguate data across instances. For us they generally include 'project_id',
	// 'location' and 'cluster'.
	// Per Prometheus semantics external labels are merged into lset, while lset takes precedence on
	// label name collisions.
	//
	// This can be problematic as it violates hierarchical precedence. Especially 'project_id'
	// or 'location' being overwritten from a metric label could likely fill in an invalid value.
	// A sensible solution could be to adopt Prometheus collision resolution for target and metric
	// labels, in which colliding metric label keys are prefixed with 'exported_'.
	//
	// However, the semantics are also right and important for recording rules, where one would generally
	// want to retain original resource fields of the metrics but would want to default to a 'project_id' and
	// 'location' for rules which aggregated away the original fields.
	// For example a recording of 'sum(up)' would still need a fallback 'project_id' and 'location' to
	// be stored in.
	//
	// Thus we stick with the upstream semantics and consider how to address unintended consequences if
	// and when they come up.
	builder := labels.NewBuilder(lset)

	for _, l := range c.getExternalLabels() {
		if !lset.Has(l.Name) {
			builder.Set(l.Name, l.Value)
		}
	}
	lset = builder.Labels()

	// Ensure project_id and location are set but leave validating of the values to the API.
	if lset.Get(KeyProjectID) == "" {
		return nil, nil, errors.Errorf("missing required resource field %q", KeyProjectID)
	}
	if lset.Get(KeyLocation) == "" {
		return nil, nil, errors.Errorf("missing required resource field %q", KeyLocation)
	}

	// Transfer resource fields from label set onto the resource. If they are not set,
	// the respective field is set to an empty string. This explicitly is a valid value
	// in Cloud Monitoring and not the same as being unset.
	mres := &monitoredres_pb.MonitoredResource{
		Type: "prometheus_target",
		Labels: map[string]string{
			KeyProjectID: lset.Get(KeyProjectID),
			KeyLocation:  lset.Get(KeyLocation),
			KeyCluster:   lset.Get(KeyCluster),
			KeyNamespace: lset.Get(KeyNamespace),
			KeyJob:       lset.Get(KeyJob),
			KeyInstance:  lset.Get(KeyInstance),
		},
	}
	builder.Del(KeyProjectID)
	builder.Del(KeyLocation)
	builder.Del(KeyCluster)
	builder.Del(KeyNamespace)
	builder.Del(KeyJob)
	builder.Del(KeyInstance)

	return mres, builder.Labels(), nil
}

func splitMetricSuffix(name string) (prefix string, suffix metricSuffix, ok bool) {
	if strings.HasSuffix(name, string(metricSuffixTotal)) {
		return name[:len(name)-len(metricSuffixTotal)], metricSuffixTotal, true
	}
	if strings.HasSuffix(name, string(metricSuffixBucket)) {
		return name[:len(name)-len(metricSuffixBucket)], metricSuffixBucket, true
	}
	if strings.HasSuffix(name, string(metricSuffixCount)) {
		return name[:len(name)-len(metricSuffixCount)], metricSuffixCount, true
	}
	if strings.HasSuffix(name, string(metricSuffixSum)) {
		return name[:len(name)-len(metricSuffixSum)], metricSuffixSum, true
	}
	return name, metricSuffixNone, false
}

func hashSeries(s *monitoring_pb.TimeSeries) uint64 {
	h := fnv.New64a()

	h.Write([]byte(s.Resource.Type))
	hashLabels(h, s.Resource.Labels)
	h.Write([]byte(s.Metric.Type))
	hashLabels(h, s.Metric.Labels)
	binary.Write(h, binary.LittleEndian, s.MetricKind)
	binary.Write(h, binary.LittleEndian, s.ValueType)

	return h.Sum64()
}

func hashLabels(h hash.Hash, lset map[string]string) {
	sep := []byte{'\xff'}
	// Map iteration is randomized. We thus convert the labels to sorted slices
	// with labels.FromMap before hashing.
	for _, l := range labels.FromMap(lset) {
		h.Write(sep)
		h.Write([]byte(l.Name))
		h.Write(sep)
		h.Write([]byte(l.Value))
	}
	h.Write(sep)
}
