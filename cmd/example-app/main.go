// Copyright 2021 Google LLC
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
	"encoding/hex"
	"flag"
	"io"
	"log"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	addr             = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	cpuBurnOps       = flag.Int("cpu-burn-ops", 0, "Operatins per second burning CPU. (Used to simulate high CPU utilization. Sensible values: 0-100.)")
	memBallastMBs    = flag.Int("memory-ballast-mbs", 0, "Megabytes of memory ballast to allocate. (Used to simulate high memory utilization.)")
	maxCount         = flag.Int("max-count", labelCombinations, "Maximum metric instance count for all metric types.")
	histogramCount   = flag.Int("histogram-count", 2, "Number of unique instances per histogram metric.")
	gaugeCount       = flag.Int("gauge-count", -1, "Number of unique instances per gauge metric.")
	counterCount     = flag.Int("counter-count", -1, "Number of unique instances per counter metric.")
	summaryCount     = flag.Int("summary-count", -1, "Number of unique instances per summary metric.")
	exemplarSampling = flag.Float64("exemplar-sampling", 0.1, "Fraction of observations to include exemplars on histograms.")
)

var (
	availableLabels = map[string][]string{
		"method": []string{
			"POST",
			"PUT",
			"GET",
		},
		"status": []string{
			"200",
			"300",
			"400",
			"404",
			"500",
		},
		"path": []string{
			"/",
			"/index",
			"/topics",
			"/topics:new",
			"/topics/<id>",
			"/topics/<id>/comment",
			"/topics/<id>/comment:create",
			"/topics/<id>/comment:edit",
			"/imprint",
		},
	}
	labelCombinations = len(availableLabels["method"]) * len(availableLabels["status"]) * len(availableLabels["path"])
)

var (
	metricIncomingRequestsPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "example_incoming_requests_pending",
		},
		[]string{"status", "method", "path"},
	)
	metricOutgoingRequestsPending = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "example_outgoing_requests_pending",
		},
		[]string{"status", "method", "path"},
	)

	metricIncomingRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "example_incoming_requests_total",
		},
		[]string{"status", "method", "path"},
	)
	metricOutgoingRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "example_outgoing_requests_total",
		},
		[]string{"status", "method", "path"},
	)
	metricIncomingRequestErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "example_incoming_request_errors_total",
		},
		[]string{"status", "method", "path"},
	)
	metricOutgoingRequestErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "example_outgoing_request_errors_total1",
		},
		[]string{"status", "method", "path"},
	)

	metricIncomingRequestDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "example_histogram_incoming_request_duration",
			Buckets: prometheus.LinearBuckets(0, 100, 8),
		},
		[]string{"status", "method", "path"},
	)
	metricOutgoingRequestDurationHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "example_histogram_outgoing_request_duration",
			Buckets: prometheus.LinearBuckets(0, 100, 8),
		},
		[]string{"status", "method", "path"},
	)

	metricIncomingRequestDurationSummary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "example_summary_incoming_request_duration",
		},
		[]string{"status", "method", "path"},
	)
	metricOutgoingRequestDurationSummary = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "example_summary_outgoing_request_duration",
		},
		[]string{"status", "method", "path"},
	)
)

func main() {
	flag.Parse()

	metrics := prometheus.NewRegistry()

	metrics.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
		metricIncomingRequestsPending,
		metricOutgoingRequestsPending,
		metricIncomingRequests,
		metricOutgoingRequests,
		metricIncomingRequestErrors,
		metricOutgoingRequestErrors,
		metricIncomingRequestDurationHistogram,
		metricOutgoingRequestDurationHistogram,
		metricIncomingRequestDurationSummary,
		metricOutgoingRequestDurationSummary,
	)

	var memoryBallast []byte
	allocateMemoryBallast(&memoryBallast, *memBallastMBs*1000*1000)

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
					log.Println("Received SIGTERM, exiting gracefully...")
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
		server := &http.Server{Addr: *addr}
		http.Handle("/metrics", promhttp.HandlerFor(metrics, promhttp.HandlerOpts{Registry: metrics, EnableOpenMetrics: true}))

		g.Add(func() error {
			return server.ListenAndServe()
		}, func(err error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			server.Shutdown(ctx)
			cancel()
		})
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(
			func() error {
				return burnCPU(ctx, *cpuBurnOps)
			},
			func(err error) {
				cancel()
			},
		)
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(
			func() error {
				return updateMetrics(ctx)
			},
			func(err error) {
				cancel()
			},
		)
	}
	if err := g.Run(); err != nil {
		log.Println("Exit with error", err)
		os.Exit(1)
	}
}

func allocateMemoryBallast(buf *[]byte, sz int) {
	// Fill memory ballast. Fill it with random values so it results in actual memory usage.
	*buf = make([]byte, sz)
	_, err := io.ReadFull(rand.New(rand.NewSource(0)), *buf)
	if err != nil {
		panic(err)
	}
}

// burnCPU burns the given percentage of CPU of a single core.
func burnCPU(ctx context.Context, ops int) error {
	for {
		// Burn some CPU proportional to the input ops.
		// This must be fixed work, i.e. we cannot spin for a fraction of scheduling will
		// greatly affect how many times we spin, even without high CPU utilization.
		for i := 0; i < ops*20000000; i++ {
		}

		// Wait for some time inversely proportional to the input opts.
		// The constants are picked empirically. Spin and wait time must both depend
		// on the input ops for them to result in linearly scaleing CPU usage.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(100-ops) * 5 * time.Millisecond):
			// default:
		}
	}
}

func newIDs(traceBytes, spanBytes []byte) (string, string) {
	rand.Read(traceBytes)
	rand.Read(spanBytes)
	return hex.EncodeToString(traceBytes), hex.EncodeToString(spanBytes)
}

func updateMetrics(ctx context.Context) error {
	projectID := "example-project"
	traceBytes := make([]byte, 16)
	spanBytes := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
			forNumInstances(*gaugeCount, func(labels prometheus.Labels) {
				metricIncomingRequestsPending.With(labels).Set(float64(rand.Intn(200)))
				metricOutgoingRequestsPending.With(labels).Set(float64(rand.Intn(200)))
			})
			forNumInstances(*counterCount, func(labels prometheus.Labels) {
				metricIncomingRequests.With(labels).Add(float64(rand.Intn(200)))
				metricOutgoingRequests.With(labels).Add(float64(rand.Intn(100)))
				metricIncomingRequestErrors.With(labels).Add(float64(rand.Intn(15)))
				metricOutgoingRequestErrors.With(labels).Add(float64(rand.Intn(5)))
			})
			forNumInstances(*histogramCount, func(labels prometheus.Labels) {
				samp := rand.Uint64()
				thresh := uint64(*exemplarSampling * (1 << 63))
				if samp < thresh {
					traceID, spanID := newIDs(traceBytes, spanBytes)
					exemplar := prometheus.Labels{"trace_id": traceID, "span_id": spanID, "project_id": projectID}
					metricIncomingRequestDurationHistogram.With(labels).(prometheus.ExemplarObserver).ObserveWithExemplar(rand.NormFloat64()*300+500, exemplar)
				} else {
					metricIncomingRequestDurationHistogram.With(labels).Observe(rand.NormFloat64()*300 + 500)
				}
				metricOutgoingRequestDurationHistogram.With(labels).Observe(rand.NormFloat64()*200 + 300)
			})
			forNumInstances(*summaryCount, func(labels prometheus.Labels) {
				metricIncomingRequestDurationSummary.With(labels).Observe(rand.NormFloat64()*300 + 500)
				metricOutgoingRequestDurationSummary.With(labels).Observe(rand.NormFloat64()*200 + 300)
			})
		}
	}
}

func forNumInstances(c int, f func(prometheus.Labels)) {
	if c < 0 {
		c = *maxCount
	}
	for _, path := range availableLabels["path"] {
		for _, status := range availableLabels["status"] {
			for _, method := range availableLabels["method"] {
				if c <= 0 {
					return
				}
				f(prometheus.Labels{
					"path":   path,
					"status": status,
					"method": method,
				})
				c--
			}
		}
	}
}
