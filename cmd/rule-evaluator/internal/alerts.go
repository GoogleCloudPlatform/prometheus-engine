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
	"net/http"

	"github.com/prometheus/prometheus/rules"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
)

type alertsEndpointResponse struct {
	Alerts []*apiv1.Alert `json:"alerts"`
}

func (api *API) HandleAlertsEndpoint(w http.ResponseWriter, _ *http.Request) {
	activeAlerts := []*rules.Alert{}
	for _, rule := range api.rulesManager.AlertingRules() {
		activeAlerts = append(activeAlerts, rule.ActiveAlerts()...)
	}

	alertsResponse := alertsEndpointResponse{
		Alerts: alertsToAPIAlerts(activeAlerts),
	}
	api.writeSuccessResponse(w, http.StatusOK, "/api/v1/alerts", alertsResponse)
}
