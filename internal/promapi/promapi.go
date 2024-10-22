// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
)

// Redundant code for API compliance below can be DRY'ed up if/when this issue is addressed:
// https://github.com/prometheus/prometheus/issues/14962

// https://prometheus.io/docs/prometheus/latest/querying/api/#format-overview
// Response is the prometheus-compatible Response format.
type Response[T RulesResponseData | AlertsResponseData | GenericResponseData] struct {
	Status    status    `json:"status"`
	Data      T         `json:"data,omitempty"`
	ErrorType ErrorType `json:"errorType,omitempty"`
	Error     string    `json:"error,omitempty"`
	Warnings  []string  `json:"warnings,omitempty"`
	Infos     []string  `json:"infos,omitempty"`
}

type RulesResponseData struct {
	Groups []*promapiv1.RuleGroup `json:"groups"`
}

type AlertsResponseData struct {
	Alerts []*promapiv1.Alert `json:"alerts"`
}

type GenericResponseData interface{}

type ErrorType string

const (
	ErrorNone        ErrorType = ""
	ErrorTimeout     ErrorType = "timeout"
	ErrorCanceled    ErrorType = "canceled"
	ErrorExec        ErrorType = "execution"
	ErrorBadData     ErrorType = "bad_data"
	ErrorInternal    ErrorType = "internal"
	ErrorUnavailable ErrorType = "unavailable"
	ErrorNotFound    ErrorType = "not_found"
)

// https://prometheus.io/docs/prometheus/latest/querying/api/#format-overview
// status is the prometheus-compatible status type.
type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

// writeResponse writes a Response to given responseWriter w if it can, otherwise it logs the error and writes a generic error.
func writeResponse[T RulesResponseData | AlertsResponseData | GenericResponseData](logger log.Logger, w http.ResponseWriter, httpResponseCode int, endpointURI string, resp Response[T]) {
	logger = log.With(logger, "endpointURI", endpointURI, "intendedStatusCode", httpResponseCode)
	w.Header().Set("Content-Type", "application/json")

	jsonResponse, err := json.Marshal(resp)
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to marshal Response", "err", err)
		w.WriteHeader(http.StatusInternalServerError)

		if _, err = w.Write([]byte(`{"status":"error","ErrorType":"internal","error":"failed to marshal Response"}`)); err != nil {
			_ = level.Error(logger).Log("msg", "failed to write error Response to responseWriter", "err", err)
		}
		return
	}

	w.WriteHeader(httpResponseCode)
	if _, err = w.Write(jsonResponse); err != nil {
		_ = level.Error(logger).Log("msg", "failed to write Response to responseWriter", "err", err)
	}
}

// WriteSuccessResponse writes a successful Response to the given responseWriter w.
func WriteSuccessResponse[T RulesResponseData | AlertsResponseData | promapiv1.PrometheusVersion](logger log.Logger, w http.ResponseWriter, httpResponseCode int, endpointURI string, responseData T) {
	writeResponse(logger, w, httpResponseCode, endpointURI, Response[T]{
		Status: statusSuccess,
		Data:   responseData,
	})
}

// WriteError writes an error Response to the given responseWriter w.
func WriteError(logger log.Logger, w http.ResponseWriter, errType ErrorType, errMsg string, httpResponseCode int, endpointURI string) {
	writeResponse(logger, w, httpResponseCode, endpointURI, Response[GenericResponseData]{
		Status:    statusError,
		ErrorType: errType,
		Error:     errMsg,
		Data:      nil,
	})
}
