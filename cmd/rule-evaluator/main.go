// Copyright 2022 Google LLC
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

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	exportsetup "github.com/GoogleCloudPlatform/prometheus-engine/pkg/export/setup"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/oklog/run"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"

	// Import to enable 'kubernetes_sd_configs' to SD config register.
	_ "github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/strutil"
)

const projectIDVar = "PROJECT_ID"

func main() {
	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	a := kingpin.New("rule", "The Prometheus Rule Evaluator")

	a.HelpFlag.Short('h')

	var defaultProjectID string
	if metadata.OnGCE() {
		var err error
		defaultProjectID, err = metadata.ProjectID()
		if err != nil {
			_ = level.Warn(logger).Log("msg", "Unable to detect Google Cloud project", "err", err)
		}
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		grpc_prometheus.DefaultClientMetrics,
	)

	// The rule-evaluator version is identical to the export library version for now, so
	// we reuse that constant.
	version, err := export.Version()
	if err != nil {
		_ = level.Error(logger).Log("msg", "Unable to fetch module version", "err", err)
		os.Exit(1)
	}

	exporterOpts := export.ExporterOpts{
		UserAgentProduct: fmt.Sprintf("rule-evaluator/%s", version),
	}
	exportsetup.ExporterOptsFlags(a, &exporterOpts)

	metadataOpts := exportsetup.MetadataOpts{}
	metadataOpts.SetupFlags(a)

	haOpts := exportsetup.HAOptions{}
	haOpts.SetupFlags(a)

	evaluatorOpts := evaluatorOptions{
		TargetURL:     Must(url.Parse(fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", projectIDVar))),
		ProjectID:     defaultProjectID,
		DisableAuth:   false,
		ListenAddress: ":9091",
		ConfigFile:    "prometheus.yml",
		QueueCapacity: 10000,
	}
	evaluatorOpts.setupFlags(a)

	extraArgs, err := exportsetup.ExtraArgs()
	if err != nil {
		_ = level.Error(logger).Log("msg", "Error parsing commandline arguments", "err", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}
	if _, err := a.Parse(append(os.Args[1:], extraArgs...)); err != nil {
		_ = level.Error(logger).Log("msg", "Error parsing commandline arguments", "err", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	if err := evaluatorOpts.validate(); err != nil {
		_ = level.Error(logger).Log("msg", "invalid command line argument", "err", err)
		os.Exit(1)
	}

	startTime := time.Now()
	ctx := context.Background()

	metadataOpts.ExtractMetadata(logger, &exporterOpts)
	lease, err := haOpts.NewLease(logger, reg)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Unable to setup Cloud Monitoring Exporter lease", "err", err)
		os.Exit(1)
	}
	exporter, err := export.New(ctx, logger, reg, exporterOpts, lease)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Creating a Cloud Monitoring Exporter failed", "err", err)
		os.Exit(1)
	}
	destination := export.NewStorage(exporter)

	ctxDiscover, cancelDiscover := context.WithCancel(ctx)
	discoveryManager := discovery.NewManager(ctxDiscover, log.With(logger, "component", "discovery manager notify"), discovery.Name("notify"))
	notifierOptions := notifier.Options{
		Registerer:    reg,
		QueueCapacity: evaluatorOpts.QueueCapacity,
	}
	notificationManager := notifier.NewManager(&notifierOptions, log.With(logger, "component", "notifier"))
	rulesMetrics := rules.NewGroupMetrics(reg)
	ruleEvaluator, err := newRuleEvaluator(ctx, logger, reg, &evaluatorOpts, version, destination, notificationManager, rulesMetrics)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Create rule-evaluator", "err", err)
		os.Exit(1)
	}

	reloaders := []reloader{
		{
			name:     "notify",
			reloader: notificationManager.ApplyConfig,
		}, {
			name:     "exporter",
			reloader: destination.ApplyConfig,
		}, {
			name: "notify_sd",
			reloader: func(cfg *config.Config) error {
				c := make(map[string]discovery.Configs)
				for k, v := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
					c[k] = v.ServiceDiscoveryConfigs
				}
				return discoveryManager.ApplyConfig(c)
			},
		}, {
			name:     "rules",
			reloader: ruleEvaluator.ApplyConfig,
		},
	}

	configMetrics := newConfigMetrics(reg)

	// Do an initial load of the configuration for all components.
	if err := reloadConfig(evaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
		_ = level.Error(logger).Log("msg", "error loading config file.", "err", err)
		os.Exit(1)
	}

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		cancel := make(chan struct{})
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		g.Add(
			func() error {
				select {
				case <-term:
					_ = level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
				case <-cancel:
				}
				return nil
			},
			func(error) {
				close(cancel)
			},
		)
	}
	{
		// Rule manager.
		g.Add(func() error {
			ruleEvaluator.Run()
			return nil
		}, func(error) {
			ruleEvaluator.Stop()
		})
	}
	{
		// Notifier.
		g.Add(func() error {
			notificationManager.Run(discoveryManager.SyncCh())
			_ = level.Info(logger).Log("msg", "Notification manager stopped")
			return nil
		},
			func(error) {
				notificationManager.Stop()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManager.Run()
				_ = level.Info(logger).Log("msg", "Discovery manager stopped")
				return err
			},
			func(error) {
				_ = level.Info(logger).Log("msg", "Stopping Discovery manager...")
				cancelDiscover()
			},
		)
	}
	{
		// Storage Processing.
		ctxStorage, cancelStorage := context.WithCancel(ctx)
		g.Add(func() error {
			err = destination.Run(ctxStorage)
			_ = level.Info(logger).Log("msg", "Background processing of storage stopped")
			return err
		}, func(error) {
			_ = level.Info(logger).Log("msg", "Stopping background storage processing...")
			cancelStorage()
		})
	}
	cwd, err := os.Getwd()
	reloadCh := make(chan chan error)
	{
		// Web Server.
		server := &http.Server{Addr: evaluatorOpts.ListenAddress}

		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		http.HandleFunc("/-/reload", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				rc := make(chan error)
				reloadCh <- rc
				if err := <-rc; err != nil {
					http.Error(w, fmt.Sprintf("Failed to reload config: %s", err), http.StatusInternalServerError)
				}
			} else {
				http.Error(w, "Only POST requests allowed.", http.StatusMethodNotAllowed)
			}
		})
		http.HandleFunc("/-/healthy", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		http.HandleFunc("/-/ready", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "rule-evaluator is Ready.\n")
		})
		// https://prometheus.io/docs/prometheus/latest/querying/api/#runtime-information
		// Useful for knowing whether a config reload was successful.
		http.HandleFunc("/api/v1/status/runtimeinfo", func(w http.ResponseWriter, _ *http.Request) {
			runtimeInfo := apiv1.RuntimeInfo{
				StartTime:           startTime,
				CWD:                 cwd,
				GoroutineCount:      runtime.NumGoroutine(),
				GOMAXPROCS:          runtime.GOMAXPROCS(0),
				GOMEMLIMIT:          debug.SetMemoryLimit(-1),
				GOGC:                os.Getenv("GOGC"),
				GODEBUG:             os.Getenv("GODEBUG"),
				StorageRetention:    "0d",
				CorruptionCount:     0,
				ReloadConfigSuccess: configMetrics.lastReloadSuccess,
				LastConfigTime:      configMetrics.lastReloadSuccessTime,
			}
			response := response{
				Status: "success",
				Data:   runtimeInfo,
			}
			data, err := json.Marshal(response)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to marshal status: %s", err), http.StatusInternalServerError)
				return
			}

			if _, err := w.Write(data); err != nil {
				_ = level.Error(logger).Log("msg", "Unable to write runtime info status", "err", err)
			}
		})
		g.Add(func() error {
			_ = level.Info(logger).Log("msg", "Starting web server", "listen", evaluatorOpts.ListenAddress)
			return server.ListenAndServe()
		}, func(error) {
			ctxServer, cancelServer := context.WithTimeout(ctx, time.Minute)
			if err := server.Shutdown(ctxServer); err != nil {
				_ = level.Error(logger).Log("msg", "Server failed to shut down gracefully.")
			}
			cancelServer()
		})
	}
	{
		// Reload handler.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				for {
					select {
					case <-hup:
						if err := reloadConfig(evaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
							_ = level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-reloadCh:
						if err := reloadConfig(evaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
							_ = level.Error(logger).Log("msg", "Error reloading config", "err", err)
							rc <- err
						} else {
							rc <- nil
						}
					case <-cancel:
						return nil
					}
				}
			},
			func(error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}

	// Run a test query to check status of rule evaluator.
	_, err = ruleEvaluator.Query(ctx, "vector(1)", time.Now())
	if err != nil {
		_ = level.Error(logger).Log("msg", "Error querying Prometheus instance", "err", err)
	}

	if err := g.Run(); err != nil {
		_ = level.Error(logger).Log("msg", "Running rule evaluator failed", "err", err)
		os.Exit(1)
	}
}

// Must panics if there's any error.
func Must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

type evaluatorOptions struct {
	TargetURL       *url.URL
	ProjectID       string
	GeneratorURL    *url.URL
	CredentialsFile string
	DisableAuth     bool
	ListenAddress   string
	ConfigFile      string
	QueueCapacity   int
}

func (opts *evaluatorOptions) setupFlags(a *kingpin.Application) {
	a.Flag("query.project-id", "Project ID of the Google Cloud Monitoring scoping project to evaluate rules against.").
		Default(opts.ProjectID).
		StringVar(&opts.ProjectID)

	a.Flag("query.target-url", fmt.Sprintf("The address of the Prometheus server query endpoint. (%s is replaced with the --query.project-id flag.)", projectIDVar)).
		Default(opts.TargetURL.String()).
		URLVar(&opts.TargetURL)

	a.Flag("query.generator-url", "The base URL used for the generator URL in the alert notification payload. Should point to an instance of a query frontend that accesses the same data as --query.target-url.").
		PlaceHolder("<URL>").
		URLVar(&opts.GeneratorURL)

	a.Flag("query.credentials-file", "Credentials file for OAuth2 authentication with --query.target-url.").
		PlaceHolder("<FILE>").
		StringVar(&opts.CredentialsFile)

	a.Flag("query.debug.disable-auth", "Disable authentication (for debugging purposes).").
		Default("false").
		BoolVar(&opts.DisableAuth)

	a.Flag("web.listen-address", "The address to listen on for HTTP requests.").
		Default(":9091").
		StringVar(&opts.ListenAddress)

	a.Flag("config.file", "Prometheus configuration file path.").
		Default(opts.ConfigFile).
		StringVar(&opts.ConfigFile)

	a.Flag("alertmanager.notification-queue-capacity", "The capacity of the queue for pending Alertmanager notifications.").
		Default(strconv.Itoa(opts.QueueCapacity)).
		IntVar(&opts.QueueCapacity)
}

func (opts *evaluatorOptions) validate() error {
	if opts.ProjectID == "" {
		return errors.New("no --query.project-id was specified or could be derived from the environment")
	}

	targetURL, err := url.Parse(strings.ReplaceAll(opts.TargetURL.String(), projectIDVar, opts.ProjectID))
	if err != nil {
		return fmt.Errorf("unable to parse --query.target-url value %q: %w", opts.TargetURL.String(), err)
	}
	opts.TargetURL = targetURL

	// Don't expand external labels on config file loading. It's a feature we like but we want to
	// remain compatible with Prometheus and this is still an experimental feature, which we don't
	// support.
	if _, err := config.LoadFile(opts.ConfigFile, false, false, log.NewNopLogger()); err != nil {
		return fmt.Errorf("loading config %q: %w", opts.ConfigFile, err)
	}
	return nil
}

func newAPI(ctx context.Context, opts *evaluatorOptions, version string, roundTripperFunc func(http.RoundTripper) http.RoundTripper) (v1.API, error) {
	clientOpts := []option.ClientOption{
		option.WithScopes("https://www.googleapis.com/auth/monitoring.read"),
		option.WithUserAgent(fmt.Sprintf("rule-evaluator/%s", version)),
	}
	if opts.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(opts.CredentialsFile))
	}
	if opts.DisableAuth {
		clientOpts = append(clientOpts,
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
	}
	transport, err := apihttp.NewTransport(ctx, http.DefaultTransport, clientOpts...)
	if err != nil {
		return nil, err
	}
	roundTripper := roundTripperFunc(transport)
	client, err := api.NewClient(api.Config{
		Address:      opts.TargetURL.String(),
		RoundTripper: roundTripper,
	})
	if err != nil {
		return nil, err
	}
	return v1.NewAPI(client), nil
}

// response wraps all Prometheus API responses.
type response struct {
	Status string `json:"status"`
	Data   any    `json:"data,omitempty"`
}

// QueryFunc queries a Prometheus instance and returns a promql.Vector.
func QueryFunc(ctx context.Context, q string, t time.Time, v1api v1.API) (parser.Value, v1.Warnings, error) {
	results, warnings, err := v1api.Query(ctx, q, t)
	if err != nil {
		return nil, warnings, fmt.Errorf("Error querying Prometheus: %w", err)
	}
	v, err := convertModelToPromQLValue(results)
	return v, warnings, err
}

// sendAlerts returns the rules.NotifyFunc for a Notifier.
func sendAlerts(s *notifier.Manager, externalURL *url.URL) rules.NotifyFunc {
	return func(_ context.Context, expr string, alerts ...*rules.Alert) {
		var res []*notifier.Alert
		for _, alert := range alerts {
			a := &notifier.Alert{
				StartsAt:    alert.FiredAt,
				Labels:      alert.Labels,
				Annotations: alert.Annotations,
			}
			if !alert.ResolvedAt.IsZero() {
				a.EndsAt = alert.ResolvedAt
			} else {
				a.EndsAt = alert.ValidUntil
			}
			if externalURL != nil {
				a.GeneratorURL = externalURL.String() + strutil.TableLinkForExpression(expr)
			}
			res = append(res, a)
		}
		if len(alerts) > 0 {
			s.Send(res...)
		}
	}
}

type reloader struct {
	name     string
	reloader func(*config.Config) error
}

// configMetrics establishes reloading metrics similar to Prometheus' built-in ones.
type configMetrics struct {
	lastReloadSuccess       bool
	lastReloadSuccessTime   time.Time
	reloadSuccessMetric     prometheus.Gauge
	reloadSuccessTimeMetric prometheus.Gauge
}

func newConfigMetrics(reg prometheus.Registerer) *configMetrics {
	m := &configMetrics{
		reloadSuccessMetric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rule_evaluator_config_last_reload_successful",
			Help: "Whether the last configuration reload attempt was successful.",
		}),
		reloadSuccessTimeMetric: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "rule_evaluator_config_last_reload_success_timestamp_seconds",
			Help: "Timestamp of the last successful configuration reload.",
		}),
	}
	if reg != nil {
		reg.MustRegister(m.reloadSuccessMetric, m.reloadSuccessTimeMetric)
	}
	return m
}

func (m *configMetrics) setSuccess() {
	m.lastReloadSuccess = true
	m.lastReloadSuccessTime = time.Now()
	m.reloadSuccessMetric.Set(1)
	m.reloadSuccessTimeMetric.SetToCurrentTime()
}

func (m *configMetrics) setFailure() {
	m.lastReloadSuccess = false
	m.reloadSuccessMetric.Set(0)
}

// reloadConfig applies the configuration files.
func reloadConfig(filename string, logger log.Logger, metrics *configMetrics, rls ...reloader) (err error) {
	start := time.Now()
	timings := []interface{}{}
	_ = level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	conf, err := config.LoadFile(filename, false, false, logger)
	if err != nil {
		metrics.setFailure()
		return fmt.Errorf("couldn't load configuration (--config.file=%q): %w", filename, err)
	}

	failed := false
	for _, rl := range rls {
		rstart := time.Now()
		if err := rl.reloader(conf); err != nil {
			_ = level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
		timings = append(timings, rl.name, time.Since(rstart))
	}
	if failed {
		metrics.setFailure()
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	metrics.setSuccess()
	l := []interface{}{"msg", "Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start)}
	_ = level.Info(logger).Log(append(l, timings...)...)
	return nil
}

// convertMetricToLabel converts model.Metric to labels.label.
func convertMetricToLabel(metric model.Metric) labels.Labels {
	ls := make(labels.Labels, 0, len(metric))
	for name, value := range metric {
		l := labels.Label{
			Name:  string(name),
			Value: string(value),
		}
		ls = append(ls, l)
	}
	return ls
}

// convertModelToPromQLValue converts model.Value type to promql type.
func convertModelToPromQLValue(val model.Value) (parser.Value, error) {
	switch results := val.(type) {
	case model.Matrix:
		m := make(promql.Matrix, len(results))
		for i, result := range results {
			pts := make([]promql.FPoint, len(result.Values))
			for j, samplePair := range result.Values {
				pts[j] = promql.FPoint{
					T: int64(samplePair.Timestamp),
					F: float64(samplePair.Value),
				}
			}
			m[i] = promql.Series{
				Metric: convertMetricToLabel(result.Metric),
				Floats: pts,
			}
		}
		return m, nil

	case model.Vector:
		v := make(promql.Vector, len(results))
		for i, result := range results {
			v[i] = promql.Sample{
				T:      int64(result.Timestamp),
				F:      float64(result.Value),
				Metric: convertMetricToLabel(result.Metric),
			}
		}
		return v, nil

	default:
		return nil, fmt.Errorf("Expected Prometheus results of type matrix or vector. Actual results type: %v", val.Type())
	}
}

// Converting v1.Warnings to storage.Warnings.
func convertV1WarningsToStorageWarnings(w v1.Warnings) storage.Warnings {
	warnings := make(storage.Warnings, len(w))
	for i, warning := range w {
		warnings[i] = errors.New(warning)
	}
	return warnings
}

// listSeriesSet implements the storage.SeriesSet interface on top a list of listSeries.
type listSeriesSet struct {
	m        promql.Matrix
	idx      int
	err      error
	warnings storage.Warnings
}

// Next advances the iterator and returns true if there's data to consume.
func (ss *listSeriesSet) Next() bool {
	ss.idx++
	return ss.idx < len(ss.m)
}

// At returns the current series.
func (ss *listSeriesSet) At() storage.Series {
	return promql.NewStorageSeries(ss.m[ss.idx])
}

// Err returns an error encountered while iterating.
func (ss *listSeriesSet) Err() error {
	return ss.err
}

// Warnings returns warnings encountered while iterating.
func (ss *listSeriesSet) Warnings() storage.Warnings {
	return ss.warnings
}

func newListSeriesSet(v promql.Matrix, err error, w v1.Warnings) *listSeriesSet {
	return &listSeriesSet{m: v, idx: -1, err: err, warnings: convertV1WarningsToStorageWarnings(w)}
}

// convertMatchersToPromQL converts []*labels.Matcher to a PromQL query.
func convertMatchersToPromQL(matchers []*labels.Matcher, d int64) (string, []string) {
	metricLabels := make([]string, 0, len(matchers))
	filteredMatchers := make([]string, 0, len(matchers))
	for _, m := range matchers {
		metricLabels = append(metricLabels, m.String())
		filteredMatchers = append(filteredMatchers, m.Name)
	}
	queryExpression := fmt.Sprintf("{%s}[%ds]", strings.Join(metricLabels, ", "), d)
	return queryExpression, filteredMatchers
}

// queryStorage implements storage.Queryable.
type queryStorage struct {
	api v1.API
}

// Querier provides querying access over time series data of a fixed time range.
func (s *queryStorage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	db := &queryAccess{
		api:   s.api,
		ctx:   ctx,
		mint:  mint / 1000, // divide by 1000 to convert milliseconds to seconds.
		maxt:  maxt / 1000,
		query: QueryFunc,
	}
	return db, nil
}

// queryAccess implements storage.Querier.
type queryAccess struct {
	// storage.LabelQuerier satisfies the interface. Calling related methods will result in panic.
	storage.LabelQuerier
	api   v1.API
	mint  int64
	maxt  int64
	ctx   context.Context
	query func(context.Context, string, time.Time, v1.API) (parser.Value, v1.Warnings, error)
}

// Select returns a set of series that matches the given label matchers and time range.
func (db *queryAccess) Select(sort bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if sort || hints != nil {
		return newListSeriesSet(nil, fmt.Errorf("sorting series and select hints are not supported"), nil)
	}

	duration := db.maxt - db.mint
	if duration <= 0 { // not a valid time duration.
		return newListSeriesSet(nil, nil, nil)
	}

	queryExpression, filteredMatchers := convertMatchersToPromQL(matchers, duration)
	maxt := time.Unix(db.maxt, 0)
	v, warnings, err := db.query(db.ctx, queryExpression, maxt, db.api)
	if err != nil {
		return newListSeriesSet(nil, err, warnings)
	}

	m, ok := v.(promql.Matrix)
	if !ok {
		return newListSeriesSet(nil, fmt.Errorf("Error querying Prometheus, Expected type matrix response. Actual type %v", v.Type()), nil)
	}
	// TODO(maxamin) GCM returns label names and values that are not in matchers.
	// Ensure results from query are equivalent to the requested matchers because
	// manager.go checks if returned labels have the same length as matchers.
	// Upstream change to prometheus code may be necessary.
	for i, sample := range m {
		m[i].Metric = sample.Metric.MatchLabels(true, filteredMatchers...)
	}
	return newListSeriesSet(m, err, warnings)
}

func (db *queryAccess) Close() error {
	return nil
}

// makeInstrumentedRoundTripper instruments the original RoundTripper with middleware to observe the request result.
// The new RoundTripper counts the number of query requests sent to GCM and measures the latency of each request.
func makeInstrumentedRoundTripper(transport http.RoundTripper, reg prometheus.Registerer) http.RoundTripper {
	queryCounter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rule_evaluator_query_requests_total",
			Help: "A counter for query requests sent to GCM.",
		},
		[]string{"code", "method"},
	)
	queryHistogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rule_evaluator_query_requests_latency_seconds",
			Help:    "Histogram of response latency of query requests sent to GCM.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code", "method"},
	)
	reg.MustRegister(queryCounter, queryHistogram)

	return promhttp.InstrumentRoundTripperCounter(queryCounter,
		promhttp.InstrumentRoundTripperDuration(queryHistogram, transport))
}

type ruleEvaluator struct {
	queryFunc    rules.QueryFunc
	rulesManager *rules.Manager
}

func newRuleEvaluator(
	ctx context.Context,
	logger log.Logger,
	reg prometheus.Registerer,
	evaluatorOpts *evaluatorOptions,
	version string,
	appendable storage.Appendable,
	notifierManager *notifier.Manager,
	rulesMetrics *rules.Metrics,
) (*ruleEvaluator, error) {
	v1api, err := newAPI(ctx, evaluatorOpts, version, func(rt http.RoundTripper) http.RoundTripper {
		return makeInstrumentedRoundTripper(rt, reg)
	})
	if err != nil {
		return nil, fmt.Errorf("query client: %w", err)
	}
	queryFunc := newQueryFunc(logger, v1api)

	rulesManager := rules.NewManager(&rules.ManagerOptions{
		ExternalURL: evaluatorOpts.GeneratorURL,
		QueryFunc:   queryFunc,
		Context:     ctx,
		Appendable:  appendable,
		Queryable: &queryStorage{
			api: v1api,
		},
		Logger:     logger,
		NotifyFunc: sendAlerts(notifierManager, evaluatorOpts.GeneratorURL),
		Metrics:    rulesMetrics,
	})

	evaluator := ruleEvaluator{
		rulesManager: rulesManager,
		queryFunc:    queryFunc,
	}

	return &evaluator, nil
}

func (e *ruleEvaluator) ApplyConfig(cfg *config.Config) error {
	// Get all rule files matching the configuration paths.
	var files []string
	for _, pat := range cfg.RuleFiles {
		fs, err := filepath.Glob(pat)
		if fs == nil || err != nil {
			return fmt.Errorf("retrieving rule file: %s", pat)
		}
		files = append(files, fs...)
	}
	return e.rulesManager.Update(
		time.Duration(cfg.GlobalConfig.EvaluationInterval),
		files,
		cfg.GlobalConfig.ExternalLabels,
		"",
		nil,
	)
}

func (e *ruleEvaluator) Query(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
	return e.queryFunc(ctx, q, t)
}

func (e *ruleEvaluator) Run() {
	e.rulesManager.Run()
}

func (e *ruleEvaluator) Stop() {
	e.rulesManager.Stop()
}

func newQueryFunc(logger log.Logger, v1api v1.API) rules.QueryFunc {
	return func(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
		v, warnings, err := QueryFunc(ctx, q, t, v1api)
		if len(warnings) > 0 {
			_ = level.Warn(logger).Log("msg", "Querying Prometheus instance returned warnings", "warn", warnings)
		}
		if err != nil {
			return nil, fmt.Errorf("execute query: %w", err)
		}
		vec, ok := v.(promql.Vector)
		if !ok {
			return nil, fmt.Errorf("query Prometheus, Expected type vector response. Actual type %v", v.Type())
		}
		return vec, nil
	}
}
