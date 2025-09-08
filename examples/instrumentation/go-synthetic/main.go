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
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	mathrand "math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
)

var (
	addr                 = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
	cpuBurnOps           = flag.Int("cpu-burn-ops", 0, "Operations per second burning CPU. (Used to simulate high CPU utilization. Sensible values: 0-100.)")
	memBallastMBs        = flag.Int("memory-ballast-mbs", 0, "Megabytes of memory ballast to allocate. (Used to simulate high memory utilization.)")
	histogramCount       = flag.Int("histogram-count", 2, "Number of unique instances per histogram metric.")
	nativeHistogramCount = flag.Int("native-histogram-count", -1, "Number of unique instances per native-histogram metric."+
		"Note that native histograms are not supported in text format exposition, so traditional protobuf format has to be enabled on your collector. See https://prometheus.io/docs/prometheus/latest/feature_flags/#native-histograms")
	gaugeCount   = flag.Int("gauge-count", -1, "Number of unique instances per gauge metric.")
	counterCount = flag.Int("counter-count", -1, "Number of unique instances per counter metric.")
	summaryCount = flag.Int("summary-count", -1, "Number of unique instances per summary metric.")

	// TODO(bwplotka): Implement for testing one day.
	//nolint:unused
	omStateSetCount = flag.Int("om-stateset-count", -1, "Number of OpenMetrics StateSet metrics (https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#stateset). Requires OpenMetrics format to be negotiated.")
	//nolint:unused
	omInfoCount = flag.Int("om-info-count", -1, "Number of OpenMetrics Info metrics (https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#stateset). Requires OpenMetrics format to be negotiated.")
	//nolint:unused
	omGaugeHistogramCount = flag.Int("om-gaugehistogram-count", -1, "Number of OpenMetrics GaugeHistogram metrics (https://github.com/prometheus/OpenMetrics/blob/v1.0.0/specification/OpenMetrics.md#stateset). Requires OpenMetrics format to be negotiated.")

	exemplarSampling = flag.Float64("exemplar-sampling", 0.1, "Fraction of observations to include exemplars on histograms.")

	metricNamingMode = flag.String("metric-naming-style", PrometheusStyle, `Change the default metric names to test UTF-8 extended charset features. This option will affect all "example_*" metric names produced by this application. For example:
- 'prometheus' style will keep the old name 'example_incoming_requests_pending'
- 'gcm-extended' style will add /, . and - , so 'example/incoming.requests-pending'
- 'exotic-utf-8' style will add (forbidden in GCM) exotic chars like '😂', so 'example/🗻😂/incoming.requests-pending'`)
	statusLabelNamingMode = flag.String("status-label-naming-style", PrometheusStyle, `Change the default label name for example metrics with 'status' label to test UTF-8 extended charset features. For example:
- 'prometheus' style will keep the old label name 'status'
- 'gcm-label-extended' style will add / and . , so 'example/http.request.status'
- 'exotic-utf-8' style will add (forbidden in GCM) exotic chars like '😂', so 'example/🗻😂/http.request-status'`)
)

type namingStyle = string

const (
	// PrometheusStyle represents Prometheus OSS strict charset (https://prometheus.io/docs/concepts/data_model/#metric-names-and-labels).
	PrometheusStyle namingStyle = "prometheus"
	// GCMExtendedCharsStyle represents GCM charset, so PrometheusStyle and "/.-" characters.
	GCMExtendedCharsStyle namingStyle = "gcm-extended" // (/[a-zA-Z0-9$-_.+!*'(),%:]+)+ for metric names and Prometheus + "/." chars for labels.
	// ExoticUTF8 represents any UTF-8 characters.
	// NOTE: Using those will cause GCM ingestion to block such metrics.
	ExoticUTF8 namingStyle = "exotic-utf-8"
)

func adjustExampleMetricName(name string, style namingStyle) string {
	if !strings.HasPrefix(name, "example_") {
		panic(fmt.Sprintf("expected example_ prefixed metric, got %v", name))
	}
	switch style {
	case PrometheusStyle:
		return name
	case GCMExtendedCharsStyle, ExoticUTF8:
		name = strings.Replace(name, "_", "/", 1)
		name = strings.Replace(name, "_", ".", 1)
		// This likely kills the suffix (_total > -total).
		// Do it on purpose for a stress test (Otel metrics would skip suffix).
		name = strings.ReplaceAll(name, "_", "-")
		if style == GCMExtendedCharsStyle {
			return name
		}
		return strings.Replace(name, "example/", "example/🗻😂/", 1)
	default:
		panic(fmt.Sprintf("unsupported %v style", style))
	}
}

func getStatusLabelName(style namingStyle) string {
	switch style {
	case PrometheusStyle:
		return "string"
	case GCMExtendedCharsStyle:
		return "example/http.request.status"
	case ExoticUTF8:
		return "example/🗻😂/http.request-status"
	default:
		panic(fmt.Sprintf("unsupported %v style", style))
	}
}

// availableLabels represents human-readable labels. This gives us max
// of len(availableLabels["method"]) * len(availableLabels["status"]) * len(availableLabels["path"])
// combinations of those. If the specified metric count surpasses that, artificial GET, 200, /yolo/<number> will be generated.
var availableLabels = map[string][]string{
	"method": {
		"POST",
		"PUT",
		"GET",
	},
	"status": {
		"200",
		"300",
		"400",
		"404",
		"500",
	},
	"path": {
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

type metrics struct {
	metricIncomingRequestsPending                *prometheus.GaugeVec
	metricOutgoingRequestsPending                *prometheus.GaugeVec
	metricIncomingRequests                       *prometheus.CounterVec
	metricOutgoingRequests                       *prometheus.CounterVec
	metricIncomingRequestErrors                  *prometheus.CounterVec
	metricOutgoingRequestErrors                  *prometheus.CounterVec
	metricIncomingRequestDurationHistogram       *prometheus.HistogramVec
	metricOutgoingRequestDurationHistogram       *prometheus.HistogramVec
	metricIncomingRequestDurationNativeHistogram *prometheus.HistogramVec
	metricOutgoingRequestDurationNativeHistogram *prometheus.HistogramVec
	metricIncomingRequestDurationSummary         *prometheus.SummaryVec
	metricOutgoingRequestDurationSummary         *prometheus.SummaryVec
}

func newExampleMetrics(reg prometheus.Registerer) (m metrics) {
	m.metricIncomingRequestsPending = promauto.With(reg).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: adjustExampleMetricName("example_incoming_requests_pending", *metricNamingMode),
			Help: "The number of pending incoming requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequestsPending = promauto.With(reg).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: adjustExampleMetricName("example_outgoing_requests_pending", *metricNamingMode),
			Help: "The number of pending outgoing requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricIncomingRequests = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: adjustExampleMetricName("example_incoming_requests_total", *metricNamingMode),
			Help: "The total number of incoming requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequests = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: adjustExampleMetricName("example_outgoing_requests_total", *metricNamingMode),
			Help: "The total number of outgoing requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricIncomingRequestErrors = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: adjustExampleMetricName("example_incoming_request_errors_total", *metricNamingMode),
			Help: "The number of errors on incoming requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequestErrors = promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: adjustExampleMetricName("example_outgoing_request_errors_total", *metricNamingMode),
			Help: "The number of errors on outgoing requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricIncomingRequestDurationHistogram = promauto.With(reg).NewHistogramVec(
		//nolint:promlinter // Histogram included in metric name to disambiguate from native histogram.
		prometheus.HistogramOpts{
			Name:    adjustExampleMetricName("example_histogram_incoming_request_duration", *metricNamingMode),
			Help:    "Duration ranges of incoming requests.",
			Buckets: prometheus.LinearBuckets(0, 100, 8),
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequestDurationHistogram = promauto.With(reg).NewHistogramVec(
		//nolint:promlinter // Histogram included in metric name to disambiguate from native histogram.
		prometheus.HistogramOpts{
			Name:    adjustExampleMetricName("example_histogram_outgoing_request_duration", *metricNamingMode),
			Help:    "Duration ranges of outgoing requests.",
			Buckets: prometheus.LinearBuckets(0, 100, 8),
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricIncomingRequestDurationNativeHistogram = promauto.With(reg).NewHistogramVec(
		//nolint:promlinter // Native histogram included in metric name to disambiguate from histogram.
		prometheus.HistogramOpts{
			Name:                            adjustExampleMetricName("example_native_histogram_incoming_request_duration", *metricNamingMode),
			Help:                            "Duration ranges of incoming requests.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  150,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequestDurationNativeHistogram = promauto.With(reg).NewHistogramVec(
		//nolint:promlinter // Native histogram included in metric name to disambiguate from histogram.
		prometheus.HistogramOpts{
			Name:                            adjustExampleMetricName("example_native_histogram_outgoing_request_duration", *metricNamingMode),
			Help:                            "Duration ranges of outgoing requests.",
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramMaxBucketNumber:  150,
			NativeHistogramMinResetDuration: time.Hour,
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricIncomingRequestDurationSummary = promauto.With(reg).NewSummaryVec(
		//nolint:promlinter // Summary included in metric name to disambiguate from histograms.
		prometheus.SummaryOpts{
			Name: adjustExampleMetricName("example_summary_incoming_request_duration", *metricNamingMode),
			Help: "Duration of incoming requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	m.metricOutgoingRequestDurationSummary = promauto.With(reg).NewSummaryVec(
		//nolint:promlinter // Summary included in metric name to disambiguate from histograms.
		prometheus.SummaryOpts{
			Name: adjustExampleMetricName("example_summary_outgoing_request_duration", *metricNamingMode),
			Help: "Duration of outgoing requests.",
		},
		[]string{getStatusLabelName(*statusLabelNamingMode), "method", "path"},
	)
	return m
}

func main() {
	httpClientConfig := newHTTPClientConfigFromFlags()
	flag.Parse()
	if err := httpClientConfig.validate(); err != nil {
		log.Println("Invalid HTTP client config flags:", err)
		os.Exit(1)
	}
	if *metricNamingMode != PrometheusStyle || *statusLabelNamingMode != PrometheusStyle {
		// Extend charset.
		model.NameValidationScheme = model.UTF8Validation
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll)),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	m := newExampleMetrics(reg)

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
			func(error) {
				close(cancel)
			},
		)
	}
	{
		mux := http.NewServeMux()
		mux.Handle("/metrics", httpClientConfig.handle(promhttp.HandlerFor(reg, promhttp.HandlerOpts{
			Registry:          reg,
			EnableOpenMetrics: true,
		})))
		httpClientConfig.register(mux)

		tlsConfig, err := httpClientConfig.getTLSConfig()
		if err != nil {
			log.Println("Unable to create TLS config", err)
			os.Exit(1)
		}

		server := &http.Server{
			Addr:      *addr,
			Handler:   mux,
			TLSConfig: tlsConfig,
		}

		g.Add(func() error {
			if tlsConfig != nil {
				log.Printf("Starting server on %q with TLS\n", *addr)
				return server.ListenAndServeTLS("", "")
			}
			log.Printf("Starting server on %q\n", *addr)
			return server.ListenAndServe()
		}, func(error) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			_ = server.Shutdown(ctx)
			cancel()
		})
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(
			func() error {
				return burnCPU(ctx, *cpuBurnOps)
			},
			func(error) {
				cancel()
			},
		)
	}
	{
		ctx, cancel := context.WithCancel(context.Background())
		g.Add(
			func() error {
				return updateMetrics(ctx, m)
			},
			func(error) {
				cancel()
			},
		)
	}
	if err := g.Run(); err != nil {
		log.Println("Exit with error:", err)
		os.Exit(1)
	}
}

// allocateMemoryBallast allocates the heap with random data to simulate
// memory pressure from a real workload.
func allocateMemoryBallast(buf *[]byte, sz int) {
	// Fill memory ballast. Fill it with random values so it results in actual memory usage.
	*buf = make([]byte, sz)
	_, err := rand.Read(*buf)
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
		//nolint:revive // Intentionally empty block.
		for range ops * 20000000 {
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

// newTraceIDs generates random trace and span ID strings that conform to the
// open telemetry spec: https://github.com/open-telemetry/opentelemetry-specification/blob/v1.18.0/specification/overview.md#spancontext.
func newTraceIDs(traceBytes, spanBytes []byte) (traceID, spanID string) {
	_, _ = rand.Read(traceBytes)
	_, _ = rand.Read(spanBytes)
	return hex.EncodeToString(traceBytes), hex.EncodeToString(spanBytes)
}

// updateMetrics is a blocking function that periodically updates toy metrics
// with new values.
func updateMetrics(ctx context.Context, m metrics) error {
	projectID := "example-project"
	traceBytes := make([]byte, 16)
	spanBytes := make([]byte, 8)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(100 * time.Millisecond):
			forNumInstances(*gaugeCount, func(labels prometheus.Labels) {
				m.metricIncomingRequestsPending.With(labels).Set(float64(mathrand.Intn(200)))
				m.metricOutgoingRequestsPending.With(labels).Set(float64(mathrand.Intn(200)))
			})
			forNumInstances(*counterCount, func(labels prometheus.Labels) {
				m.metricIncomingRequests.With(labels).Add(float64(mathrand.Intn(200)))
				m.metricOutgoingRequests.With(labels).Add(float64(mathrand.Intn(100)))
				m.metricIncomingRequestErrors.With(labels).Add(float64(mathrand.Intn(15)))
				m.metricOutgoingRequestErrors.With(labels).Add(float64(mathrand.Intn(5)))
			})
			forNumInstances(*histogramCount, func(labels prometheus.Labels) {
				// Record exemplar with histogram depending on sampling fraction.
				samp := mathrand.Uint64()
				thresh := uint64(*exemplarSampling * (1 << 63))
				if samp < thresh {
					traceID, spanID := newTraceIDs(traceBytes, spanBytes)
					exemplar := prometheus.Labels{"trace_id": traceID, "span_id": spanID, "project_id": projectID}
					m.metricIncomingRequestDurationHistogram.With(labels).(prometheus.ExemplarObserver).ObserveWithExemplar(mathrand.NormFloat64()*300+500, exemplar)
				} else {
					m.metricIncomingRequestDurationHistogram.With(labels).Observe(mathrand.NormFloat64()*300 + 500)
				}
				m.metricOutgoingRequestDurationHistogram.With(labels).Observe(mathrand.NormFloat64()*200 + 300)
			})
			forNumInstances(*nativeHistogramCount, func(labels prometheus.Labels) {
				// Record exemplar with native histogram depending on sampling fraction.
				samp := mathrand.Uint64()
				thresh := uint64(*exemplarSampling * (1 << 63))
				if samp < thresh {
					traceID, spanID := newTraceIDs(traceBytes, spanBytes)
					exemplar := prometheus.Labels{"trace_id": traceID, "span_id": spanID, "project_id": projectID}
					m.metricIncomingRequestDurationNativeHistogram.With(labels).(prometheus.ExemplarObserver).ObserveWithExemplar(mathrand.NormFloat64()*300+500, exemplar)
				} else {
					m.metricIncomingRequestDurationNativeHistogram.With(labels).Observe(mathrand.NormFloat64()*300 + 500)
				}
				m.metricOutgoingRequestDurationNativeHistogram.With(labels).Observe(mathrand.NormFloat64()*200 + 300)
			})
			forNumInstances(*summaryCount, func(labels prometheus.Labels) {
				m.metricIncomingRequestDurationSummary.With(labels).Observe(mathrand.NormFloat64()*300 + 500)
				m.metricOutgoingRequestDurationSummary.With(labels).Observe(mathrand.NormFloat64()*200 + 300)
			})
		}
	}
}

//nolint:unused
type omCollector struct {
	// TODO(bwplotka): Add om custom types.
}

// forNumInstances calls a provided function to parameterize exported metrics
// with various combinations of Prometheus labels up to `c` times.
func forNumInstances(c int, f func(prometheus.Labels)) {
	if c < 0 {
		c = len(availableLabels["method"]) * len(availableLabels["status"]) * len(availableLabels["path"])
	}
	for _, path := range availableLabels["path"] {
		for _, status := range availableLabels["status"] {
			for _, method := range availableLabels["method"] {
				if c <= 0 {
					return
				}
				f(prometheus.Labels{
					"path": path,
					getStatusLabelName(*statusLabelNamingMode): status,
					"method": method,
				})
				c--
			}
		}
	}

	if c > 0 {
		for ; c > 0; c-- {
			f(prometheus.Labels{
				"path": fmt.Sprintf("/yolo/%d", c),
				getStatusLabelName(*statusLabelNamingMode): "200",
				"method": "GET",
			})
		}
	}
}
