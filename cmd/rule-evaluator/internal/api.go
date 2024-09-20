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

package internal

import (
	"encoding/json"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/rules"
)

type errorType string

// Copied from github.com/prometheus/prometheus/web/api/v1.
const (
	errorNone        errorType = ""
	errorTimeout     errorType = "timeout"
	errorCanceled    errorType = "canceled"
	errorExec        errorType = "execution"
	errorBadData     errorType = "bad_data"
	errorInternal    errorType = "internal"
	errorUnavailable errorType = "unavailable"
	errorNotFound    errorType = "not_found"
)

// https://prometheus.io/docs/prometheus/latest/querying/api/#format-overview
// status is the prometheus-compatible status type.
type status string

const (
	statusSuccess status = "success"
	statusError   status = "error"
)

// https://prometheus.io/docs/prometheus/latest/querying/api/#format-overview
// response is the prometheus-compatible response format.
type response struct {
	Status    status      `json:"status"`
	Data      interface{} `json:"data,omitempty"`
	ErrorType errorType   `json:"errorType,omitempty"`
	Error     string      `json:"error,omitempty"`
	Warnings  []string    `json:"warnings,omitempty"`
	Infos     []string    `json:"infos,omitempty"`
}

// RuleGroupsRetriever RulesRetriever provides a list of active rules.
type RuleGroupsRetriever interface {
	RuleGroups() []*rules.Group
}

// API provides an HTTP API singleton for handling http endpoints in the rule evaluator.
type API struct {
	rulesManager RuleGroupsRetriever
	logger       log.Logger
}

// NewAPI creates a new API instance.
func NewAPI(logger log.Logger, rulesManager RuleGroupsRetriever) *API {
	return &API{
		rulesManager: rulesManager,
		logger:       logger,
	}
}

func (api *API) writeResponse(w http.ResponseWriter, httpResponseCode int, endpointURI string, resp response) {
	logger := log.With(api.logger, "endpointURI", endpointURI, "intendedStatusCode", httpResponseCode)

	jsonResponse, err := json.Marshal(resp)
	if err != nil {
		_ = level.Error(logger).Log("msg", "failed to marshal response", "err", err)

		w.WriteHeader(http.StatusInternalServerError)
		if _, err = w.Write([]byte(`{"status":"error","errorType":"internal","error":"failed to marshal response"}`)); err != nil {
			_ = level.Error(logger).Log("msg", "failed to write error response to responseWriter", "err", err)
		}
	}

	w.WriteHeader(httpResponseCode)
	if _, err = w.Write(jsonResponse); err != nil {
		_ = level.Error(logger).Log("msg", "failed to write response to responseWriter", "err", err)
	}
}

func (api *API) writeSuccessResponse(w http.ResponseWriter, httpResponseCode int, endpointURI string, data interface{}) {
	api.writeResponse(w, httpResponseCode, endpointURI, response{
		Status: statusSuccess,
		Data:   data,
	})
}

// writeError writes an error response to the client if it can, otherwise it logs the error and writes a generic error.
func (api *API) writeError(w http.ResponseWriter, errType errorType, errMsg string, httpResponseCode int, endpointURI string) {
	api.writeResponse(w, httpResponseCode, endpointURI, response{
		Status:    statusError,
		ErrorType: errType,
		Error:     errMsg,
	})
}
