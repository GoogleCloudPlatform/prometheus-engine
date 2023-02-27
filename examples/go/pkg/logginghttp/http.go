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

// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

// Copied from https://github.com/thanos-io/thanos/tree/19dcc7902d2431265154cefff82426fbc91448a3/pkg/logging

package logginghttp

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// ResponseWriterWithStatus wraps around http.ResponseWriter to capture the status code of the response.
type ResponseWriterWithStatus struct {
	http.ResponseWriter
	statusCode      int
	isHeaderWritten bool
}

// WrapResponseWriterWithStatus wraps the http.ResponseWriter for extracting status.
func WrapResponseWriterWithStatus(w http.ResponseWriter) *ResponseWriterWithStatus {
	return &ResponseWriterWithStatus{ResponseWriter: w}
}

// Status returns http response status.
func (r *ResponseWriterWithStatus) Status() string {
	return fmt.Sprintf("%v", r.statusCode)
}

// StatusCode returns http response status code.
func (r *ResponseWriterWithStatus) StatusCode() int {
	return r.statusCode
}

// WriteHeader writes the header.
func (r *ResponseWriterWithStatus) WriteHeader(code int) {
	if !r.isHeaderWritten {
		r.statusCode = code
		r.ResponseWriter.WriteHeader(code)
		r.isHeaderWritten = true
	}
}

type HTTPMiddleware struct {
	opts   *options
	logger log.Logger
}

var RequestIDCtxKey struct{}

func (m *HTTPMiddleware) getRequestID(r *http.Request) string {
	id, ok := r.Context().Value(RequestIDCtxKey).(string)
	if !ok {
		return r.Header.Get("X-Request-ID")
	}
	return id
}

func (m *HTTPMiddleware) preCall(name string, start time.Time, r *http.Request) {
	logger := m.opts.filterLog(m.logger)

	_ = level.Debug(logger).Log("http.start_time", start.String(), "http.method", fmt.Sprintf("%s %s", r.Method, r.URL), "http.request_id", m.getRequestID(r), "thanos.method_name", name, "msg", "started call")
}

func (m *HTTPMiddleware) postCall(name string, start time.Time, wrapped *ResponseWriterWithStatus, r *http.Request) {
	logger := log.With(m.logger, "http.method", fmt.Sprintf("%s %s", r.Method, r.URL), "http.request_id", m.getRequestID(r), "http.status_code", wrapped.Status(),
		"http.time_ms", fmt.Sprintf("%v", durationToMilliseconds(time.Since(start))), "http.remote_addr", r.RemoteAddr, "thanos.method_name", name)

	logger = m.opts.filterLog(logger)
	_ = m.opts.levelFunc(logger, wrapped.StatusCode()).Log("msg", "finished call")
}

func (m *HTTPMiddleware) WrapHandler(name string, next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		wrapped := WrapResponseWriterWithStatus(w)
		start := time.Now()
		hostPort := r.Host
		if hostPort == "" {
			hostPort = r.URL.Host
		}

		var port string
		var err error
		// Try to extract port if there is ':' as part of 'hostPort'.
		if strings.Contains(hostPort, ":") {
			_, port, err = net.SplitHostPort(hostPort)
			if err != nil {
				level.Error(m.logger).Log("msg", "failed to parse host port for http log decision", "err", err)
				next.ServeHTTP(w, r)
				return
			}
		}

		deciderURL := r.URL.String()
		if len(port) > 0 {
			deciderURL = net.JoinHostPort(deciderURL, port)
		}
		decision := m.opts.shouldLog(deciderURL, nil)

		switch decision {
		case NoLogCall:
			next.ServeHTTP(w, r)

		case LogStartAndFinishCall:
			m.preCall(name, start, r)
			next.ServeHTTP(wrapped, r)
			m.postCall(name, start, wrapped, r)

		case LogFinishCall:
			next.ServeHTTP(wrapped, r)
			m.postCall(name, start, wrapped, r)
		}
	}
}

// NewHTTPServerMiddleware returns an http middleware.
func NewHTTPServerMiddleware(logger log.Logger, opts ...Option) *HTTPMiddleware {
	o := evaluateOpt(opts)
	return &HTTPMiddleware{
		logger: log.With(logger, "protocol", "http", "http.component", "server"),
		opts:   o,
	}
}
