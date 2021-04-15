// Copyright 2020 Google Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package export

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"google.golang.org/api/option"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
)

var (
	samplesExported = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_samples_exported_total",
		Help: "Number of samples exported at scrape time.",
	})
	samplesDropped = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_samples_dropped_total",
		Help: "Number of exported samples that were dropped because shard queues were full.",
	})
	samplesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_samples_sent_total",
		Help: "Number of exported samples sent to GCM.",
	})
	sendIterations = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_send_iterations_total",
		Help: "Number of processing iterations of the sample export send handler.",
	})
	shardProcess = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_shard_process_total",
		Help: "Number of shard retrievals.",
	})
	shardProcessPending = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_collector_shard_process_pending_total",
		Help: "Number of shard retrievals with an empty result.",
	})
	shardProcessSamplesTaken = prometheus.NewSummary(prometheus.SummaryOpts{
		Name:       "gcm_collector_shard_process_samples_taken",
		Help:       "Number of samples taken when processing a shard.",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	})
	pendingRequests = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "gcm_collector_pending_requests",
		Help: "Number of in-flight requests to GCM.",
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
)

// ExporterOpts holds options for an exporter.
type ExporterOpts struct {
	Disable bool
	// Google Cloud project ID to which data is sent.
	ProjectID string
	// The location identifier used for the monitored resource of exported data.
	Location string
	// The cluster identifier used for the monitored resource of exported data.
	Cluster string

	// GCM API endpoint to send metric data to.
	Endpoint string
	// Credentials file for authentication with the GCM API.
	CredentialsFile string
	// Disable authentication (for debugging purposes).
	DisableAuth bool

	// Maximum batch size to use when sending data to the GCM API. The default
	// maximum will be used if set to 0.
	BatchSize uint
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

	a.Flag("gcm.disable", "Disable exporting to GCM.").
		Default("false").BoolVar(&opts.Disable)

	a.Flag("gcm.project_id", "Google Cloud project ID to which data is sent.").
		Default(opts.ProjectID).StringVar(&opts.ProjectID)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("gcm.label.location", fmt.Sprintf("The location set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", KeyLocation)).
		Default(opts.Location).StringVar(&opts.Location)

	a.Flag("gcm.label.cluster", fmt.Sprintf("The cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", KeyCluster)).
		Default(opts.Cluster).StringVar(&opts.Cluster)

	a.Flag("gcm.endpoint", "GCM API endpoint to send metric data to.").
		Default("monitoring.googleapis.com:443").StringVar(&opts.Endpoint)

	a.Flag("gcm.credentials-file", "Credentials file for authentication with the GCM API.").
		StringVar(&opts.CredentialsFile)

	a.Flag("gcm.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default("false").BoolVar(&opts.DisableAuth)

	a.Flag("gcm.debug.batch-size", "Maximum number of points to send in one batch to the GCM API.").
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
			samplesExported,
			samplesDropped,
			samplesSent,
			sendIterations,
			shardProcess,
			shardProcessPending,
			shardProcessSamplesTaken,
			pendingRequests,
		)
	}

	if opts.ProjectID == "" {
		return nil, errors.New("GCP project ID missing")
	}
	// Location is generally also required but we allow it to also be set
	// through Prometheus's external labels, which we receive via ApplyConfig.

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
	e.seriesCache = newSeriesCache(logger, reg, e.getExternalLabels)
	e.builder = &sampleBuilder{series: e.seriesCache}

	for i := range e.shards {
		e.shards[i] = newShard(shardBufferSize)
	}

	return e, nil
}

// The target label keys used for the Prometheus monitored resource.
const (
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

	// If location and cluster were set through explicit flags or auto-discovery, set
	// them in the external labels.
	builder := labels.NewBuilder(cfg.GlobalConfig.ExternalLabels)

	if e.opts.Location != "" {
		builder.Set(KeyLocation, e.opts.Location)
	}
	if e.opts.Cluster != "" {
		builder.Set(KeyCluster, e.opts.Cluster)
	}
	lset := builder.Labels()

	// At this point we expect a location to be set. It is very unlikely a user wants to set
	// this dynamically via target label. It would imply collection across failure domains, which
	// is an anti-pattern.
	// The cluster however is allowed to be empty or could feasibly be set through target labels.
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
		panic("Global GCM exporter used before initialization.")
	}
	return globalExporter
}

// SetLabelsByIDFunc injects a function that can be used to retrieve a label set
// based on a series ID we got through exported sample records.
// Must be called before any call to Export is made.
func (e *Exporter) SetLabelsByIDFunc(f func(uint64) labels.Labels) {
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
func (e *Exporter) Export(target Target, samples []record.RefSample) {
	if e.opts.Disable {
		return
	}
	var (
		sample *monitoring_pb.TimeSeries
		hash   uint64
		err    error
	)
	for len(samples) > 0 {
		sample, hash, samples, err = e.builder.next(target, samples)
		if err != nil {
			level.Debug(e.logger).Log("msg", "building sample failed", "err", err)
		}
		if sample != nil {
			// TODO(freinartz): decouple sending from ingestion by writing to a
			// dedicated write-ahead-log here from which the send queues consume.
			e.enqueue(hash, sample)
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
	ClientName = "gpe-collector"
	Version    = ""
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
			option.WithUserAgent(ClientName+"/"+Version),
		)
	}
	if e.opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(e.opts.CredentialsFile))
	}
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
		batch         = make([]*monitoring_pb.TimeSeries, 0, e.opts.BatchSize)
		pendingShards = make([]*shard, 0, shardCount)
	)

	// Send the currently accumulated batch to GCM asynchronously.
	send := func() {
		pendingRequests.Inc()

		go func(batch []*monitoring_pb.TimeSeries, pendingShards []*shard) {
			if err := e.send(ctx, metricClient, batch); err != nil {
				level.Error(e.logger).Log("msg", "send batch", "err", err)
			}
			samplesSent.Add(float64(len(batch)))

			for _, s := range pendingShards {
				s.notifyBatchDone()
			}
			pendingRequests.Dec()
		}(batch, pendingShards)

		// Reset state for new batch.
		stopTimer()
		timer.Reset(batchDelayMax)

		pendingShards = make([]*shard, 0, shardCount)
		batch = make([]*monitoring_pb.TimeSeries, 0, e.opts.BatchSize)
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

				if took := shard.fill(&batch); took > 0 {
					pendingShards = append(pendingShards, shard)
				}
				if len(batch) == cap(batch) {
					send()
				}
			}
			// If we didn't make a full pass over all shards, there may be more work.
			if i < len(e.shards) {
				e.triggerNext()
			}

		case <-timer.C:
			// Flush batch that has been pending for too long.
			if len(batch) > 0 {
				send()
			} else {
				timer.Reset(batchDelayMax)
			}
		}
	}
}

func (e *Exporter) send(ctx context.Context, client *monitoring.MetricClient, batch []*monitoring_pb.TimeSeries) error {
	// TODO(freinartz): Handle retries if the error type allows.
	return client.CreateTimeSeries(ctx, &monitoring_pb.CreateTimeSeriesRequest{
		Name:       fmt.Sprintf("projects/%s", e.opts.ProjectID),
		TimeSeries: batch,
	})
}

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
	shardProcess.Inc()

	s.mtx.Lock()
	defer s.mtx.Unlock()

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

// NewStorage returns a new Prometheus storage that's exporting data via an Exporter.
func NewStorage(logger log.Logger, reg prometheus.Registerer, opts ExporterOpts) (*Storage, error) {
	exporter, err := New(logger, reg, opts)
	if err != nil {
		return nil, err
	}
	// Call ApplyConfig once with an empty config so it can initialize the exporter state properly.
	exporter.ApplyConfig(&config.Config{})

	s := &Storage{
		exporter: exporter,
		labels:   map[uint64]labels.Labels{},
	}
	exporter.SetLabelsByIDFunc(s.labelsByID)

	return s, nil
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
	// they are of type gauge. Thus we pass in a target that always returns default metric
	// metadata.
	// In the future we may want to populate the help text with information on the rule
	// that produced the metric.
	a.storage.exporter.Export(gaugeTarget{}, a.samples)

	// After export is complete, we can clear the labels again.
	a.storage.clearLabels(a.samples)

	return nil
}

type gaugeTarget struct{}

func (t gaugeTarget) Metadata(metric string) (scrape.MetricMetadata, bool) {
	return scrape.MetricMetadata{
		Metric: metric,
		Type:   textparse.MetricTypeGauge,
	}, true
}
