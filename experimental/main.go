package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/gpe-collector/pkg/export"
	"github.com/pkg/errors"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
)

// convertValueToVector converts model.Value type to promql.Vector type
func convertValueToVector(val model.Value) (promql.Vector, error) {
	results, ok := val.(model.Vector)
	if !ok {
		return nil, errors.Errorf("Expected Prometheus results of type vector. Actual results type: %v\n", results.Type())
	}
	v := make(promql.Vector, len(results))
	for i, result := range results {
		ls := make(labels.Labels, 0, len(result.Metric))
		for name, value := range result.Metric {
			l := labels.Label{
				Name:  string(name),
				Value: string(value),
			}
			ls = append(ls, l)
		}
		s := promql.Sample{
			Point: promql.Point{
				T: int64(result.Timestamp),
				V: float64(result.Value),
			},
			Metric: ls,
		}
		v[i] = s
	}
	return v, nil
}

// QueryFunc queries a Prometheus instance and returns a promql.Vector
func QueryFunc(ctx context.Context, targetURL, q string, t time.Time) (promql.Vector, error) {
	client, err := api.NewClient(api.Config{
		Address: targetURL,
	})
	if err != nil {
		return nil, errors.Errorf("Error creating client: %v\n", err)
	}
	v1api := v1.NewAPI(client)
	results, warnings, err := v1api.Query(ctx, q, time.Now())
	if err != nil {
		return nil, errors.Errorf("Error querying Prometheus: %v\n", err)
	}
	if len(warnings) > 0 { //TODO(maxamin): use logger rather than Printf
		fmt.Printf("Warnings: %v\n", warnings)
	}
	return convertValueToVector(results)
}

// NotifyFunc sends alerts to GCM
func NotifyFunc(ctx context.Context, expr string, alerts ...*rules.Alert) {
	// send alerts
}

func main() {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "ts", log.DefaultTimestampUTC)
	logger = log.With(logger, "caller", log.DefaultCaller)

	a := kingpin.New("rule", "The Prometheus Rule Evaluator")
	exporterOptions := export.NewFlagOptions(a)

	targetURL := a.Flag("target-url", "The address of the Prometheus server query endpoint.").Required().String()
	ruleFiles := a.Flag("rule-file", "The Prometheus rule file containing the necessary rule statements.").Required().Strings()
	listenAddress := a.Flag("listen-address", "The address to listen on for HTTP requests.").Default(":9091").String()

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	destination, err := export.NewStorage(logger, nil, *exporterOptions)
	if err != nil {
		level.Error(logger).Log("msg", "Creating a Cloud Monitoring Exporter failed", "err", err)
		os.Exit(1)
	}

	noopQueryable := func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return storage.NoopQuerier(), nil
	}

	queryFunc := func(ctx context.Context, q string, t time.Time) (promql.Vector, error) {
		return QueryFunc(ctx, *targetURL, q, t)
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)

	managerOptions := &rules.ManagerOptions{
		ExternalURL: &url.URL{},
		QueryFunc:   queryFunc,
		Context:     context.Background(),
		Appendable:  destination,
		Queryable:   storage.QueryableFunc(noopQueryable),
		Logger:      logger,
		NotifyFunc:  NotifyFunc,
		Metrics:     rules.NewGroupMetrics(reg),
	}

	manager := rules.NewManager(managerOptions)
	err = manager.Update(time.Second*20, *ruleFiles, nil)
	if err != nil {
		level.Error(logger).Log("msg", "Updating rule manager failed", "err", err)
		os.Exit(1)
	}

	var g run.Group
	{
		g.Add(func() error {
			manager.Run()
			return nil
		}, func(error) {
			manager.Stop()
		})
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			return destination.Run(ctx)
		}, func(error) {
			level.Info(logger).Log("msg", "Background processing of storage stopped")
			cancel()
		})
	}
	{
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
		server := &http.Server{Addr: *listenAddress}
		http.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))

		g.Add(func() error {
			level.Info(logger).Log("msg", "Starting web server for metrics", "listen", listenAddress)
			return server.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			server.Shutdown(ctx)
			cancel()
		})
	}

	if err := g.Run(); err != nil {
		level.Error(logger).Log("msg", "Running rule evaluator failed", "err", err)
		os.Exit(1)
	}
}
