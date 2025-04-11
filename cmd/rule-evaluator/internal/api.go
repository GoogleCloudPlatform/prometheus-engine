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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/rules"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
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

// RuleRetriever provides a list of active rules.
type RuleRetriever interface {
	RuleGroups() []*rules.Group
	AlertingRules() []*rules.AlertingRule
}

// API provides an HTTP API singleton for handling http endpoints in the rule evaluator.
type API struct {
	rulesManager RuleRetriever
	logger       log.Logger
}

// NewAPI creates a new API instance.
func NewAPI(logger log.Logger, rulesManager RuleRetriever) *API {
	return &API{
		rulesManager: rulesManager,
		logger:       logger,
	}
}

func (api *API) writeResponse(w http.ResponseWriter, httpResponseCode int, endpointURI string, resp response) {
	logger := log.With(api.logger, "endpointURI", endpointURI, "intendedStatusCode", httpResponseCode)
	w.Header().Set("Content-Type", "application/json")

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

// alertsToAPIAlerts converts a slice of rules.Alert to a slice of apiv1.Alert.
func alertsToAPIAlerts(alerts []*rules.Alert) []*apiv1.Alert {
	apiAlerts := make([]*apiv1.Alert, 0, len(alerts))
	for _, ruleAlert := range alerts {
		var keepFiringSince *time.Time
		if !ruleAlert.KeepFiringSince.IsZero() {
			keepFiringSince = &ruleAlert.KeepFiringSince
		}

		apiAlerts = append(apiAlerts, &apiv1.Alert{
			Labels:          ruleAlert.Labels,
			Annotations:     ruleAlert.Annotations,
			State:           ruleAlert.State.String(),
			ActiveAt:        &ruleAlert.ActiveAt,
			KeepFiringSince: keepFiringSince,
			Value:           strconv.FormatFloat(ruleAlert.Value, 'e', -1, 64),
		})
	}
	// Sort for testability.
	sort.Slice(apiAlerts, func(i, j int) bool {
		a, b := apiAlerts[i].Labels.Hash(), apiAlerts[j].Labels.Hash()
		if a == b {
			a, b = apiAlerts[i].Annotations.Hash(), apiAlerts[j].Annotations.Hash()
		}
		if a == b {
			return strings.Compare(apiAlerts[i].State, apiAlerts[j].State) < 0 // firing before pending.
		}
		return a >= b
	})
	return apiAlerts
}
