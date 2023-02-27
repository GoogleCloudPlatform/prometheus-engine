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
	stdlog "log"
	"math/rand"
	"net/http"
	httppprof "net/http/pprof"
	"os"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
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
	appVersion         = flag.String("set-version", "v0.1.0", "Injected version to be presented via metrics.")
	lat                = flag.String("latency", "90%500ms,10%200ms", "Encoded latency and probability of the response in format as: <probability>%<duration>,<probability>%<duration>....")
	successProb        = flag.Float64("success-probability", 100, "The probability (in %) of getting a successful response")
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
		version.NewCollector("ping"),
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// 2. Create logger (go-kit/logger).
	logger := logging.NewLogger(*logLevel, *logFormat, "ping", os.Stderr)

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
		tracing.WithServiceName("go-app:ping"),
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

	latDecider, err := newLatencyDecider(*lat)
	if err != nil {
		return err
	}

	m := http.NewServeMux()
	// Create HTTP handler for Prometheus metrics.
	m.Handle("/metrics", mw.WrapHandler("/metrics", promhttp.HandlerFor(
		reg,
		promhttp.HandlerOpts{
			// Opt into OpenMetrics e.g. to support exemplars.
			EnableOpenMetrics: true,
		},
	)))
	// Create HTTP handler for our ping-like implementation.
	m.HandleFunc("/ping", mw.WrapHandler("/ping", pingHandler(logger, latDecider)))

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
	g.Add(run.SignalHandler(context.Background(), syscall.SIGINT, syscall.SIGTERM))
	return g.Run()
}

func pingHandler(logger log.Logger, latDecider *latencyDecider) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		latDecider.AddLatency(r.Context(), logger)

		if err := tracing.DoInSpan(ctx, "evaluatePing", func(ctx context.Context) error {
			var err error
			spanCtx := tracing.GetSpan(ctx)
			pprof.Do(r.Context(), pprof.Labels("trace_id", spanCtx.Context().TraceID()), func(ctx context.Context) {
				err = func() error {
					tracing.GetSpan(ctx).SetAttributes("successProbability", *successProb)
					level.Debug(logger).Log("msg", "evaluating ping", "successProbability", *successProb)

					if rand.Float64()*100 <= *successProb {
						return nil
					}
					return errors.New("decided to NOT return success, sorry")
				}()
			})
			return err
		}); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			// Not smart to pass error straight away. Sanitize on production.
			_, _ = fmt.Fprintln(w, err.Error())
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintln(w, "pong")
	}
}

type latencyDecider struct {
	latencies     []time.Duration
	probabilities []float64 // Sorted ascending.
}

func newLatencyDecider(encodedLatencies string) (*latencyDecider, error) {
	l := latencyDecider{}

	s := strings.Split(encodedLatencies, ",")
	// Be smart, sort while those are encoded, so they are sorted by probability number.
	sort.Strings(s)

	cumulativeProb := 0.0
	for _, e := range s {
		entry := strings.Split(e, "%")
		if len(entry) != 2 {
			return nil, errors.Newf("invalid input %v", encodedLatencies)
		}
		f, err := strconv.ParseFloat(entry[0], 64)
		if err != nil {
			return nil, errors.Wrapf(err, "parse probabilty %v as float", entry[0])
		}
		cumulativeProb += f
		l.probabilities = append(l.probabilities, f)

		d, err := time.ParseDuration(entry[1])
		if err != nil {
			return nil, errors.Wrapf(err, "parse latency %v as duration", entry[1])
		}
		l.latencies = append(l.latencies, d)
	}
	if cumulativeProb != 100 {
		return nil, errors.Newf("overall probability has to equal 100. Parsed input equals to %v", cumulativeProb)
	}
	fmt.Println("Latency decider created:", l)
	return &l, nil
}

func (l latencyDecider) AddLatency(ctx context.Context, logger log.Logger) {
	_, span := tracing.StartSpan(ctx, "addingLatencyBasedOnProbability")
	defer span.End(nil)

	n := rand.Float64() * 100
	span.SetAttributes("latencyProbabilities", l.probabilities, "lucky%", n)

	for i, p := range l.probabilities {
		if n <= p {
			span.SetAttributes("latencyIntroduced", l.latencies[i].String())
			level.Debug(logger).Log(
				"msg", "adding latency based on probability",
				"latencyIntroduced", l.latencies[i].String(),
				"latencyProbabilities", l.probabilities,
				"lucky%", n,
			)
			<-time.After(l.latencies[i])
			return
		}
	}
}
