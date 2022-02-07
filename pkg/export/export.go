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
	"math"
	"strings"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	gax "github.com/googleapis/gax-go/v2"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb/record"
	"google.golang.org/api/option"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	samplesExported = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_exported_total",
		Help: "Number of samples exported at scrape time.",
	})
	samplesDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gcm_export_samples_dropped_total",
		Help: "Number of exported samples that were dropped because shard queues were full.",
	}, []string{"reason"})
	samplesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_sent_total",
		Help: "Number of exported samples sent to GCM.",
	})
	sendIterations = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_send_iterations_total",
		Help: "Number of processing iterations of the sample export send handler.",
	})
	shardProcess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_shard_process_total",
		Help: "Number of shard retrievals.",
	})
	shardProcessPending = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_shard_process_pending_total",
		Help: "Number of shard retrievals with an empty result.",
	})
	shardProcessSamplesTaken = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "gcm_export_shard_process_samples_taken",
		Help: "Number of samples taken when processing a shard.",
		// Limit buckets to 200, which is the real-world batch size for GCM.
		Buckets: []float64{1, 2, 5, 10, 20, 50, 100, 150, 200},
	})
	pendingRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gcm_export_pending_requests",
		Help: "Number of in-flight requests to GCM.",
	})
	projectsPerBatch = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "gcm_export_projects_per_batch",
		Help:    "Number of different projects in a batch that's being sent.",
		Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1024},
	})
	samplesPerRPCBatch = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: "gcm_export_samples_per_rpc_batch",
		Help: "Number of samples that ended up in a single RPC batch.",
		// Limit buckets to 200, which is the real-world batch size for GCM.
		Buckets: []float64{1, 2, 5, 10, 20, 50, 100, 150, 200},
	})
)

// Exporter converts Prometheus samples into Cloud Monitoring samples and exports them.
type Exporter struct {
	logger log.Logger
	opts   ExporterOpts

	metricClient *monitoring.MetricClient
	seriesCache  *seriesCache
	shards       []*shard

	// Channel for signaling that there may be more work items to
	// be processed.
	nextc chan struct{}

	// The external labels may be updated asynchronously by configuration changes
	// and must be locked with mtx.
	mtx            sync.Mutex
	externalLabels labels.Labels
	// A set of metrics for which we defaulted the metadata to untyped and have
	// issued a warning about that.
	warnedUntypedMetrics map[string]struct{}
}

const (
	// Number of shards by which series are bucketed.
	shardCount = 1024
	// Buffer size for each individual shard.
	shardBufferSize = 2048

	// Maximum number of samples to pack into a batch sent to GCM.
	BatchSizeMax = 200
	// Time after an accumulating batch is flushed to GCM. This avoids data being
	// held indefinititely if not enough new data flows in to fill up the batch.
	batchDelayMax = 5 * time.Second

	// Prefix for GCM metric.
	MetricTypePrefix = "prometheus.googleapis.com"
)

// Supported gRPC compression formats.
const (
	CompressionNone = "none"
	CompressionGZIP = "gzip"
)

// ExporterOpts holds options for an exporter.
type ExporterOpts struct {
	// Whether to disable exporting of metrics.
	Disable bool
	// GCM API endpoint to send metric data to.
	Endpoint string
	// Compression format to use for gRPC requests.
	Compression string
	// Credentials file for authentication with the GCM API.
	CredentialsFile string
	// Disable authentication (for debugging purposes).
	DisableAuth bool
	// A user agent string added as a suffix to the regular user agent.
	UserAgent string

	// Default monitored resource fields set on exported data.
	ProjectID string
	Location  string
	Cluster   string

	// A list of metric matchers. Only Prometheus time series satisfying at
	// least one of the matchers are exported.
	// This option matches the semantics of the Prometheus federation match[]
	// parameter.
	Matchers Matchers

	// Maximum batch size to use when sending data to the GCM API. The default
	// maximum will be used if set to 0.
	BatchSize uint
	// Prefix under which metrics are written to GCM.
	MetricTypePrefix string

	// A lease on a time range for which the exporter send sample data.
	// It is checked for on each batch provided to the Export method.
	// If unset, data is always sent.
	Lease Lease
}

// NopExporter returns an inactive exporter.
func NopExporter() *Exporter {
	return &Exporter{
		opts: ExporterOpts{Disable: true},
	}
}

// Lease determines a currently owned time range.
type Lease interface {
	// Range informs whether the caller currently holds the lease and for what time range.
	// The range is inclusive.
	Range() (start, end time.Time, ok bool)
	// Run background processing until context is cancelled.
	Run(context.Context)
	// OnLeaderChange sets a callback that is invoked when the lease leader changes.
	// Must be called before Run.
	OnLeaderChange(func())
}

// alwaysLease is a lease that is always held.
type alwaysLease struct{}

func (alwaysLease) Range() (time.Time, time.Time, bool) {
	return time.UnixMilli(math.MinInt64), time.UnixMilli(math.MaxInt64), true
}

func (alwaysLease) Run(ctx context.Context) {
	<-ctx.Done()
}

func (alwaysLease) OnLeaderChange(f func()) {
	// We never lose the lease as it's always owned.
}

func newMetricClient(ctx context.Context, opts ExporterOpts) (*monitoring.MetricClient, error) {
	// Identity User Agent for all gRPC requests.
	ua := strings.TrimSpace(fmt.Sprintf("%s/%s %s", ClientName, Version, opts.UserAgent))

	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor)),
		option.WithUserAgent(ua),
	}
	if opts.Endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(opts.Endpoint))
	}
	if opts.DisableAuth {
		clientOpts = append(clientOpts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithInsecure()),
		)
	}
	if opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(opts.CredentialsFile))
	}
	client, err := monitoring.NewMetricClient(ctx, clientOpts...)
	if err != nil {
		return nil, err
	}
	if opts.Compression == CompressionGZIP {
		client.CallOptions.CreateTimeSeries = append(client.CallOptions.CreateTimeSeries,
			gax.WithGRPCOptions(grpc.UseCompressor(gzip.Name)))
	}
	return client, nil
}

// New returns a new Cloud Monitoring Exporter.
func New(logger log.Logger, reg prometheus.Registerer, opts ExporterOpts) (*Exporter, error) {
	grpc_prometheus.EnableClientHandlingTimeHistogram()

	if logger == nil {
		logger = log.NewNopLogger()
	}
	if reg != nil {
		reg.MustRegister(
			prometheusSamplesDiscarded,
			samplesExported,
			samplesDropped,
			samplesSent,
			sendIterations,
			shardProcess,
			shardProcessPending,
			shardProcessSamplesTaken,
			pendingRequests,
			projectsPerBatch,
			samplesPerRPCBatch,
		)
	}

	if opts.BatchSize == 0 {
		opts.BatchSize = BatchSizeMax
	}
	if opts.BatchSize > BatchSizeMax {
		return nil, errors.Errorf("Maximum supported batch size is %d, got %d", BatchSizeMax, opts.BatchSize)
	}
	if opts.MetricTypePrefix == "" {
		opts.MetricTypePrefix = MetricTypePrefix
	}
	if opts.Lease == nil {
		opts.Lease = alwaysLease{}
	}

	metricClient, err := newMetricClient(context.Background(), opts)
	if err != nil {
		return nil, errors.Wrap(err, "create metric client")
	}
	e := &Exporter{
		logger:               logger,
		opts:                 opts,
		metricClient:         metricClient,
		nextc:                make(chan struct{}, 1),
		shards:               make([]*shard, shardCount),
		warnedUntypedMetrics: map[string]struct{}{},
	}
	e.seriesCache = newSeriesCache(logger, reg, opts.MetricTypePrefix, opts.Matchers)

	// Whenever the lease is lost, clear the series cache so we don't start off of out-of-range
	// reset timestamps when we gain the lease again.
	opts.Lease.OnLeaderChange(e.seriesCache.clear)

	for i := range e.shards {
		e.shards[i] = newShard(shardBufferSize)
	}

	return e, nil
}

// The target label keys used for the Prometheus monitored resource.
const (
	KeyProjectID = "project_id"
	KeyLocation  = "location"
	KeyCluster   = "cluster"
	KeyNamespace = "namespace"
	KeyJob       = "job"
	KeyInstance  = "instance"
)

// ApplyConfig updates the exporter state to the given configuration.
// Must be called at least once before Export() can be used.
func (e *Exporter) ApplyConfig(cfg *config.Config) (err error) {
	// If project_id, location and cluster were set through explicit flags or auto-discovery,
	// set them in the external labels. Currently we don't expect a use case where one would want
	// to override auto-discovery values via external labels.
	// All resource labels may still later be overriden by metric labels per the precedence semantics
	// Prometheus upstream has.
	builder := labels.NewBuilder(cfg.GlobalConfig.ExternalLabels)

	if e.opts.ProjectID != "" {
		builder.Set(KeyProjectID, e.opts.ProjectID)
	}
	if e.opts.Location != "" {
		builder.Set(KeyLocation, e.opts.Location)
	}
	if e.opts.Cluster != "" {
		builder.Set(KeyCluster, e.opts.Cluster)
	}
	lset := builder.Labels()

	// At this point we expect location and project ID to be set. They are effectively only a default
	// however as they may be overriden by metric labels.
	// In production scenarios, "location" should most likely never be overriden as it means crossing
	// failure domains. Instead, each location should run a replica of the evaluator with the same rules.
	if lset.Get(KeyProjectID) == "" {
		return errors.Errorf("no label %q set via external labels or flag", KeyProjectID)
	}
	if lset.Get(KeyLocation) == "" {
		return errors.Errorf("no label %q set via external labels or flag", KeyLocation)
	}
	if labels.Equal(e.externalLabels, lset) {
		return nil
	}
	// New external labels possibly invalidate the cached series conversions.
	e.mtx.Lock()
	e.externalLabels = lset
	e.seriesCache.forceRefresh()
	e.mtx.Unlock()

	return nil
}

// SetLabelsByIDFunc injects a function that can be used to retrieve a label set
// based on a series ID we got through exported sample records.
// Must be called before any call to Export is made.
func (e *Exporter) SetLabelsByIDFunc(f func(uint64) labels.Labels) {
	// Prevent panics in case a default disabled exporter was instantiated (see Global()).
	if e.opts.Disable {
		return
	}
	if e.seriesCache.getLabelsByRef != nil {
		panic("SetLabelsByIDFunc must only be called once")
	}
	e.seriesCache.getLabelsByRef = f
}

// Export enqueues the samples to be written to Cloud Monitoring.
func (e *Exporter) Export(metadata MetadataFunc, batch []record.RefSample) {
	if e.opts.Disable {
		return
	}

	metadata = e.wrapMetadata(metadata)

	e.mtx.Lock()
	externalLabels := e.externalLabels
	start, end, ok := e.opts.Lease.Range()
	e.mtx.Unlock()

	if !ok {
		prometheusSamplesDiscarded.WithLabelValues("no-ha-range").Inc()
		return
	}

	builder := newSampleBuilder(e.seriesCache)
	defer builder.close()

	for len(batch) > 0 {
		var (
			samples []hashedSeries
			err     error
		)
		samples, batch, err = builder.next(metadata, externalLabels, batch)
		if err != nil {
			level.Debug(e.logger).Log("msg", "building sample failed", "err", err)
			continue
		}
		for _, s := range samples {
			// Only enqueue samples for within our HA range.
			if sampleInRange(s.proto, start, end) {
				e.enqueue(s.hash, s.proto)
			} else {
				samplesDropped.WithLabelValues("not-in-ha-range").Inc()
			}
		}
	}
	// Signal that new data is available.
	e.triggerNext()
}

func sampleInRange(sample *monitoring_pb.TimeSeries, start, end time.Time) bool {
	// A sample has exactly one point in the time series. The start timestamp may be unset for gauges.
	if s := sample.Points[0].Interval.StartTime; s != nil && s.AsTime().Before(start) {
		return false
	}
	if sample.Points[0].Interval.EndTime.AsTime().After(end) {
		return false
	}
	return true
}

func (e *Exporter) enqueue(hash uint64, sample *monitoring_pb.TimeSeries) {
	idx := hash % uint64(len(e.shards))
	e.shards[idx].enqueue(hash, sample)
}

func (e *Exporter) triggerNext() {
	select {
	case e.nextc <- struct{}{}:
	default:
	}
}

// ClientName and Version are used to identify to User Agent. TODO(maxamin): automate versioning.
const (
	ClientName = "prometheus-engine-export"
	Version    = "0.2.2"
)

// Run sends exported samples to Google Cloud Monitoring. Must be called at most once.
// ApplyConfig must be called once prior to calling Run.
func (e *Exporter) Run(ctx context.Context) error {
	defer e.metricClient.Close()
	go e.seriesCache.run(ctx)
	go e.opts.Lease.Run(ctx)

	timer := time.NewTimer(batchDelayMax)
	stopTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	defer stopTimer()

	// Start a loop that gathers samples and sends them to GCM.
	//
	// Samples must not arrive at the GCM API out of order. To ensure that, there
	// must be at most one in-flight request per series. Tracking every series individually
	// would also require separate queue per series. This would come with a lot of overhead
	// and implementation complexity.
	// Instead, we shard the series space and maintain one queue per shard. For every shard
	// we ensure that there is at most one in-flight request.
	//
	// One solution would be to have a separate send loop per shard that reads from
	// the queue, accumulates a batch, and sends it to the GCM API. The drawback is that one
	// has to get the number of shards right. Too low, and samples per shard cannot be sent
	// fast enough. Too high, and batches do not fill up, potentially sending new requests
	// for every sample.
	// As a result, fine-tuning at startup but also runtime is necessary to respond to changing
	// load patterns and latency of the API.
	//
	// We largely avoid this issue by filling up batches from multiple shards. Under high load,
	// a batch contains samples from fewer shards, under low load from more.
	// The per-shard overhead is minimal and thus a high number can be picked, which allows us
	// to cover a large range of potential throughput and latency combinations without requiring
	// user configuration or, even worse, runtime changes to the shard number.

	// The batch and the shards that have contributed data to it so far.
	var (
		batch         = newBatch(e.logger, e.opts.BatchSize)
		pendingShards = make([]*shard, 0, shardCount)
	)

	// Send the currently accumulated batch to GCM asynchronously.
	send := func() {
		go batch.send(ctx, pendingShards, e.metricClient.CreateTimeSeries)

		// Reset state for new batch.
		stopTimer()
		timer.Reset(batchDelayMax)

		pendingShards = make([]*shard, 0, shardCount)
		batch = newBatch(e.logger, e.opts.BatchSize)
	}

	// Starting index when iterating over shards. This ensures we don't always start at 0 so that
	// some shards may never be sent in a busy collector.
	shardOffset := 0

	for {
		select {
		// NOTE(freinartz): we will terminate once context is cancelled and not flush remaining
		// buffered data. In-flight requests will be aborted as well.
		// This is fine once we persist data submitted via Export() but for now there may be some
		// data loss on shutdown.
		case <-ctx.Done():
			return nil
		// This is activated for each new sample that arrives
		case <-e.nextc:
			sendIterations.Inc()

			// Drain shards to fill up the batch.
			//
			// If the shard count is high given the overall throughput, a lot of shards may
			// be packed into the same batch. A slow request will then block all those shards
			// from further parallel sends.
			// If this becomes a problem (especially when we grow maximum batch size), consider
			// adding a heuristic to send partial batches in favor of limiting the number of
			// shards they span.
			i := 0
			for ; i < len(e.shards); i++ {
				shardOffset = (shardOffset + 1) % len(e.shards)
				shard := e.shards[shardOffset]

				if took := shard.fill(batch); took > 0 {
					pendingShards = append(pendingShards, shard)
				}
				if batch.full() {
					send()
				}
			}
			// If we didn't make a full pass over all shards, there may be more work.
			if i < len(e.shards) {
				e.triggerNext()
			}

		case <-timer.C:
			// Flush batch that has been pending for too long.
			if !batch.empty() {
				send()
			} else {
				timer.Reset(batchDelayMax)
			}
		}
	}
}

// CtxKey is a dedicated type for keys of context-embedded values propagated
// with the scrape context.
type ctxKey int

// Valid CtxKey values.
const (
	ctxKeyMetadata ctxKey = iota + 1
)

// WithMetadataFunc stores mf in the context.
func WithMetadataFunc(ctx context.Context, mf MetadataFunc) context.Context {
	return context.WithValue(ctx, ctxKeyMetadata, mf)
}

// MetadataFuncFromContext extracts a MetataFunc from ctx.
func MetadataFuncFromContext(ctx context.Context) (MetadataFunc, bool) {
	mf, ok := ctx.Value(ctxKeyMetadata).(MetadataFunc)
	return mf, ok
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

func (e *Exporter) wrapMetadata(f MetadataFunc) MetadataFunc {
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
	// Ensure that we always cover synthetic scrape metrics and in doubt fallback
	// to untyped metrics. The wrapping order is important!
	f = withScrapeMetricMetadata(f)
	f = e.withUntypedDefaultMetadata(f)

	return f
}

// gaugeMetadata is a MetadataFunc that always returns the gauge type.
// Help and Unit are left empty.
func gaugeMetadata(metric string) (MetricMetadata, bool) {
	return MetricMetadata{
		Metric: metric,
		Type:   textparse.MetricTypeGauge,
	}, true
}

// untypedMetadata is a MetadataFunc that always returns the untyped/unknown type.
// Help and Unit are left empty.
func untypedMetadata(metric string) (MetricMetadata, bool) {
	return MetricMetadata{
		Metric: metric,
		Type:   textparse.MetricTypeUnknown,
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
	return func(metric string) (MetricMetadata, bool) {
		md, ok := internalMetricMetadata[metric]
		if ok {
			return md, true
		}
		return f(metric)
	}
}

// withUntypedDefaultMetadata returns a MetadataFunc that returns the untyped
// type, if no metadata is found through f.
// It logs a warning once per metric name where a default to untyped happened
// as this is generally undesirable.
//
// For Prometheus this primarily handles cases where metric relabeling is used to
// create new metric names on the fly, for which no metadata is known.
// This allows ingesting this data in a best-effort manner.
func (e *Exporter) withUntypedDefaultMetadata(f MetadataFunc) MetadataFunc {
	return func(metric string) (MetricMetadata, bool) {
		md, ok := f(metric)
		if ok {
			return md, true
		}
		// The metric name may contain suffixes (_sum, _bucket, _count), which need to be stripped
		// to find the matching metadata. Before we can assume that not metadata exist, we've
		// to verify that the base name is not found either.
		// Our transformation logic applies the same lookup sequence. Without this step
		// we'd incorrectly return the untyped metadata for all those sub-series.
		if baseName, _, ok := splitMetricSuffix(metric); ok {
			if _, ok := f(baseName); ok {
				// There is metadata for the underlying metric, return false and let the
				// conversion logic do its thing.
				return MetricMetadata{}, false
			}
		}
		// We only log a message the first time for each metric. We check this against a global cache
		// as the total number of unique observed names is generally negligible.
		e.mtx.Lock()
		defer e.mtx.Unlock()

		if _, ok := e.warnedUntypedMetrics[metric]; !ok {
			level.Warn(e.logger).Log("msg", "no metadata found, defaulting to untyped metric", "metric_name", metric)
			e.warnedUntypedMetrics[metric] = struct{}{}
		}
		return untypedMetadata(metric)
	}
}

// batch accumulates a batch of samples to be sent to GCM. Once the batch is full
// it must be sent and cannot be used anymore after that.
type batch struct {
	logger  log.Logger
	maxSize uint

	m       map[string][]*monitoring_pb.TimeSeries
	oneFull bool
	total   int
}

func newBatch(logger log.Logger, maxSize uint) *batch {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &batch{
		logger:  logger,
		maxSize: maxSize,
		m:       make(map[string][]*monitoring_pb.TimeSeries, 1),
	}
}

// add a new sample to the batch. Must only be called after full() returned false.
func (b *batch) add(s *monitoring_pb.TimeSeries) {
	pid := s.Resource.Labels[KeyProjectID]

	l, ok := b.m[pid]
	if !ok {
		l = make([]*monitoring_pb.TimeSeries, 0, b.maxSize)
	}
	l = append(l, s)
	b.m[pid] = l

	if len(l) == cap(l) {
		b.oneFull = true
	}
	b.total++
}

// full returns whether the batch is full. Being full means that add() must not be called again
// and it guarantees that at most one request per project with at most maxSize samples is made.
func (b *batch) full() bool {
	// We determine that a batch is full if at least one project's batch is full.
	//
	// TODO(freinartz): We could add further conditions here like the total number projects or samples so we don't
	// accumulate too many requests that block the shards that contributed to the batch.
	// However, this may in turn result in too many small requests in flight.
	// Another option is to limit the number of shards contributing to a single batch.
	return b.oneFull
}

// empty returns true if the batch contains no samples.
func (b *batch) empty() bool {
	return b.total == 0
}

// send the accumulated samples to their respective projects. It returns once all
// requests have completed and notifies the pending shards.
func (b batch) send(
	ctx context.Context,
	pendingShards []*shard,
	sendOne func(context.Context, *monitoring_pb.CreateTimeSeriesRequest, ...gax.CallOption) error,
) {
	// Set timeout so slow requests in the batch do not block overall progress indefinitely.
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	projectsPerBatch.Observe(float64(len(b.m)))
	var wg sync.WaitGroup

	for pid, l := range b.m {
		wg.Add(1)

		go func(pid string, l []*monitoring_pb.TimeSeries) {
			defer wg.Done()

			pendingRequests.Inc()
			defer pendingRequests.Dec()

			samplesPerRPCBatch.Observe(float64(len(l)))

			// We do not retry any requests due to the risk of producing a backlog
			// that cannot be worked down, especially if large amounts of clients try to do so.
			err := sendOne(sendCtx, &monitoring_pb.CreateTimeSeriesRequest{
				Name:       fmt.Sprintf("projects/%s", pid),
				TimeSeries: l,
			})
			if err != nil {
				level.Error(b.logger).Log("msg", "send batch", "size", len(l), "err", err)
			}
			samplesSent.Add(float64(len(l)))
		}(pid, l)
	}
	wg.Wait()

	for _, s := range pendingShards {
		s.notifyDone()
	}
}

// Matchers holds a list of metric selectors that can be set as a flag.
type Matchers []labels.Selector

func (m *Matchers) String() string {
	return fmt.Sprintf("%v", []labels.Selector(*m))
}

func (m *Matchers) Set(s string) error {
	if s == "" {
		return nil
	}
	ms, err := parser.ParseMetricSelector(s)
	if err != nil {
		return errors.Wrapf(err, "invalid metric matcher %q", s)
	}
	*m = append(*m, ms)
	return nil
}

func (m *Matchers) IsCumulative() bool {
	return true
}

func (m *Matchers) Matches(lset labels.Labels) bool {
	if len(*m) == 0 {
		return true
	}
	for _, sel := range *m {
		if sel.Matches(lset) {
			return true
		}
	}
	return false
}
