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
	"os"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	gax "github.com/googleapis/gax-go/v2"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb/record"
	"google.golang.org/api/option"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	samplesExported = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_exported_total",
		Help: "Number of samples exported at scrape time.",
	})
	samplesDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_dropped_total",
		Help: "Number of exported samples that were dropped because shard queues were full.",
	})
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

	seriesCache *seriesCache
	builder     *sampleBuilder
	shards      []*shard

	// Channel for signaling that there may be more work items to
	// be processed.
	nextc chan struct{}

	mtx            sync.Mutex
	externalLabels labels.Labels
}

const (
	// Number of shards by which series are bucketed.
	shardCount = 1024
	// Buffer size for each individual shard.
	shardBufferSize = 2048

	// Maximum number of samples to pack into a batch sent to GCM.
	batchSizeMax = 200
	// Time after an accumulating batch is flushed to GCM. This avoids data being
	// held indefinititely if not enough new data flows in to fill up the batch.
	batchDelayMax = 5 * time.Second

	// Prefix for GCM metric.
	metricTypePrefix = "prometheus.googleapis.com"
)

// ExporterOpts holds options for an exporter.
type ExporterOpts struct {
	// Whether to disable exporting of metrics.
	Disable bool
	// GCM API endpoint to send metric data to.
	Endpoint string
	// Credentials file for authentication with the GCM API.
	CredentialsFile string
	// Disable authentication (for debugging purposes).
	DisableAuth bool

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
}

// NewFlagOptions returns new exporter options that are populated through flags
// registered in the given application.
func NewFlagOptions(a *kingpin.Application) *ExporterOpts {
	var opts ExporterOpts

	// Default target fields if we can detect them in GCP.
	if metadata.OnGCE() {
		opts.ProjectID, _ = metadata.ProjectID()
		// These attributes are set for GKE nodes.
		opts.Location, _ = metadata.InstanceAttributeValue("cluster-location")
		opts.Cluster, _ = metadata.InstanceAttributeValue("cluster-name")
	}

	a.Flag("export.disable", "Disable exporting to GCM.").
		Default("false").BoolVar(&opts.Disable)

	a.Flag("export.endpoint", "GCM API endpoint to send metric data to.").
		Default("monitoring.googleapis.com:443").StringVar(&opts.Endpoint)

	a.Flag("export.credentials-file", "Credentials file for authentication with the GCM API.").
		StringVar(&opts.CredentialsFile)

	a.Flag("export.label.project-id", fmt.Sprintf("Default project ID set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", KeyProjectID)).
		Default(opts.ProjectID).StringVar(&opts.ProjectID)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("export.label.location", fmt.Sprintf("The default location set for all exported data. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", KeyLocation)).
		Default(opts.Location).StringVar(&opts.Location)

	a.Flag("export.label.cluster", fmt.Sprintf("The default cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", KeyCluster)).
		Default(opts.Cluster).StringVar(&opts.Cluster)

	a.Flag("export.match", `A Prometheus time series matcher. Can be repeated. Every time series must match at least one of the matchers to be exported. This flag can be used equivalently to the match[] parameter of the Prometheus federation endpoint to selectively export data. (Example: --export.match='{job="prometheus"}' --export.match='{__name__=~"job:.*"})`).
		SetValue(&opts.Matchers)

	a.Flag("export.debug.metric-prefix", "Google Cloud Monitoring metric prefix to use.").
		Default(metricTypePrefix).StringVar(&opts.MetricTypePrefix)

	a.Flag("export.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default("false").BoolVar(&opts.DisableAuth)

	a.Flag("export.debug.batch-size", "Maximum number of points to send in one batch to the GCM API.").
		Default(strconv.Itoa(batchSizeMax)).UintVar(&opts.BatchSize)

	return &opts
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
		opts.BatchSize = batchSizeMax
	}
	if opts.BatchSize > batchSizeMax {
		return nil, errors.Errorf("Maximum supported batch size is %d, got %d", batchSizeMax, opts.BatchSize)
	}

	e := &Exporter{
		logger: logger,
		opts:   opts,
		nextc:  make(chan struct{}, 1),
		shards: make([]*shard, shardCount),
	}
	e.seriesCache = newSeriesCache(logger, reg, opts.MetricTypePrefix, e.getExternalLabels, opts.Matchers)
	e.builder = &sampleBuilder{series: e.seriesCache}

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
func (e *Exporter) ApplyConfig(cfg *config.Config) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

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
	// New external labels invalidate the cached series conversions.
	if !labels.Equal(e.externalLabels, lset) {
		e.externalLabels = lset
		e.seriesCache.invalidateAll()
	}
	return nil
}

// Generally, global state is not a good approach and actively discouraged throughout
// the Prometheus code bases. However, this is the most practical way to inject the export
// path into lower layers of Prometheus without touching an excessive amount of functions
// in our fork to propagate it.
var globalExporter *Exporter

// InitGlobal initializes the global instance of the GCM exporter.
func InitGlobal(logger log.Logger, reg prometheus.Registerer, opts ExporterOpts) (err error) {
	globalExporter, err = New(logger, reg, opts)
	return err
}

// Global returns the global instance of the GCM exporter.
func Global() *Exporter {
	if globalExporter == nil {
		// This should usually be a panic but we set an inactive default exporter in this case
		// to not break existing tests in Prometheus.
		fmt.Fprintln(os.Stderr, "No global GCM exporter was set, setting default inactive exporter.")
		return &Exporter{
			opts: ExporterOpts{Disable: true},
		}
	}
	return globalExporter
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

func (e *Exporter) getExternalLabels() labels.Labels {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	return e.externalLabels
}

// Export enqueues the samples to be written to Cloud Monitoring.
func (e *Exporter) Export(metadata MetadataFunc, batch []record.RefSample) {
	if e.opts.Disable {
		return
	}
	for len(batch) > 0 {
		var (
			samples []hashedSeries
			err     error
		)
		samples, batch, err = e.builder.next(metadata, batch)
		if err != nil {
			level.Debug(e.logger).Log("msg", "building sample failed", "err", err)
			continue
		}
		for _, s := range samples {
			e.enqueue(s.hash, s.proto)
		}
	}
	// Signal that new data is available.
	e.triggerNext()
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
	ClientName = "gmp-collector"
	Version    = "v2.28.1-gmp.0"
)

// Run sends exported samples to Google Cloud Monitoring.
func (e *Exporter) Run(ctx context.Context) error {
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor)),
	}
	if e.opts.Endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(e.opts.Endpoint))
	}
	if e.opts.DisableAuth {
		clientOpts = append(clientOpts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithInsecure()),
		)
	}
	if e.opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(e.opts.CredentialsFile))
	}

	// Identity User Agent for all gRPC requests.
	clientOpts = append(clientOpts, option.WithUserAgent(ClientName+"/"+Version))

	metricClient, err := monitoring.NewMetricClient(ctx, clientOpts...)
	if err != nil {
		return err
	}
	defer metricClient.Close()

	go e.seriesCache.run(ctx)

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
		go batch.send(ctx, pendingShards, metricClient.CreateTimeSeries)

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
