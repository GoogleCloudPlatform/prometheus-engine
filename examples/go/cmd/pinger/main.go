// Copyright 2023 Google LLC
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
	"flag"
	"fmt"
	"io"
	stdlog "log"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/examples/go/pkg/instrumentationhttp"
	"github.com/GoogleCloudPlatform/prometheus-engine/examples/go/pkg/logging"
	"github.com/bwplotka/tracing-go/tracing"
	"github.com/bwplotka/tracing-go/tracing/exporters/otlp"
	"github.com/efficientgo/core/errcapture"
	"github.com/efficientgo/core/errors"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
)

var (
	addr               = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	endpoint           = flag.String("endpoint", "http://observable-ping.default.svc.cluster.local:8080/ping", "The address of pong app we can connect to and send requests.")
	appVersion         = flag.String("set-version", "v0.2.0", "Injected version to be presented via metrics.")
	pingsPerSec        = flag.Int("pings-per-second", 10, "How many pings per second we should request")
	traceEndpoint      = flag.String("trace-endpoint", "", "Optional GRPC OTLP endpoint for tracing backend. Set it to 'stdout' to print traces to the output instead.")
	traceSamplingRatio = flag.Float64("trace-sampling-ratio", 1.0, "Sampling ratio. Currently 1.0 is the best value to use with exemplars.")
	logLevel           = flag.String("log-level", "info", "Log filtering level. Possible values: \"error\", \"warn\", \"info\", \"debug\"")
	logFormat          = flag.String("log-format", logging.LogFormatLogfmt, fmt.Sprintf("Log format to use. Possible options: %s or %s", logging.LogFormatLogfmt, logging.LogFormatJSON))
)

func main() {
	flag.Parse()
	if err := runMain(); err != nil {
		// Use %+v for github.com/efficientgo/core/errors error to print with stack.
		stdlog.Fatalf("Error: %+v", errors.Wrapf(err, "%s", flag.Arg(0)))
	}
}

func runMain() (err error) {
	version.Version = *appVersion

	// 1. Create registry for Prometheus metrics (prometheus/client_golang).
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		version.NewCollector("pinger"),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// 2. Create logger (go-kit/logger).
	logger := logging.NewLogger(*logLevel, *logFormat, "pinger", os.Stderr)

	// 3. Create tracer (bwplotka/tracing, a simple wrapper of go.opentelemetry.io/otel modules).
	var exporter tracing.ExporterBuilder
	switch *traceEndpoint {
	case "stdout":
		exporter = tracing.NewWriterExporter(os.Stdout)
	default:
		exporter = otlp.Exporter(*traceEndpoint, otlp.WithInsecure())
	}
	tracer, closeFn, err := tracing.NewTracer(
		exporter,
		tracing.WithSampler(tracing.TraceIDRatioBasedSampler(*traceSamplingRatio)),
		tracing.WithServiceName("go-app:pinger"),
	)
	if err != nil {
		return err
	}
	defer errcapture.Do(&err, closeFn, "close tracers")

	level.Info(logger).Log(
		"msg", "metrics, logs and tracing enabled",
		"metricAddress", *addr,
		"logOutput", "stderr",
		"traceTargetEndpoint", *traceEndpoint,
	)

	// Create middleware that will instrument our HTTP server with logs, tracing and metrics (with exemplars).
	mw := instrumentationhttp.NewMiddleware(reg, nil, logger, tracer)

	m := http.NewServeMux()
	// Create HTTP handler for Prometheus metrics.
	m.Handle("/metrics", mw.WrapHandler("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics e.g. to support exemplars.
			EnableOpenMetrics: true,
		},
	)))

	// Debug profiling endpoints.
	m.HandleFunc("/debug/pprof/", httppprof.Index)
	m.HandleFunc("/debug/pprof/cmdline", httppprof.Cmdline)
	m.HandleFunc("/debug/pprof/profile", httppprof.Profile)
	m.HandleFunc("/debug/pprof/symbol", httppprof.Symbol)

	srv := http.Server{Addr: *addr, Handler: m}

	g := &run.Group{}
	g.Add(func() error {
		level.Info(logger).Log("msg", "starting HTTP server", "addr", *addr)
		if err := srv.ListenAndServe(); err != nil {
			return errors.Wrap(err, "starting web server")
		}
		return nil
	}, func(error) {
		if err := srv.Close(); err != nil {
			level.Error(logger).Log("msg", "failed to stop web server", "err", err)
		}
	})
	{
		client := &http.Client{
			// TODO(bwplotka): Add tripperware that will instrument HTTP client with logs, metrics and traces.
		}

		ctx, cancel := context.WithCancel(context.Background())
		g.Add(func() error {
			spamPings(ctx, client, logger, *endpoint, *pingsPerSec)
			return nil
		}, func(error) {
			cancel()
		})
	}
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))
	return g.Run()
}

func spamPings(ctx context.Context, client *http.Client, logger log.Logger, endpoint string, pingsPerSec int) {
	var wg sync.WaitGroup
	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			return
		case <-time.After(1 * time.Second):
		}

		for i := 0; i < pingsPerSec; i++ {
			wg.Add(1)
			go ping(ctx, client, logger, endpoint, &wg)
		}
	}
}

func ping(ctx context.Context, client *http.Client, logger log.Logger, endpoint string, wg *sync.WaitGroup) {
	defer wg.Done()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	r, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create request", "err", err)
		return
	}
	res, err := client.Do(r)
	if err != nil {
		level.Error(logger).Log("msg", "failed to send request", "err", err)
		return
	}
	if res.StatusCode != http.StatusOK {
		level.Error(logger).Log("msg", "got non 200 response", "code", res.StatusCode)
	}
	if res.Body != nil {
		// We don't care about response, just release resources.
		_, _ = io.Copy(io.Discard, res.Body)
		_ = res.Body.Close()
	}
}
