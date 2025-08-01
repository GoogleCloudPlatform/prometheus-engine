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
	"crypto/fips140"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"cloud.google.com/go/compute/metadata"
	"github.com/GoogleCloudPlatform/prometheus-engine/cmd/rule-evaluator/internal"
	"github.com/GoogleCloudPlatform/prometheus-engine/internal/promapi"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/oklog/run"
	versioninfo "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/google/export"
	exportsetup "github.com/prometheus/prometheus/google/export/setup"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
	"google.golang.org/api/option"
	apihttp "google.golang.org/api/transport/http"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"gopkg.in/yaml.v3"

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
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/strutil"
)

const projectIDVar = "PROJECT_ID"

var (
	googleCloudBaseURL = url.URL{
		Scheme: "https",
		Host:   "console.cloud.google.com",
		Path:   "/monitoring/metrics-explorer",
	}

	queryCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "rule_evaluator_query_requests_total",
			Help: "A counter for query requests sent to GCM.",
		},
		[]string{"code", "method"},
	)
	queryHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "rule_evaluator_query_requests_latency_seconds",
			Help:    "Histogram of response latency of query requests sent to GCM.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"code", "method"},
	)
)

func main() {
	ctx := context.Background()

	logger := log.NewJSONLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	if !fips140.Enabled() {
		_ = logger.Log("msg", "FIPS mode not enabled")
		os.Exit(1)
	}

	a := kingpin.New("rule", "The Prometheus Rule Evaluator")
	logLevel := a.Flag("log.level",
		"The level of logging. Can be one of 'debug', 'info', 'warn', 'error'").Default(
		"info").Enum("debug", "info", "warn", "error")

	a.HelpFlag.Short('h')

	var defaultProjectID string
	if metadata.OnGCE() {
		var err error
		defaultProjectID, err = metadata.ProjectIDWithContext(ctx)
		if err != nil {
			_ = level.Warn(logger).Log("msg", "Unable to detect Google Cloud project", "err", err)
		}
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		versioninfo.NewCollector("rule-evaluator"), // Add build_info metric.
		grpc_prometheus.DefaultClientMetrics,
		queryCounter,
		queryHistogram,
	)

	sdMetrics, err := discovery.CreateAndRegisterSDMetrics(reg)
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	opts := exportsetup.Opts{
		ExporterOpts: export.ExporterOpts{
			UserAgentProduct: fmt.Sprintf("rule-evaluator/%s", version.Version),
		},
	}
	opts.SetupFlags(a)

	defaultEvaluatorOpts := evaluatorOptions{
		TargetURL:     Must(url.Parse(fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", projectIDVar))),
		ProjectID:     defaultProjectID,
		DisableAuth:   false,
		ListenAddress: ":9091",
		ConfigFile:    "prometheus.yml",
		QueueCapacity: 10000,
	}
	defaultEvaluatorOpts.setupFlags(a)

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
	switch strings.ToLower(*logLevel) {
	case "debug":
		logger = level.NewFilter(logger, level.AllowDebug())
	case "warn":
		logger = level.NewFilter(logger, level.AllowWarn())
	case "error":
		logger = level.NewFilter(logger, level.AllowError())
	default:
		logger = level.NewFilter(logger, level.AllowInfo())
	}

	if err := defaultEvaluatorOpts.validate(); err != nil {
		_ = level.Error(logger).Log("msg", "invalid command line argument", "err", err)
		os.Exit(1)
	}

	startTime := time.Now()

	ctxExporter, cancelExporter := context.WithCancel(ctx)
	exporter, err := opts.NewExporter(ctxExporter, logger, reg)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Creating a Cloud Monitoring Exporter failed", "err", err)
		os.Exit(1)
	}
	destination := export.NewStorage(exporter)

	ctxDiscover, cancelDiscover := context.WithCancel(ctx)
	discoveryManager := discovery.NewManager(ctxDiscover, log.With(logger, "component", "discovery manager notify"), reg, sdMetrics, discovery.Name("notify"))
	notifierOptions := notifier.Options{
		Registerer:    reg,
		QueueCapacity: defaultEvaluatorOpts.QueueCapacity,
	}
	notificationManager := notifier.NewManager(&notifierOptions, log.With(logger, "component", "notifier"))
	rulesMetrics := rules.NewGroupMetrics(reg)
	ruleEvaluator, err := newRuleEvaluator(ctx, logger, &defaultEvaluatorOpts, version.Version, destination, notificationManager, rulesMetrics)
	if err != nil {
		_ = level.Error(logger).Log("msg", "Create rule-evaluator", "err", err)
		os.Exit(1)
	}

	reloaders := []reloader{
		{
			name: "notify",
			reloader: func(cfg *operator.RuleEvaluatorConfig) error {
				return notificationManager.ApplyConfig(&cfg.Config)
			},
		}, {
			name: "exporter",
			reloader: func(cfg *operator.RuleEvaluatorConfig) error {
				return destination.ApplyConfig(&cfg.Config)
			},
		}, {
			name: "notify_sd",
			reloader: func(cfg *operator.RuleEvaluatorConfig) error {
				c := make(map[string]discovery.Configs)
				for k, v := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
					c[k] = v.ServiceDiscoveryConfigs
				}
				return discoveryManager.ApplyConfig(c)
			},
		}, {
			name: "rules",
			reloader: func(cfg *operator.RuleEvaluatorConfig) error {
				// Don't modify defaults. Copy defaults and modify based on config.
				evaluatorOpts := defaultEvaluatorOpts
				if cfg.GoogleCloudQuery.CredentialsFile != "" {
					evaluatorOpts.CredentialsFile = cfg.GoogleCloudQuery.CredentialsFile
				}
				if cfg.GoogleCloudQuery.GeneratorURL != "" {
					generatorURL, err := url.Parse(cfg.GoogleCloudQuery.GeneratorURL)
					if err != nil {
						return fmt.Errorf("unable to parse Google Cloud generator URL: %w", err)
					}
					evaluatorOpts.GeneratorURL = generatorURL
				}
				if cfg.GoogleCloudQuery.ProjectID != "" {
					evaluatorOpts.ProjectID = cfg.GoogleCloudQuery.ProjectID
				}
				return ruleEvaluator.ApplyConfig(&cfg.Config, &evaluatorOpts)
			},
		},
	}

	configMetrics := newConfigMetrics(reg)

	// Do an initial load of the configuration for all components.
	if err := reloadConfig(defaultEvaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
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
		g.Add(func() error {
			err = destination.Run()
			_ = level.Info(logger).Log("msg", "Background processing of storage stopped")
			return err
		}, func(error) {
			_ = level.Info(logger).Log("msg", "Stopping background storage processing...")
			cancelExporter()
		})
	}
	cwd, err := os.Getwd()
	reloadCh := make(chan chan error)
	{
		// Web Server.
		server := &http.Server{Addr: defaultEvaluatorOpts.ListenAddress}

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

		// https://prometheus.io/docs/prometheus/latest/querying/api/#build-information
		buildInfoHandler := promapi.BuildinfoHandlerFunc(log.With(logger, "handler", "buildinfo"), "rule-evaluator", version.Version)
		http.HandleFunc("/api/v1/status/buildinfo", buildInfoHandler)

		// https://prometheus.io/docs/prometheus/latest/querying/api/#rules
		apiHandler := internal.NewAPI(logger, ruleEvaluator.rulesManager)
		http.HandleFunc("/api/v1/rules", apiHandler.HandleRulesEndpoint)
		http.HandleFunc("/api/v1/rules/", http.NotFound)

		// https://prometheus.io/docs/prometheus/latest/querying/api/#alerts
		http.HandleFunc("/api/v1/alerts", apiHandler.HandleAlertsEndpoint)

		g.Add(func() error {
			_ = level.Info(logger).Log("msg", "Starting web server", "listen", defaultEvaluatorOpts.ListenAddress)
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
						if err := reloadConfig(defaultEvaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
							_ = level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-reloadCh:
						if err := reloadConfig(defaultEvaluatorOpts.ConfigFile, logger, configMetrics, reloaders...); err != nil {
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
		Default(googleCloudBaseURL.String()).
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
	contents, err := os.ReadFile(opts.ConfigFile)
	if err != nil {
		return fmt.Errorf("read config %q: %w", opts.ConfigFile, err)
	}

	cfg, err := loadConfig(contents)
	if err != nil {
		return fmt.Errorf("load config %q: %w", opts.ConfigFile, err)
	}

	if opts.ProjectID == "" && cfg.GoogleCloudQuery.ProjectID != "" {
		opts.ProjectID = cfg.GoogleCloudQuery.ProjectID
	}

	// Pass a placeholder project ID value "x" to ensure the URL replacement is valid.
	if _, err := url.Parse(strings.ReplaceAll(opts.TargetURL.String(), projectIDVar, "x")); err != nil {
		return fmt.Errorf("unable to parse --query.target-url value %q: %w", opts.TargetURL.String(), err)
	}

	return nil
}

func newAPI(ctx context.Context, opts *evaluatorOptions, version string) (v1.API, error) {
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
	roundTripper := promhttp.InstrumentRoundTripperCounter(queryCounter,
		promhttp.InstrumentRoundTripperDuration(queryHistogram, transport))
	client, err := api.NewClient(api.Config{
		Address:      strings.ReplaceAll(opts.TargetURL.String(), projectIDVar, opts.ProjectID),
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
		return nil, warnings, fmt.Errorf("error querying Prometheus: %w", err)
	}
	v, err := convertModelToPromQLValue(results)
	return v, warnings, err
}

// sendAlerts returns the rules.NotifyFunc for a Notifier.
func sendAlerts(s *notifier.Manager, projectID string, generatorURL *url.URL) rules.NotifyFunc {
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
			if generatorURL != nil {
				if generatorURL.String() == googleCloudBaseURL.String() {
					// Project ID is empty when the rule-evaluator is instantiated, before config-reloader runs.
					if projectID != "" {
						// If it's a GCM link (default), create the full URL for the alert.
						a.GeneratorURL = googleCloudLink(projectID, expr, alert.FiredAt, alert.FiredAt.Add(-time.Hour)).String()
					}
				} else {
					// Otherwise, if it was specified we assume it points to a Prometheus frontend.
					a.GeneratorURL = generatorURL.String() + strutil.TableLinkForExpression(expr)
				}
			}
			res = append(res, a)
		}
		if len(alerts) > 0 {
			s.Send(res...)
		}
	}
}

// googleCloudLink returns the link to the Google Cloud project, optionally with a pre-populated
// query. The passed project must be a valid project.
func googleCloudLink(projectID, expr string, endTime, startTime time.Time) *url.URL {
	// Clone URL to avoid mutating the original.
	url := googleCloudBaseURL
	// Note: The URL API was reverse-engineered.
	if !endTime.IsZero() {
		url.Path += ";endTime=" + endTime.Format(time.RFC3339)
	}
	if !startTime.IsZero() {
		url.Path += ";startTime=" + startTime.Format(time.RFC3339)
	}

	values := url.Query()
	values.Set("project", projectID)

	if expr != "" {
		// These settings reflect the default on metrics explorer for majority of use-cases.
		pageState := map[string]any{
			"xyChart": map[string]any{
				"dataSets": []map[string]any{
					{
						"prometheusQuery": expr,
					},
				},
			},
		}
		// Note, this also escapes the JSON.
		pageStateValue, err := json.Marshal(pageState)
		if err != nil {
			panic(err)
		}
		values.Set("pageState", string(pageStateValue))
	}

	// Escapes the query (which may have escaped JSON).
	url.RawQuery = values.Encode()
	return &url
}

type reloader struct {
	name     string
	reloader func(*operator.RuleEvaluatorConfig) error
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

	content, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("read configuration (--config.file=%q): %w", filename, err)
	}
	conf, err := loadConfig(content)
	if err != nil {
		metrics.setFailure()
		return fmt.Errorf("load configuration (--config.file=%q): %w", filename, err)
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

func loadConfig(content []byte) (*operator.RuleEvaluatorConfig, error) {
	conf := &operator.RuleEvaluatorConfig{
		Config: config.DefaultConfig,
	}
	// Don't expand external labels on config file loading. It's a feature we like but we
	// want to remain compatible with Prometheus and this is still an experimental feature,
	// which we don't support. See the Prometheus' config.LoadFile method.
	if err := yaml.Unmarshal(content, conf); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return conf, nil
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
		return nil, fmt.Errorf("expected Prometheus results of type matrix or vector. actual results type: %v", val.Type())
	}
}

// Converting v1.Warnings to annotations.Annotations.
func convertV1WarningsToAnnotations(w v1.Warnings) annotations.Annotations {
	a := annotations.New()
	for _, warning := range w {
		a.Add(errors.New(warning))
	}
	return *a
}

// listSeriesSet implements the storage.SeriesSet interface on top a list of listSeries.
type listSeriesSet struct {
	m        promql.Matrix
	idx      int
	err      error
	warnings annotations.Annotations
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
func (ss *listSeriesSet) Warnings() annotations.Annotations {
	return ss.warnings
}

func newListSeriesSet(v promql.Matrix, err error, w v1.Warnings) *listSeriesSet {
	return &listSeriesSet{m: v, idx: -1, err: err, warnings: convertV1WarningsToAnnotations(w)}
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
func (s *queryStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	db := &queryAccess{
		api:   s.api,
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
	query func(context.Context, string, time.Time, v1.API) (parser.Value, v1.Warnings, error)
}

// Select returns a set of series that matches the given label matchers and time range.
func (db *queryAccess) Select(ctx context.Context, sort bool, hints *storage.SelectHints, matchers ...*labels.Matcher) storage.SeriesSet {
	if sort || hints != nil {
		return newListSeriesSet(nil, errors.New("sorting series and select hints are not supported"), nil)
	}

	duration := db.maxt - db.mint
	if duration <= 0 { // not a valid time duration.
		return newListSeriesSet(nil, nil, nil)
	}

	queryExpression, filteredMatchers := convertMatchersToPromQL(matchers, duration)
	maxt := time.Unix(db.maxt, 0)
	v, warnings, err := db.query(ctx, queryExpression, maxt, db.api)
	if err != nil {
		return newListSeriesSet(nil, err, warnings)
	}

	m, ok := v.(promql.Matrix)
	if !ok {
		return newListSeriesSet(nil, fmt.Errorf("error querying Prometheus, expected type matrix response. actual type %v", v.Type()), nil)
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

type ruleEvaluator struct {
	ctx             context.Context
	logger          log.Logger
	version         string
	appendable      storage.Appendable
	notifierManager *notifier.Manager
	rulesMetrics    *rules.Metrics

	queryFunc         rules.QueryFunc
	rulesManager      *rules.Manager
	lastEvaluatorOpts *evaluatorOptions
	mtx               sync.Mutex
}

// Returns the URL that points to the rule-evaluator instance (set by the user). By default, or if
// using a Google Cloud base URL, this returns a link to the Google Cloud project page.
func getExternalURL(generatorURL *url.URL, projectID string) *url.URL {
	// Project ID is empty when the rule-evaluator is instantiated, before config-reloader runs.
	if generatorURL == nil || projectID == "" {
		return nil
	}

	if generatorURL.String() == googleCloudBaseURL.String() {
		// If it's a GCM link (default), create the full URL for the alert.
		return googleCloudLink(projectID, "", time.Time{}, time.Time{})
	}
	return generatorURL
}

func newRuleEvaluator(
	ctx context.Context,
	logger log.Logger,
	evaluatorOpts *evaluatorOptions,
	version string,
	appendable storage.Appendable,
	notifierManager *notifier.Manager,
	rulesMetrics *rules.Metrics,
) (*ruleEvaluator, error) {
	v1api, err := newAPI(ctx, evaluatorOpts, version)
	if err != nil {
		return nil, fmt.Errorf("query client: %w", err)
	}
	queryFunc := newQueryFunc(logger, v1api)

	rulesManager := rules.NewManager(&rules.ManagerOptions{
		ExternalURL: getExternalURL(evaluatorOpts.GeneratorURL, evaluatorOpts.ProjectID),
		QueryFunc:   queryFunc,
		Context:     ctx,
		Appendable:  appendable,
		Queryable: &queryStorage{
			api: v1api,
		},
		Logger:     logger,
		NotifyFunc: sendAlerts(notifierManager, evaluatorOpts.ProjectID, evaluatorOpts.GeneratorURL),
		Metrics:    rulesMetrics,
	})

	evaluator := ruleEvaluator{
		ctx:             ctx,
		logger:          logger,
		version:         version,
		appendable:      appendable,
		notifierManager: notifierManager,
		rulesMetrics:    rulesMetrics,

		rulesManager:      rulesManager,
		queryFunc:         queryFunc,
		lastEvaluatorOpts: evaluatorOpts,
	}

	return &evaluator, nil
}

func (e *ruleEvaluator) ApplyConfig(cfg *config.Config, evaluatorOpts *evaluatorOptions) error {
	if evaluatorOpts != nil && !reflect.DeepEqual(evaluatorOpts, e.lastEvaluatorOpts) {
		e.lastEvaluatorOpts = evaluatorOpts

		v1api, err := newAPI(e.ctx, evaluatorOpts, e.version)
		if err != nil {
			return fmt.Errorf("query client: %w", err)
		}
		queryFunc := newQueryFunc(e.logger, v1api)

		rulesManager := rules.NewManager(&rules.ManagerOptions{
			ExternalURL: getExternalURL(evaluatorOpts.GeneratorURL, evaluatorOpts.ProjectID),
			QueryFunc:   queryFunc,
			Context:     e.ctx,
			Appendable:  e.appendable,
			Queryable: &queryStorage{
				api: v1api,
			},
			Logger:     e.logger,
			NotifyFunc: sendAlerts(e.notifierManager, evaluatorOpts.ProjectID, evaluatorOpts.GeneratorURL),
			Metrics:    e.rulesMetrics,
		})

		// Set new rule-manager and flag before stopping, so we can rerun with the new one.
		e.mtx.Lock()
		oldRuleManager := e.rulesManager
		e.rulesManager = rulesManager
		oldRuleManager.Stop()
		e.queryFunc = queryFunc
		e.mtx.Unlock()

		_, err = queryFunc(e.ctx, "vector(1)", time.Now())
		if err != nil {
			_ = level.Error(e.logger).Log("msg", "Error querying Prometheus instance", "err", err)
		}
	}

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
	// Copy the function in case it changes, but don't block until it completes.
	e.mtx.Lock()
	queryFunc := e.queryFunc
	e.mtx.Unlock()
	return queryFunc(ctx, q, t)
}

func (e *ruleEvaluator) Run() {
	for {
		// Copy the rule-manager before running, so we don't hold the lock.
		e.mtx.Lock()
		curr := e.rulesManager
		e.mtx.Unlock()

		// A nil indicates shutdown, otherwise it's a config update requiring restart.
		if curr == nil {
			break
		}
		curr.Run()
	}
}

func (e *ruleEvaluator) Stop() {
	e.mtx.Lock()
	defer e.mtx.Unlock()
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
