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
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"reflect"
	"runtime/debug"
	"strings"
	"sync"
	"testing"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	gax "github.com/googleapis/gax-go/v2"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
)

var (
	gcmExportCalledWhileDisabled = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_called_while_disabled_total",
		Help: "Number of calls to export while metric exporting was disabled.",
	})
	samplesExported = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_exported_total",
		Help: "Number of samples exported at scrape time.",
	})
	exemplarsExported = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_exemplars_exported_total",
		Help: "Number of exemplars exported at scrape time.",
	})
	samplesDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gcm_export_samples_dropped_total",
		Help: "Number of exported samples that were intentionally dropped.",
	}, []string{"reason"})
	exemplarsDropped = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gcm_export_exemplars_dropped_total",
		Help: "Number of exported exemplars that were intentionally dropped.",
	}, []string{"reason"})
	samplesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "gcm_export_samples_sent_total",
		Help: "Number of exported samples sent to GCM.",
	})
	samplesSendErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "gcm_export_samples_send_errors_total",
		Help: "Number of errors encountered while sending samples to GCM",
	}, []string{"project_id"})
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
	ErrLocationGlobal = errors.New("Location must be set to a named Google Cloud " +
		"region and cannot be set to \"global\". Please choose the " +
		"Google Cloud region that is physically nearest to your cluster. " +
		"See https://www.cloudinfrastructuremap.com/")
)

type metricServiceClient interface {
	Close() error
	CreateTimeSeries(context.Context, *monitoring_pb.CreateTimeSeriesRequest, ...gax.CallOption) error
}

// Exporter converts Prometheus samples into Cloud Monitoring samples and exports them.
type Exporter struct {
	logger log.Logger
	ctx    context.Context
	opts   ExporterOpts

	metricClient metricServiceClient
	seriesCache  *seriesCache
	shards       []*shard

	// Channel for signaling that there may be more work items to
	// be processed.
	nextc chan struct{}

	// Channel for signaling to exit the exporter. Used to indicate
	// data is done exporting during unit tests.
	exitc chan struct{}
	// The external labels may be updated asynchronously by configuration changes
	// and must be locked with mtx.
	mtx            sync.RWMutex
	externalLabels labels.Labels
	// A set of metrics for which we defaulted the metadata to untyped and have
	// issued a warning about that.
	warnedUntypedMetrics map[string]struct{}

	// A lease on a time range for which the exporter send sample data.
	// It is checked for on each batch provided to the Export method.
	// If unset, data is always sent.
	lease Lease

	// Used to construct a new metric client when options change, or at initialization. It
	// is exposed as a variable so that unit tests may change the constructor.
	newMetricClient func(ctx context.Context, opts ExporterOpts) (metricServiceClient, error)
}

const (
	// DefaultShardCount represents number of shards by which series are bucketed.
	DefaultShardCount = 1024
	// DefaultShardBufferSize represents the buffer size for each individual shard.
	// Each element in buffer (queue) consists of sample and hash.
	DefaultShardBufferSize = 2048

	// BatchSizeMax represents maximum number of samples to pack into a batch sent to GCM.
	BatchSizeMax = 200
	// Time after an accumulating batch is flushed to GCM. This avoids data being
	// held indefinititely if not enough new data flows in to fill up the batch.
	batchDelayMax = 50 * time.Millisecond
	// Time after context is cancelled that we use to flush the remaining buffered data.
	// This avoids data loss on shutdown.
	cancelTimeout = 15 * time.Second
	// Time after the final shards are drained before the exporter is closed on shutdown.
	flushTimeout = 100 * time.Millisecond
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
	// CredentialsFromJSON represents content of credentials file for
	// authentication with the GCM API. CredentialsFile has priority over this.
	CredentialsFromJSON []byte
	// Disable authentication (for debugging purposes).
	DisableAuth bool
	// A user agent product string added to the regular user agent.
	// See: https://www.rfc-editor.org/rfc/rfc7231#section-5.5.3
	UserAgentProduct string
	// A string added as a suffix to the regular user agent.
	UserAgentMode string
	// UserAgentEnv where calls to GCM API are made.
	UserAgentEnv string

	// Default monitored resource fields set on exported data.
	ProjectID string
	Location  string
	Cluster   string

	// A list of metric matchers. Only Prometheus time series satisfying at
	// least one of the matchers are exported.
	// This option matches the semantics of the Prometheus federation match[]
	// parameter.
	Matchers Matchers

	// Prefix under which metrics are written to GCM.
	MetricTypePrefix string

	// Request URL and body for generating an alternative GCE token source.
	// This allows metrics to be exported to an alternative project.
	TokenURL  string
	TokenBody string

	// The project ID of an alternative project for quota attribution.
	QuotaProject string

	// Efficiency represents exporter options that allows fine-tuning of
	// internal data structure sizes. Only for advance users. No compatibility
	// guarantee (might change in future).
	Efficiency EfficiencyOpts
}

// DefaultUnsetFields defaults any zero-valued fields.
func (opts *ExporterOpts) DefaultUnsetFields() {
	if opts.Efficiency.BatchSize == 0 {
		opts.Efficiency.BatchSize = BatchSizeMax
	}
	if opts.Efficiency.ShardCount == 0 {
		opts.Efficiency.ShardCount = DefaultShardCount
	}
	if opts.Efficiency.ShardBufferSize == 0 {
		opts.Efficiency.ShardBufferSize = DefaultShardBufferSize
	}

	if opts.Endpoint == "" {
		opts.Endpoint = "monitoring.googleapis.com:443"
	}
	if opts.Compression == "" {
		opts.Compression = CompressionNone
	}
	if opts.MetricTypePrefix == "" {
		opts.MetricTypePrefix = MetricTypePrefix
	}
	if opts.UserAgentMode == "" {
		opts.UserAgentMode = "unspecified"
	}
}

func (opts *ExporterOpts) Validate() error {
	if opts.Efficiency.BatchSize > BatchSizeMax {
		return fmt.Errorf("maximum supported batch size is %d, got %d", BatchSizeMax, opts.Efficiency.BatchSize)
	}
	return nil
}

// EfficiencyOpts represents exporter options that allows fine-tuning of
// internal data structure sizes. Only for advance users. No compatibility
// guarantee (might change in future).
type EfficiencyOpts struct {
	// BatchSize controls a maximum batch size to use when sending data to the GCM
	// API. Defaults to BatchSizeMax when 0. The BatchSizeMax is also
	// the maximum number this field can have due to GCM quota for write requests
	// size. See https://cloud.google.com/monitoring/quotas?hl=en#custom_metrics_quotas.
	BatchSize uint

	// ShardCount controls number of shards. Refer to Exporter.Run documentation
	// to learn more about algorithm. Defaults to DefaultShardCount when 0.
	ShardCount uint
	// ShardBufferSize controls the size for each individual shard. Each element
	// in buffer (queue) consists of sample and hash. Refer to Exporter.Run
	// documentation to learn more about algorithm. Defaults to
	// DefaultShardBufferSize when 0.
	ShardBufferSize uint
}

// NopExporter returns a permanently inactive exporter.
func NopExporter() *Exporter {
	return &Exporter{
		opts: ExporterOpts{
			Disable: true,
		},
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

// NopLease returns a lease that disables leasing.
func NopLease() Lease {
	return &alwaysLease{}
}

// alwaysLease is a lease that is always held.
type alwaysLease struct{}

func (alwaysLease) Range() (time.Time, time.Time, bool) {
	return time.UnixMilli(math.MinInt64), time.UnixMilli(math.MaxInt64), true
}

func (alwaysLease) Run(ctx context.Context) {
	<-ctx.Done()
}

func (alwaysLease) OnLeaderChange(func()) {
	// We never lose the lease as it's always owned.
}

func defaultNewMetricClient(ctx context.Context, opts ExporterOpts) (metricServiceClient, error) {
	version, err := Version()
	if err != nil {
		return nil, fmt.Errorf("unable to fetch user agent version: %w", err)
	}

	// Identity User Agent for all gRPC requests.
	ua := strings.TrimSpace(fmt.Sprintf("%s/%s %s (env:%s;mode:%s)",
		ClientName, version, opts.UserAgentProduct, opts.UserAgentEnv, opts.UserAgentMode))

	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(grpc_prometheus.UnaryClientInterceptor)),
		option.WithUserAgent(ua),
	}
	if opts.Endpoint != "" {
		clientOpts = append(clientOpts, option.WithEndpoint(opts.Endpoint))
	}
	// Disable auth when the exporter is disabled because we don't want a panic when default
	// credentials are not found.
	if opts.DisableAuth || opts.Disable {
		clientOpts = append(clientOpts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	} else if opts.CredentialsFile == "" && len(opts.CredentialsFromJSON) == 0 {
		// If no credentials are found, gRPC panics so we check manually.
		_, err := google.FindDefaultCredentials(ctx)
		if err != nil {
			return nil, err
		}
	}
	if opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(opts.CredentialsFile))
	} else if len(opts.CredentialsFromJSON) > 0 {
		clientOpts = append(clientOpts, option.WithCredentialsJSON(opts.CredentialsFromJSON))
	}

	if opts.TokenURL != "" && opts.TokenBody != "" {
		tokenSource := NewAltTokenSource(opts.TokenURL, opts.TokenBody)
		clientOpts = append(clientOpts, option.WithTokenSource(tokenSource))
	}
	if opts.QuotaProject != "" {
		clientOpts = append(clientOpts, option.WithQuotaProject(opts.QuotaProject))
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
func New(ctx context.Context, logger log.Logger, reg prometheus.Registerer, opts ExporterOpts, lease Lease) (*Exporter, error) {
	grpc_prometheus.EnableClientHandlingTimeHistogram(
		grpc_prometheus.WithHistogramBuckets([]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 15, 20, 30, 40, 50, 60}),
	)

	if logger == nil {
		logger = log.NewNopLogger()
	}
	if reg != nil {
		reg.MustRegister(
			prometheusSamplesDiscarded,
			samplesExported,
			samplesDropped,
			samplesSent,
			samplesSendErrors,
			sendIterations,
			shardProcess,
			shardProcessPending,
			shardProcessSamplesTaken,
			pendingRequests,
			projectsPerBatch,
			samplesPerRPCBatch,
			gcmExportCalledWhileDisabled,
		)
	}

	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if lease == nil {
		lease = NopLease()
	}

	metricClient, err := defaultNewMetricClient(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("create metric client: %w", err)
	}

	e := &Exporter{
		logger:               logger,
		ctx:                  ctx,
		opts:                 opts,
		metricClient:         metricClient,
		seriesCache:          newSeriesCache(logger, reg, opts.MetricTypePrefix, opts.Matchers),
		externalLabels:       createLabelSet(&config.Config{}, &opts),
		newMetricClient:      defaultNewMetricClient,
		nextc:                make(chan struct{}, 1),
		exitc:                make(chan struct{}, 1),
		shards:               make([]*shard, opts.Efficiency.ShardCount),
		warnedUntypedMetrics: map[string]struct{}{},
		lease:                lease,
	}

	// Whenever the lease is lost, clear the series cache so we don't start off of out-of-range
	// reset timestamps when we gain the lease again.
	lease.OnLeaderChange(e.seriesCache.clear)

	for i := range e.shards {
		e.shards[i] = newShard(opts.Efficiency.ShardBufferSize)
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

// ApplyConfig updates the exporter state to the given configuration. The given `ExporterOpts`,
// if non-nil, is applied to the exporter, potentially recreating the metric client. It must be
// defaulted and validated.
func (e *Exporter) ApplyConfig(cfg *config.Config, opts *ExporterOpts) (err error) {
	// Note: We don't expect the NopExporter to call this. Only the config reloader calls it.
	e.mtx.Lock()
	defer e.mtx.Unlock()

	// Don't recreate the metric client each time. If the metric client is recreated, it has to
	// potentially redo the TCP handshake. With HTTP/2, TCP connections are kept alive for a small
	// amount of time to reduce load when multiple requests are made to the same server in
	// succession. In our case, we might send a CMP call every 50ms at the worst case, which is
	// highly likely to benefit from the persistent TPC connection.
	optsChanged := false
	if opts != nil {
		optsChanged = !reflect.DeepEqual(e.opts, opts)
		if optsChanged {
			e.opts = *opts
		}
	}

	lset := createLabelSet(cfg, &e.opts)
	labelsChanged := !labels.Equal(e.externalLabels, lset)

	// We don't need to validate if there's no scrape configs or rules, i.e. at startup.
	hasScrapeConfigs := len(cfg.ScrapeConfigs) != 0 || len(cfg.ScrapeConfigFiles) != 0
	hasRules := len(cfg.RuleFiles) != 0
	if hasScrapeConfigs || hasRules {
		if err := validateLabelSet(lset); err != nil {
			return err
		}
	}

	// If changed, or we're calling this for the first time, we need to recreate the client.
	if optsChanged {
		e.metricClient, err = e.newMetricClient(e.ctx, e.opts)
		if err != nil {
			return fmt.Errorf("create metric client: %w", err)
		}
	}

	if labelsChanged {
		e.externalLabels = lset
		// New external labels possibly invalidate the cached series conversions.
		e.seriesCache.forceRefresh()
	}

	return nil
}

func createLabelSet(cfg *config.Config, opts *ExporterOpts) labels.Labels {
	// If project_id, location, or cluster were set through the external_labels in the config
	// file, these values take precedence. If they are unset, the flag value, which defaults
	// to an environment-specific value on GCE/GKE, is used.
	builder := labels.NewBuilder(cfg.GlobalConfig.ExternalLabels)

	if !cfg.GlobalConfig.ExternalLabels.Has(KeyProjectID) {
		builder.Set(KeyProjectID, opts.ProjectID)
	}
	if !cfg.GlobalConfig.ExternalLabels.Has(KeyLocation) {
		builder.Set(KeyLocation, opts.Location)
	}
	if !cfg.GlobalConfig.ExternalLabels.Has(KeyCluster) {
		builder.Set(KeyCluster, opts.Cluster)
	}
	return builder.Labels()
}

func validateLabelSet(lset labels.Labels) error {
	// We expect location and project ID to be set. They are effectively only a default
	// however as they may be overridden by metric labels.
	if lset.Get(KeyProjectID) == "" {
		return fmt.Errorf("no label %q set via external labels or flag", KeyProjectID)
	}

	// In production scenarios, "location" should most likely never be overridden as it
	// means crossing failure domains. Instead, each location should run a replica of the
	// evaluator with the same rules.
	if loc := lset.Get(KeyLocation); loc == "" {
		return fmt.Errorf("no label %q set via external labels or flag", KeyLocation)
	} else if loc == "global" {
		return ErrLocationGlobal
	}
	return nil
}

// SetLabelsByIDFunc injects a function that can be used to retrieve a label set
// based on a series ID we got through exported sample records.
// Must be called before any call to Export is made.
func (e *Exporter) SetLabelsByIDFunc(f func(storage.SeriesRef) labels.Labels) {
	if e.seriesCache == nil {
		// We don't have a cache in a nop exporter, so we skip.
		return
	}
	if e.seriesCache.getLabelsByRef != nil {
		panic("SetLabelsByIDFunc must only be called once")
	}
	e.seriesCache.getLabelsByRef = f
}

// Export enqueues the samples and exemplars to be written to Cloud Monitoring.
func (e *Exporter) Export(metadata MetadataFunc, batch []record.RefSample, exemplarMap map[storage.SeriesRef]record.RefExemplar) {
	if e.opts.Disable {
		gcmExportCalledWhileDisabled.Inc()
		return
	}
	// Wether we're sending data or not, add batchsize of samples exported by
	// Prometheus from appender commit.
	batchSize := len(batch)
	samplesExported.Add(float64(batchSize))

	metadata = e.wrapMetadata(metadata)

	e.mtx.Lock()
	externalLabels := e.externalLabels
	start, end, ok := e.lease.Range()
	e.mtx.Unlock()

	if !ok {
		exemplarsDropped.WithLabelValues("not-in-ha-range").Add(float64(len(exemplarMap)))
		samplesDropped.WithLabelValues("not-in-ha-range").Add(float64(batchSize))
		return
	}
	builder := newSampleBuilder(e.seriesCache)
	defer builder.close()
	exemplarsExported.Add(float64(len(exemplarMap)))

	for len(batch) > 0 {
		var (
			samples []hashedSeries
			err     error
		)
		samples, batch, err = builder.next(metadata, externalLabels, batch, exemplarMap)
		if err != nil {
			//nolint:errcheck
			level.Debug(e.logger).Log("msg", "building sample failed", "err", err)
			continue
		}
		for _, s := range samples {
			// Only enqueue samples for within our HA range.
			if sampleInRange(s.proto, start, end) {
				e.enqueue(s.hash, s.proto)
			} else {
				// Hashed series protos should only ever have one point. If this is
				// a distribution increase exemplarsDropped if there are exemplars.
				if dist := s.proto.Points[0].Value.GetDistributionValue(); dist != nil {
					exemplarsDropped.WithLabelValues("not-in-ha-range").Add(float64(len(dist.GetExemplars())))
				}
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

const (
	// ClientName is used to identify the User Agent.
	ClientName = "prometheus-engine-export"
	// mainModuleVersion is the version of the main module. Align with git tag.
	// TODO(TheSpiritXIII): Remove with https://github.com/golang/go/issues/50603
	mainModuleVersion = "v0.13.0-rc.0" // x-release-please-version
	// mainModuleName is the name of the main module. Align with go.mod.
	mainModuleName = "github.com/GoogleCloudPlatform/prometheus-engine"
)

// Version is used in the User Agent. This version is automatically detected if
// this function is imported as a library. However, the version is statically
// set if this function is used in a binary in prometheus-engine due to Golang
// restrictions. While testing, the static version is validated for correctness.
func Version() (string, error) {
	if testing.Testing() {
		// TODO(TheSpiritXIII): After https://github.com/golang/go/issues/50603 just return an empty
		// string here. For now, use the opportunity to confirm that the static version is correct.
		// We manually get the closest git tag if the user is running the unit test locally, but
		// fallback to the GIT_TAG environment variable in case the user is running the test via
		// Docker (like `make test` does by default).
		if testTag, found := os.LookupEnv("TEST_TAG"); !found || testTag == "false" {
			return mainModuleVersion, nil
		}
		cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
		var stdout bytes.Buffer
		cmd.Stdout = &stdout

		version := ""
		if err := cmd.Run(); err != nil {
			version = strings.TrimSpace(os.Getenv("GIT_TAG"))
			if version == "" {
				return "", errors.New("unable to detect git tag, please set GIT_TAG env variable")
			}
		} else {
			version = strings.TrimSpace(stdout.String())
		}

		return version, nil
	}

	// TODO(TheSpiritXIII): Due to https://github.com/golang/go/issues/50603 we must use a static
	// string for the main module (when we import this function locally for binaries).

	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", fmt.Errorf("unable to retrieve build info")
	}

	if bi.Main.Path == mainModuleName {
		return mainModuleVersion, nil
	}

	var exportDep *debug.Module
	for _, dep := range bi.Deps {
		if dep.Path == mainModuleName {
			exportDep = dep
			break
		}
	}
	if exportDep == nil {
		return "", fmt.Errorf("unable to find module %q %v", mainModuleName, bi.Deps)
	}
	return exportDep.Version, nil
}

// Run sends exported samples to Google Cloud Monitoring. Must be called at most once.
//
// Run starts a loop that gathers samples and sends them to GCM.
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
func (e *Exporter) Run() error {
	// Note: We don't expect the NopExporter to call this. Only the main binary calls this.
	defer e.close()
	go e.seriesCache.run(e.ctx)
	go e.lease.Run(e.ctx)

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

	e.mtx.RLock()
	opts := e.opts
	e.mtx.RUnlock()

	curBatch := newBatch(e.logger, opts.Efficiency.ShardCount, opts.Efficiency.BatchSize)

	// Send the currently accumulated batch to GCM asynchronously.
	send := func() {
		e.mtx.RLock()
		opts := e.opts
		if e.metricClient == nil {
			// Flush timeout reached, runner is shut down.
			e.mtx.RUnlock()
			return
		}
		sendFunc := e.metricClient.CreateTimeSeries
		e.mtx.RUnlock()

		// Send the batch and once it completed, trigger next to process remaining data in the
		// shards that were part of the batch. This ensures that if we didn't take all samples
		// from a shard when filling the batch, we'll come back for them and any queue built-up
		// gets sent eventually.
		go func(ctx context.Context, b *batch) {
			if !opts.Disable {
				b.send(ctx, sendFunc)
			}
			// We could only trigger if we didn't fully empty shards in this batch.
			// Benchmarking showed no beneficial impact of this optimization.
			e.triggerNext()
		}(e.ctx, curBatch)

		// Reset state for new batch.
		stopTimer()
		timer.Reset(batchDelayMax)

		curBatch = newBatch(e.logger, opts.Efficiency.ShardCount, opts.Efficiency.BatchSize)
	}

	// Try to drain the remaining data before exiting or the time limit (15 seconds) expires.
	// A sleep timer is added after draining the shards to ensure it has time to be sent.
	drainShardsBeforeExiting := func() {
		//nolint:errcheck
		level.Info(e.logger).Log("msg", "Exiting Exporter - will attempt to send remaining data in the next 15 seconds.")
		exitTimer := time.NewTimer(cancelTimeout)
		drained := make(chan struct{}, 1)
		go func() {
			for {
				totalRemaining := 0
				pending := false
				for _, shard := range e.shards {
					_, remaining := shard.fill(curBatch)
					totalRemaining += remaining
					shard.mtx.Lock()
					pending = pending || shard.pending
					shard.mtx.Unlock()
					if !curBatch.empty() {
						send()
					}
				}
				if totalRemaining == 0 && !pending {
					// NOTE(ridwanmsharif): the sending of the batches happen asyncronously
					// and we only wait for a fixed amount of time after the final batch is sent
					// before shutting down the exporter.
					time.Sleep(flushTimeout)
					drained <- struct{}{}
				}
			}
		}()
		for {
			select {
			case <-exitTimer.C:
				//nolint:errcheck
				level.Info(e.logger).Log("msg", "Exiting Exporter - Data wasn't sent within the timeout limit.")
				samplesDropped.WithLabelValues("Data wasn't sent within the timeout limit.")
				return
			case <-drained:
				return
			}
		}
	}

	for {
		select {
		// NOTE(freinartz): we will terminate once context is cancelled and not flush remaining
		// buffered data. In-flight requests will be aborted as well.
		// This is fine once we persist data submitted via Export() but for now there may be some
		// data loss on shutdown.
		case <-e.ctx.Done():
			// on termination, try to drain the remaining shards within the CancelTimeout.
			// This is done to prevent data loss during a shutdown.
			drainShardsBeforeExiting()
			// This channel is used for unit test case.
			e.exitc <- struct{}{}
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
			for _, shard := range e.shards {
				shard.fill(curBatch)
				if curBatch.full() {
					send()
				}
			}

		case <-timer.C:
			// Flush batch that has been pending for too long.
			if !curBatch.empty() {
				send()
			} else {
				timer.Reset(batchDelayMax)
			}
		}
	}
}

func (e *Exporter) close() {
	e.mtx.Lock()
	defer e.mtx.Unlock()
	if err := e.metricClient.Close(); err != nil {
		//nolint:errcheck
		e.logger.Log("msg", "error closing metric client", "err", err)
	}
	e.metricClient = nil
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
			//nolint:errcheck
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
	shards  []*shard
	oneFull bool
	total   int
}

func newBatch(logger log.Logger, shardsCount uint, maxSize uint) *batch {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &batch{
		logger:  logger,
		maxSize: maxSize,
		m:       make(map[string][]*monitoring_pb.TimeSeries, 1),
		shards:  make([]*shard, 0, shardsCount/2),
	}
}

func (b *batch) addShard(s *shard) {
	b.shards = append(b.shards, s)
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
				//nolint:errcheck
				level.Error(b.logger).Log("msg", "send batch", "size", len(l), "err", err)
				samplesSendErrors.WithLabelValues(pid).Inc()
			}
			samplesSent.Add(float64(len(l)))
		}(pid, l)
	}
	wg.Wait()

	for _, s := range b.shards {
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
		return fmt.Errorf("invalid metric matcher %q: %w", s, err)
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
