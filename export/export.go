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
	"github.com/prometheus/prometheus/scrape"
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

// Exporter converts Prometheus samples into Cloud Monitoring samples and exporst them.
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
	// Keeping it at just 5 seconds generally prevents two scrapes of the same target
	// making it into the same batch, which would trigger an error in GCM.
	// This saves us implementing detection logic for a case only affecting tiny servers.
	batchDelayMax = 5 * time.Second
)

// ExporterOpts holds options for an exporter.
type ExporterOpts struct {
	// Google Cloud project ID to which data is sent.
	ProjectID string
	// The location identifier used for the monitored resource of exported data.
	Location string
	// The cluster identifier used for the monitored resource of exported data.
	Cluster string

	ExternalLabels string
	// Test endpoint to send data to instead of GCM API
	TestEndpoint string
	// Credentials file for authentication with the GCM API.
	CredentialsFile string
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

	a.Flag("gcm.experimental.project_id", "Google Cloud project ID to which data is sent.").
		Default(opts.ProjectID).StringVar(&opts.ProjectID)

	// The location and cluster flag should probably not be used. On the other hand, they make it easy
	// to populate these important values in the monitored resource without interfering with existing
	// Prometheus configuration.
	a.Flag("gcm.experimental.location", fmt.Sprintf("The location set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", keyLocation)).
		Default(opts.Location).StringVar(&opts.Location)

	a.Flag("gcm.experimental.cluster", fmt.Sprintf("The cluster set for all scraped targets. Prefer setting the external label %q in the Prometheus configuration if not using the auto-discovered default.", keyCluster)).
		Default(opts.Cluster).StringVar(&opts.Cluster)

	a.Flag("gcm.experimental.test_endpoint", "Test endpoint to send data to instead of GCM API.").
		StringVar(&opts.TestEndpoint)

	a.Flag("gcm.experimental.credentials_file", "Credentials file for authentication with the GCM API.").
		StringVar(&opts.CredentialsFile)

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

	e := &Exporter{
		logger: logger,
		opts:   opts,
		nextc:  make(chan struct{}, 1),
		shards: make([]*shard, shardCount),
	}
	e.seriesCache = newSeriesCache(logger, metricsPrefix, e.getExternalLabels)
	e.builder = &sampleBuilder{series: e.seriesCache}

	for i := range e.shards {
		e.shards[i] = newShard(shardBufferSize)
	}

	return e, nil
}

// The target label keys used for the Prometheus monitored resource.
const (
	keyLocation  = "location"
	keyCluster   = "cluster"
	keyNamespace = "namespace"
	keyJob       = "job"
	keyInstance  = "instance"
)

// ApplyConfig updates the exporter state to the given configuration.
func (e *Exporter) ApplyConfig(cfg *config.Config) error {
	e.mtx.Lock()
	defer e.mtx.Unlock()

	// If location and cluster were set through explicit flags or auto-discovery, set
	// them in the external labels.
	builder := labels.NewBuilder(cfg.GlobalConfig.ExternalLabels)

	if e.opts.Location != "" {
		builder.Set(keyLocation, e.opts.Location)
	}
	if e.opts.Cluster != "" {
		builder.Set(keyCluster, e.opts.Cluster)
	}
	lset := builder.Labels()

	// At this point we expect a location to be set. It is very unlikely a user wants to set
	// this dynamically via target label. It would imply collection across failure domains, which
	// is an anti-pattern.
	// The cluster however is allowed to be empty or could feasibly be set through target labels.
	if lset.Get(keyLocation) == "" {
		return errors.Errorf("no label %q set via external labels or flag", keyLocation)
	}
	// TODO(freinartz): invalidate series cache to consider new base labels if they changed.
	e.externalLabels = lset
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
func (e *Exporter) Export(target *scrape.Target, samples []record.RefSample) {
	var (
		sample *monitoring_pb.TimeSeries
		hash   uint64
		err    error
	)
	for len(samples) > 0 {
		sample, hash, samples, err = e.builder.next(target, samples)
		if err != nil {
			panic(err)
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

// Run sends exported samples to Google Cloud Monitoring.
func (e *Exporter) Run(ctx context.Context) error {
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor)),
	}
	if e.opts.TestEndpoint != "" {
		clientOpts = append(clientOpts,
			option.WithEndpoint(e.opts.TestEndpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithInsecure()),
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
	// We largely avoid issue by filling up batches from multiple shards. Under high load,
	// a batch contains samples from fewer shards, under low load from more.
	// The per-shard overhead is minimal and thus a high number can be picked, which allows us
	// to cover a large range of potential throughput and latency combinations without requiring
	// user configuration or, even worse, runtime changes to the shard number.

	var (
		batch = make([]*monitoring_pb.TimeSeries, 0, batchSizeMax)
		// Cache of series hashes already seen in the current batch.
		seen = make(map[uint64]struct{}, batchSizeMax)
		// Functions to be called once the batch has been sent.
		closers = make([]func(), 0, shardCount)
	)

	// Send the currently accumulated batch to GCM asynchronously.
	send := func() {
		pendingRequests.Inc()

		go func(batch []*monitoring_pb.TimeSeries, closers []func()) {
			if err := e.send(ctx, metricClient, batch); err != nil {
				level.Error(e.logger).Log("msg", "send batch", "err", err)
			}
			samplesSent.Add(float64(len(batch)))

			for _, close := range closers {
				close()
			}
			pendingRequests.Dec()
		}(batch, closers)

		// Reset state for new batch.
		stopTimer()
		timer.Reset(batchDelayMax)

		for k := range seen {
			delete(seen, k)
		}

		closers = make([]func(), 0, shardCount)
		batch = make([]*monitoring_pb.TimeSeries, 0, batchSizeMax)
	}

	// Starting index when iterating over shards.
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
				shardProcess.Inc()
				index := (i + shardOffset) % len(e.shards)
				shard := e.shards[index]

				if shard.pending {
					shardProcessPending.Inc()
					continue
				}
				// Populate the batch until it's full or the shard is empty.
				took := 0
				for len(batch) < cap(batch) {
					e, ok := shard.get()
					if !ok {
						break
					}
					// If a series is about to be added that's already in the batch, flush
					// it and start a new one.
					_, hasCollision := seen[e.hash]
					if hasCollision {
						send()
					}
					seen[e.hash] = struct{}{}
					batch = append(batch, e.sample)
					took++

					// We just sent out a batch with data from this shard so we must not
					// gather more data from it.
					if hasCollision {
						break
					}
				}
				shardProcessSamplesTaken.Observe(float64(took))

				if took > 0 {
					shard.setPending(true)
					closers = append(closers, func() { shard.setPending(false) })
				}
				if len(batch) == cap(batch) {
					send()
				}
			}
			// If we didn't make a full pass over all shards, there may be more work.
			if i < len(e.shards) {
				e.triggerNext()
			}
			shardOffset = (shardOffset + i) % len(e.shards)

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
	queue   chan queueEntry
	pending bool
}

type queueEntry struct {
	hash   uint64
	sample *monitoring_pb.TimeSeries
}

func newShard(queueSize int) *shard {
	return &shard{queue: make(chan queueEntry, queueSize)}
}

// get oldest queue entry if it exists.
func (s *shard) get() (queueEntry, bool) {
	select {
	case e, ok := <-s.queue:
		return e, ok
	default:
	}
	return queueEntry{}, false
}

func (s *shard) enqueue(hash uint64, sample *monitoring_pb.TimeSeries) {
	samplesExported.Inc()

	e := queueEntry{
		hash:   hash,
		sample: sample,
	}
	select {
	case s.queue <- e:
	default:
		// TODO(freinartz): tail drop is not a great solution. Once we have the WAL buffer,
		// we can just block here when enqueueing from it.
		samplesDropped.Inc()
	}
}

func (s *shard) setPending(b bool) {
	s.mtx.Lock()
	// This case should never happen in our usage of shards unless there is a bug.
	if s.pending == b {
		panic(fmt.Sprintf("pending set to %v while it already was", b))
	}
	s.pending = b
	s.mtx.Unlock()
}
