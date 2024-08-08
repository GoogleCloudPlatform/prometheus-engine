// Copyright 2024 Google LLC
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
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export/exportv2"
	writev2 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/prompb/io/prometheus/write/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/golang/snappy"
)

const (
	pathPrefix = "/v1/projects/"
	pathSuffix = "/location/global/prometheus/api/v2/write"

	appProtoContentType        = "application/x-protobuf"
	prwV2ContentTypeProtoParam = "io.prometheus.write.v2.Request"
)

var projectIDRe = regexp.MustCompile("[a-z-0-9]+")

type prwHandler struct {
	exporter               *exportv2.Exporter
	logger                 log.Logger
	allowClassicHistograms bool
}

func registerRWHandler(mux *http.ServeMux, gcmClient *monitoring.MetricClient, logger log.Logger, allowClassicHistograms bool) {
	p := &prwHandler{
		exporter:               exportv2.NewExporter(gcmClient, logger),
		logger:                 logger,
		allowClassicHistograms: allowClassicHistograms,
	}
	// We expect to serve in PRW /v1/projects/PROJECT_ID/location/global/prometheus/api/v2/write
	mux.Handle(pathPrefix, http.StripPrefix(pathPrefix, p))
}

func ensureV2Message(contentType string) error {
	contentType = strings.TrimSpace(contentType)

	parts := strings.Split(contentType, ";")
	if parts[0] != appProtoContentType {
		return fmt.Errorf("expected %v as the first (media) part, got %v content-type", appProtoContentType, contentType)
	}
	// Parse potential https://www.rfc-editor.org/rfc/rfc9110#parameter
	for _, p := range parts[1:] {
		pair := strings.Split(p, "=")
		if len(pair) != 2 {
			return fmt.Errorf("as per https://www.rfc-editor.org/rfc/rfc9110#parameter expected parameters to be key-values, got %v in %v content-type", p, contentType)
		}
		if pair[0] == "proto" {
			if pair[1] == prwV2ContentTypeProtoParam {
				return nil
			}
			return fmt.Errorf("unsupported %v content type", contentType)
		}
	}
	// No "proto=" parameter, assuming v1.
	return fmt.Errorf("prometheus remote-write 1.0 message is not; got %v content type", contentType)
}

func (p *prwHandler) http400Error(w http.ResponseWriter, err error) {
	// Request logging, even error ones, are always debug, otherwise can be spammy.
	level.Debug(p.logger).Log("msg", "handling user request failed; bad request", "err", err)
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func (p *prwHandler) http415Error(w http.ResponseWriter, err error) {
	// Request logging, even error ones, are always debug, otherwise can be spammy.
	level.Debug(p.logger).Log("msg", "handling user request failed; unsupported media type", "err", err)
	http.Error(w, err.Error(), http.StatusUnsupportedMediaType)
}

// TODO(bwplotka): Instrument with metrics.
// TODO(bwplotka): Add concurrency limit.
// TODO(bwplotka): Add body size limiter.
func (p *prwHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}

	// Path suffix should be PROJECT_ID/location/global/prometheus/api/v2/write.
	if strings.HasSuffix(r.URL.Path, pathSuffix) {
		http.NotFound(w, r)
		return
	}

	// Retrieve and sanitize the PROJECT_ID.
	projID := strings.TrimSuffix(r.URL.Path, pathSuffix)
	if !projectIDRe.MatchString(projID) {
		p.http400Error(w, fmt.Errorf("the PROJECT_ID from /v1/projects/PROJECT_ID/location/global/prometheus/api/v2/write has unsupported value, got %q, expected value that matches \"[a-z-0-9]+\"", projID))
		return
	}

	// Ensure PRW headers.
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		p.http415Error(w, errors.New("missing request Content-Type header"))
		return
	}

	if err := ensureV2Message(contentType); err != nil {
		p.http415Error(w, fmt.Errorf("%w; this receiver suppports only remote write 2.0 messages, so content-type %q", err, appProtoContentType+";proto="+prwV2ContentTypeProtoParam))
		return
	}

	enc := r.Header.Get("Content-Encoding")
	if enc == "" {
		p.http415Error(w, errors.New("missing request Content-Encoding header"))
		return
	}
	if enc != "snappy" {
		p.http415Error(w, fmt.Errorf("unsupported Content-Encoding %q, requires snappy", enc))
		return
	}

	// Read the request body.
	body, err := io.ReadAll(r.Body)
	if err != nil {
		p.http400Error(w, fmt.Errorf("error reading request body; %w", err))
		return
	}

	decompressed, err := snappy.Decode(nil, body)
	if err != nil {
		p.http400Error(w, fmt.Errorf("error decompresing request payload; %w", err))
		return
	}

	// TODO(bwplotka): Benchmark pooling, to see if needed/helpful.
	req := writev2.RequestFromVTPool()
	req.ResetVT()
	if err := req.UnmarshalVT(decompressed); err != nil {
		p.http400Error(w, fmt.Errorf("error decoding io.prometheus.write.v2.Request; %w", err))
		return
	}

	if err := p.exporter.ExportPRW(r.Context(), req, p.allowClassicHistograms); err != nil {
		l := level.Debug(p.logger)
		if err.HTTPCode() == http.StatusInternalServerError {
			l = level.Error(p.logger)
		}
		l.Log("msg", "exportv2.ExportPRW failed for PRW 2.0 user request", "err", err.Error(), "seriesNum", len(req.Timeseries))
		http.Error(w, err.Error(), err.HTTPCode())
		return
	}

	// TODO(bwplotka): Consider stats as per https://github.com/prometheus/prometheus/issues/14359
	w.WriteHeader(http.StatusNoContent)
}
