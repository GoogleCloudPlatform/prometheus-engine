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

package instrumentationhttp

import (
	"context"
	"net/http"

	"github.com/GoogleCloudPlatform/prometheus-engine/examples/go/pkg/logginghttp"

	"github.com/bwplotka/tracing-go/tracing"
	tracinghttp "github.com/bwplotka/tracing-go/tracing/http"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Middleware auto instruments net/http HTTP handlers with:
// * Prometheus metrics + exemplars
// * Logging
// * Tracing + propagation
type Middleware interface {
	// WrapHandler wraps the given HTTP handler for instrumentation.
	WrapHandler(handlerName string, handler http.Handler) http.HandlerFunc
}

type nopMiddleware struct{}

func (ins nopMiddleware) WrapHandler(_ string, handler http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handler.ServeHTTP(w, r)
	}
}

// NewNopMiddleware provides a Middleware which does nothing.
func NewNopMiddleware() Middleware {
	return nopMiddleware{}
}

type middleware struct {
	reg             prometheus.Registerer
	logMiddleware   *logginghttp.HTTPMiddleware
	traceMiddleware *tracinghttp.Middleware

	buckets []float64
}

// NewMiddleware provides a Middleware that allows wrapping HTTP requests with logs, traces and metrics (with exemplars).
// Passing nil as buckets uses the default buckets.
// Passing nil to logger or traces disables logging or tracing instrumentation.
func NewMiddleware(reg prometheus.Registerer, buckets []float64, logger log.Logger, tracer *tracing.Tracer) Middleware {
	if buckets == nil {
		buckets = []float64{0.001, 0.01, 0.1, 0.3, 0.6, 1, 3, 6, 9, 20, 30, 60, 90, 120, 240, 360, 720}
	}

	return &middleware{
		reg:             reg,
		buckets:         buckets,
		logMiddleware:   logginghttp.NewHTTPServerMiddleware(logger),
		traceMiddleware: tracinghttp.NewMiddleware(tracer),
	}
}

// WrapHandler wraps the given HTTP handler for instrumentation:
// * It registers four metric collectors (if not already done) and reports HTTP metrics to the (newly or already)
// registered collectors: http_requests_total (CounterVec), http_request_duration_seconds (Histogram),
// http_request_size_bytes (Summary), http_response_size_bytes (Summary). Each has a constant label named "handler"
// with the provided handlerName as value. http_requests_total is a metric vector partitioned by HTTP method
// (label name "method") and HTTP status code (label name "code").
// * Logs requests and responses.
// * Adds spans and propagate trace metadata from request if any.
func (ins *middleware) WrapHandler(handlerName string, handler http.Handler) http.HandlerFunc {
	reg := prometheus.WrapRegistererWith(prometheus.Labels{"handler": handlerName}, ins.reg)

	requestDuration := promauto.With(reg).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "Tracks the latencies for HTTP requests.",
			Buckets: ins.buckets,
		},
		[]string{"method", "code"},
	)
	requestSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "Tracks the size of HTTP requests.",
		},
		[]string{"method", "code"},
	)
	requestsTotal := promauto.With(reg).NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Tracks the number of HTTP requests.",
		}, []string{"method", "code"},
	)
	responseSize := promauto.With(reg).NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_response_size_bytes",
			Help: "Tracks the size of HTTP responses.",
		},
		[]string{"method", "code"},
	)

	getExemplarFn := func(ctx context.Context) prometheus.Labels {
		if spanCtx := tracing.GetSpan(ctx); spanCtx.Context().IsSampled() {
			return prometheus.Labels{"traceID": spanCtx.Context().TraceID()}
		}

		return nil
	}

	base := promhttp.InstrumentHandlerRequestSize(
		requestSize,
		promhttp.InstrumentHandlerCounter(
			requestsTotal,
			promhttp.InstrumentHandlerResponseSize(
				responseSize,
				promhttp.InstrumentHandlerDuration(
					requestDuration,
					http.HandlerFunc(func(writer http.ResponseWriter, r *http.Request) {
						handler.ServeHTTP(writer, r)
					}),
					promhttp.WithExemplarFromContext(getExemplarFn),
				),
			),
			promhttp.WithExemplarFromContext(getExemplarFn),
		),
	)

	if ins.logMiddleware != nil {
		// Add context values that gives more context to request logging.
		next := base
		base = ins.logMiddleware.WrapHandler(handlerName, next)

		next = base
		base = func(w http.ResponseWriter, r *http.Request) {
			spanCtx := tracing.GetSpan(r.Context()).Context()
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), logginghttp.RequestIDCtxKey, spanCtx.TraceID())))
		}
	}

	if ins.traceMiddleware != nil {
		next := base
		// Wrap with tracing. This will be visited as a first middleware.
		base = ins.traceMiddleware.WrapHandler(handlerName, next)
	}
	return base.ServeHTTP
}
