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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
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
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
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
			level.Error(logger).Log("msg", "Unable to fetch Google Cloud project", "err", err)
		}
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		grpc_prometheus.DefaultClientMetrics,
	)

	// The rule-evaluator version is identical to the export library version for now, so
	// we reuse that constant.
	version, err := export.Version()
	if err != nil {
		level.Error(logger).Log("msg", "Unable to fetch module version", "err", err)
		os.Exit(1)
	}
	newExporter := exportsetup.FromFlags(a, fmt.Sprintf("rule-evaluator/%s", version))

	notifierOptions := notifier.Options{Registerer: reg}

	projectID := a.Flag("query.project-id", "Project ID of the Google Cloud Monitoring scoping project to evaluate rules against.").
		Default(defaultProjectID).String()

	targetURL := a.Flag("query.target-url", fmt.Sprintf("The address of the Prometheus server query endpoint. (%s is replaced with the --query.project-id flag.)", projectIDVar)).
		Default(fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", projectIDVar)).
		String()

	generatorURLStr := a.Flag("query.generator-url", "The base URL used for the generator URL in the alert notification payload. Should point to an instance of a query frontend that accesses the same data as --query.target-url.").
		PlaceHolder("<URL>").String()

	queryCredentialsFile := a.Flag("query.credentials-file", "Credentials file for OAuth2 authentication with --query.target-url.").
		Default("").String()

	listenAddress := a.Flag("web.listen-address", "The address to listen on for HTTP requests.").
		Default(":9091").String()

	configFile := a.Flag("config.file", "Prometheus configuration file path.").
		Default("prometheus.yml").String()

	a.Flag("alertmanager.notification-queue-capacity", "The capacity of the queue for pending Alertmanager notifications.").
		Default("10000").IntVar(&notifierOptions.QueueCapacity)

	extraArgs, err := exportsetup.ExtraArgs()
	if err != nil {
		level.Error(logger).Log("msg", "Error parsing commandline arguments", "err", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}
	if _, err := a.Parse(append(os.Args[1:], extraArgs...)); err != nil {
		level.Error(logger).Log("msg", "Error parsing commandline arguments", "err", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	if *projectID == "" {
		level.Error(logger).Log("msg", "no --query.project-id was specified or could be derived from the environment")
		os.Exit(2)
	}

	*targetURL = strings.ReplaceAll(*targetURL, projectIDVar, *projectID)

	generatorURL := &url.URL{}
	if *generatorURLStr != "" {
		var err error
		generatorURL, err = url.Parse(*generatorURLStr)
		if err != nil {
			level.Error(logger).Log("msg", "Invalid --query.generator-url", "err", err)
			os.Exit(2)
		}
	}

	// Don't expand external labels on config file loading. It's a feature we like but we want to remain
	// compatible with Prometheus and this is still an experimental feature, which we don't support.
	if _, err := config.LoadFile(*configFile, false, false, logger); err != nil {
		level.Error(logger).Log("msg", fmt.Sprintf("Error loading config (--config.file=%s)", *configFile), "err", err)
		os.Exit(2)
	}

	exporter, err := newExporter(logger, reg)
	if err != nil {
		level.Error(logger).Log("msg", "Creating a Cloud Monitoring Exporter failed", "err", err)
		os.Exit(1)
	}
	destination := export.NewStorage(exporter)

	ctxRuleManger := context.Background()
	ctxDiscover, cancelDiscover := context.WithCancel(context.Background())

	opts := []option.ClientOption{
		option.WithScopes("https://www.googleapis.com/auth/monitoring.read"),
		option.WithUserAgent(fmt.Sprintf("rule-evaluator/%s", version)),
	}
	if *queryCredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(*queryCredentialsFile))
	}
	transport, err := apihttp.NewTransport(ctxRuleManger, http.DefaultTransport, opts...)
	if err != nil {
		level.Error(logger).Log("msg", "Creating proxy HTTP transport failed", "err", err)
		os.Exit(1)
	}
	roundTripper := makeInstrumentedRoundTripper(transport, reg)
	client, err := api.NewClient(api.Config{
		Address:      *targetURL,
		RoundTripper: roundTripper,
	})
	if err != nil {
		level.Error(logger).Log("msg", "Error creating client", "err", err)
		os.Exit(1)
	}
	v1api := v1.NewAPI(client)

	queryFunc := func(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
		v, warnings, err := QueryFunc(ctx, q, t, v1api)
		if len(warnings) > 0 {
			level.Warn(logger).Log("msg", "Querying Promethues instance returned warnings", "warn", warnings)
		}
		if err != nil {
			return nil, fmt.Errorf("execute query: %w", err)
		}
		vec, ok := v.(promql.Vector)
		if !ok {
			return nil, fmt.Errorf("Error querying Prometheus, Expected type vector response. Actual type %v", v.Type())
		}
		return vec, nil
	}

	discoveryManager := discovery.NewManager(ctxDiscover, log.With(logger, "component", "discovery manager notify"), discovery.Name("notify"))
	notificationManager := notifier.NewManager(&notifierOptions, log.With(logger, "component", "notifier"))

	externalStorage := &queryStorage{
		api: v1api,
	}

	ruleManager := rules.NewManager(&rules.ManagerOptions{
		ExternalURL: generatorURL,
		QueryFunc:   queryFunc,
		Context:     ctxRuleManger,
		Appendable:  destination,
		Queryable:   externalStorage,
		Logger:      logger,
		NotifyFunc:  sendAlerts(notificationManager, generatorURL.String()),
		Metrics:     rules.NewGroupMetrics(reg),
	})

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
			name: "rules",
			reloader: func(cfg *config.Config) error {
				// Get all rule files matching the configuration paths.
				var files []string
				for _, pat := range cfg.RuleFiles {
					fs, err := filepath.Glob(pat)
					if fs == nil || err != nil {
						return fmt.Errorf("Error retrieving rule file: %s", pat)
					}
					files = append(files, fs...)
				}
				return ruleManager.Update(
					time.Duration(cfg.GlobalConfig.EvaluationInterval),
					files,
					cfg.GlobalConfig.ExternalLabels,
					"",
					nil,
				)
			},
		},
	}
	// Do an initial load of the configuration for all components.
	if err := reloadConfig(*configFile, logger, reloaders...); err != nil {
		level.Error(logger).Log("msg", "error loading config file.", "err", err)
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
					level.Info(logger).Log("msg", "received SIGTERM, exiting gracefully...")
				case <-cancel:
				}
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		// Rule manager.
		g.Add(func() error {
			ruleManager.Run()
			return nil
		}, func(error) {
			ruleManager.Stop()
		})
	}
	{
		// Notifier.
		g.Add(func() error {
			notificationManager.Run(discoveryManager.SyncCh())
			level.Info(logger).Log("msg", "Notification manager stopped")
			return nil
		},
			func(err error) {
				notificationManager.Stop()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManager.Run()
				level.Info(logger).Log("msg", "Discovery manager stopped")
				return err
			},
			func(err error) {
				level.Info(logger).Log("msg", "Stopping Discovery manager...")
				cancelDiscover()
			},
		)
	}
	{
		// Storage Processing.
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			err = destination.Run(ctx)
			level.Info(logger).Log("msg", "Background processing of storage stopped")
			return err
		}, func(error) {
			level.Info(logger).Log("msg", "Stopping background storage processing...")
			cancel()
		})
	}
	reloadCh := make(chan chan error)
	{
		// Web Server.
		server := &http.Server{Addr: *listenAddress}

		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
		http.HandleFunc("/-/reload", func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				rc := make(chan error)
				reloadCh <- rc
				if err := <-rc; err != nil {
					http.Error(w, fmt.Sprintf("Failed to reload config: %s", err), http.StatusInternalServerError)
				}
			} else {
				http.Error(w, "Only POST requests allowed.", http.StatusMethodNotAllowed)
			}
		})
		http.HandleFunc("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		http.HandleFunc("/-/ready", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, "rule-evaluator is Ready.\n")
		})
		g.Add(func() error {
			level.Info(logger).Log("msg", "Starting web server", "listen", *listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			server.Shutdown(ctx)
			cancel()
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
						if err := reloadConfig(*configFile, logger, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-reloadCh:
						if err := reloadConfig(*configFile, logger, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
							rc <- err
						} else {
							rc <- nil
						}
					case <-cancel:
						return nil
					}
				}
			},
			func(err error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}

	// Run a test query to check status of rule evaluator.
	_, err = queryFunc(context.Background(), "vector(1)", time.Now())
	if err != nil {
		level.Error(logger).Log("msg", "Error querying Prometheus instance", "err", err)
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "Running rule evaluator failed", "err", err)
		os.Exit(1)
	}
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
func sendAlerts(s *notifier.Manager, externalURL string) rules.NotifyFunc {
	return func(ctx context.Context, expr string, alerts ...*rules.Alert) {
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
			if externalURL != "" {
				a.GeneratorURL = externalURL + strutil.TableLinkForExpression(expr)
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

// reloadConfig applies the configuration files.
func reloadConfig(filename string, logger log.Logger, rls ...reloader) (err error) {
	start := time.Now()
	timings := []interface{}{}
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	conf, err := config.LoadFile(filename, false, false, logger)
	if err != nil {
		return fmt.Errorf("couldn't load configuration (--config.file=%q): %w", filename, err)
	}

	failed := false
	for _, rl := range rls {
		rstart := time.Now()
		if err := rl.reloader(conf); err != nil {
			level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
		timings = append(timings, rl.name, time.Since(rstart))
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	l := []interface{}{"msg", "Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start)}
	level.Info(logger).Log(append(l, timings...)...)
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
	return promql.NewStorageSeries(promql.Series(ss.m[ss.idx]))
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
