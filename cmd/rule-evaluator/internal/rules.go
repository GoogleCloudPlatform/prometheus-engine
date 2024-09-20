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
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/log/level"
	"github.com/prometheus/prometheus/rules"
	apiv1 "github.com/prometheus/prometheus/web/api/v1"
)

const (
	ruleKindAlerting  string = "alerting"
	ruleKindRecording string = "recording"

	ruleTypeFilterAlert  string = "alert"
	ruleTypeFilterRecord string = "record"

	ruleFilterQueryParamName  = "rule_name[]"
	fileFilterQueryParamName  = "file[]"
	groupFilterQueryParamName = "rule_group[]"
	matchFilterQueryParamName = "match[]"
	typeFilterQueryParamName  = "type"
	excludeAlertsQueryParam   = "exclude_alerts"

	rulesEndpoint = "/api/v1/rules"
)

// sanitizeFilterList removes empty strings from a list of filters and trims spaces from each filter.
func sanitizeFilterList(filters []string) []string {
	filterSet := []string{}
	for _, filter := range filters {
		filter = strings.Trim(filter, " ")
		if filter != "" {
			filterSet = append(filterSet, filter)
		}
	}
	return filterSet
}

type rulesEndpointResponse struct {
	Groups []*apiv1.RuleGroup `json:"groups"`
}

func (api *API) HandleRulesEndpoint(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		api.writeError(w, errorBadData, "failed to parse request parameters", http.StatusBadRequest, rulesEndpoint)
		return
	}

	ruleFilters := sanitizeFilterList(r.Form[ruleFilterQueryParamName])
	fileFilters := sanitizeFilterList(r.Form[fileFilterQueryParamName])
	groupFilters := sanitizeFilterList(r.Form[groupFilterQueryParamName])

	matchFilters := sanitizeFilterList(r.Form[matchFilterQueryParamName])
	if len(matchFilters) > 0 {
		// Todo: Once the github.com/prometheus/prometheus dependency is updated to v2.54.0 or higher, we can add
		//       support for labels-matcher filter, using the match[] request parameter.
		// Ref:  https://github.com/prometheus/prometheus/releases/tag/v2.54.0
		// Ref:  https://github.com/prometheus/prometheus/pull/10194
		api.writeError(w, errorBadData, "match[] parameter is not supported yet", http.StatusBadRequest, rulesEndpoint)
		return
	}

	ruleTypeFilter := strings.Trim(strings.ToLower(r.URL.Query().Get(typeFilterQueryParamName)), " ")
	if !slices.Contains([]string{"", ruleTypeFilterAlert, ruleTypeFilterRecord}, ruleTypeFilter) {
		api.writeError(w, errorBadData, "invalid type parameter", http.StatusBadRequest, rulesEndpoint)
		return
	}
	shouldReturnAlertRules := ruleTypeFilter == "" || ruleTypeFilter == ruleTypeFilterAlert
	shouldReturnRecordingRules := ruleTypeFilter == "" || ruleTypeFilter == ruleTypeFilterRecord

	excludeAlertsParam := strings.Trim(strings.ToLower(r.URL.Query().Get(excludeAlertsQueryParam)), " ")
	if !slices.Contains([]string{"", "true", "false"}, excludeAlertsParam) {
		api.writeError(w, errorBadData, "invalid exclude_alerts parameter", http.StatusBadRequest, rulesEndpoint)
		return
	}
	shouldExcludeAlertsFromAlertRules := excludeAlertsParam == "true"

	apiGroups := api.groupsToAPIGroups(api.rulesManager.RuleGroups(), ruleFilters, fileFilters, groupFilters, shouldReturnAlertRules, shouldReturnRecordingRules, shouldExcludeAlertsFromAlertRules)
	responseObject := rulesEndpointResponse{Groups: apiGroups}

	api.writeSuccessResponse(w, http.StatusOK, rulesEndpoint, responseObject)
}

// groupsToAPIGroups converts a slice of rules.Group to a slice of apiv1.RuleGroup.
func (api *API) groupsToAPIGroups(groups []*rules.Group, ruleFilters, fileFilters, groupFilters []string, shouldReturnAlertRules, shouldReturnRecordingRules, shouldExcludeAlertsFromAlertRules bool) []*apiv1.RuleGroup {
	apiGroups := []*apiv1.RuleGroup{} // don't pre-allocate, we don't know how many rule groups we will return
	for _, group := range groups {
		// If a rule_group parameter was specified, skip the rule group if it doesn't match any of the specified values.
		if len(groupFilters) > 0 && !slices.Contains(groupFilters, group.Name()) {
			continue
		}

		// If a file parameter was specified, skip the rule group if it doesn't match any of the specified values.
		if len(fileFilters) > 0 && !slices.Contains(fileFilters, group.File()) {
			continue
		}

		apiGroup := api.groupToAPIGroup(group, ruleFilters, shouldReturnAlertRules, shouldReturnRecordingRules, shouldExcludeAlertsFromAlertRules)

		// If we filtered out all rules from the group, skip the group.
		if len(apiGroup.Rules) == 0 {
			continue
		}

		apiGroups = append(apiGroups, apiGroup)
	}
	return apiGroups
}

// groupToAPIGroup converts a rules.Group to an apiv1.RuleGroup.
func (api *API) groupToAPIGroup(group *rules.Group, ruleFilters []string, shouldReturnAlertRules, shouldReturnRecordingRules, shouldExcludeAlertsFromAlertRules bool) *apiv1.RuleGroup {
	apiGroupRules := []apiv1.Rule{}
	for _, groupRules := range group.Rules() {
		// If a rule_name parameter was specified, skip the rule if it doesn't match any of the specified values.
		if len(ruleFilters) > 0 && !slices.Contains(ruleFilters, groupRules.Name()) {
			continue
		}

		switch rule := groupRules.(type) {
		case *rules.AlertingRule:
			if !shouldReturnAlertRules {
				continue
			}
			apiGroupRules = append(apiGroupRules, alertingRuleToAPIRule(rule, shouldExcludeAlertsFromAlertRules))
		case *rules.RecordingRule:
			if !shouldReturnRecordingRules {
				continue
			}
			apiGroupRules = append(apiGroupRules, recordingRuleToAPIRule(rule))
		default:
			err := fmt.Errorf("alert rule %s is of unknown type %T", rule.Name(), rule)
			_ = level.Warn(api.logger).Log("msg", "failed to convert rule to API rule", "err", err)
			continue // ignore faulty rules - this should not break the endpoint.
		}
	}

	return &apiv1.RuleGroup{
		Name:           group.Name(),
		File:           group.File(),
		Interval:       group.Interval().Seconds(),
		Limit:          group.Limit(),
		Rules:          apiGroupRules,
		EvaluationTime: group.GetEvaluationTime().Seconds(),
		LastEvaluation: group.GetLastEvaluation(),
	}
}

// recordingRuleToAPIRule converts a rules.RecordingRule to an apiv1.RecordingRule.
func recordingRuleToAPIRule(rule *rules.RecordingRule) *apiv1.RecordingRule {
	lastError := ""
	if rule.LastError() != nil {
		lastError = rule.LastError().Error()
	}
	return &apiv1.RecordingRule{
		Name:           rule.Name(),
		Query:          rule.Query().String(),
		Labels:         rule.Labels(),
		Health:         rule.Health(),
		LastError:      lastError,
		EvaluationTime: rule.GetEvaluationDuration().Seconds(),
		LastEvaluation: rule.GetEvaluationTimestamp(),
		Type:           ruleKindRecording,
	}
}

// alertingRuleToAPIRule converts a rules.AlertingRule to an apiv1.AlertingRule.
func alertingRuleToAPIRule(rule *rules.AlertingRule, shouldExcludeAlertsFromAlertRules bool) *apiv1.AlertingRule {
	lastError := ""
	if rule.LastError() != nil {
		lastError = rule.LastError().Error()
	}

	alerts := []*apiv1.Alert{}
	if !shouldExcludeAlertsFromAlertRules {
		alerts = alertsToAPIAlerts(rule.ActiveAlerts())
	}

	return &apiv1.AlertingRule{
		State:          rule.State().String(),
		Name:           rule.Name(),
		Query:          rule.Query().String(),
		Duration:       rule.HoldDuration().Seconds(),
		KeepFiringFor:  rule.KeepFiringFor().Seconds(),
		Labels:         rule.Labels(),
		Annotations:    rule.Annotations(),
		Alerts:         alerts,
		Health:         rule.Health(),
		LastError:      lastError,
		EvaluationTime: rule.GetEvaluationDuration().Seconds(),
		LastEvaluation: rule.GetEvaluationTimestamp(),
		Type:           ruleKindAlerting,
	}
}

// alertsToAPIAlerts converts a slice of rules.Alert to a slice of apiv1.Alert.
func alertsToAPIAlerts(alerts []*rules.Alert) []*apiv1.Alert {
	apiAlerts := make([]*apiv1.Alert, len(alerts))
	for i, ruleAlert := range alerts {
		var keepFiringSince *time.Time
		if !ruleAlert.KeepFiringSince.IsZero() {
			keepFiringSince = &ruleAlert.KeepFiringSince
		}

		apiAlerts[i] = &apiv1.Alert{
			Labels:          ruleAlert.Labels,
			Annotations:     ruleAlert.Annotations,
			State:           ruleAlert.State.String(),
			ActiveAt:        &ruleAlert.ActiveAt,
			KeepFiringSince: keepFiringSince,
			Value:           strconv.FormatFloat(ruleAlert.Value, 'e', -1, 64),
		}
	}

	return apiAlerts
}
