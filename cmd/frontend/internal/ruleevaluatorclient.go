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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"

	"github.com/GoogleCloudPlatform/prometheus-engine/internal/promapi"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
)

const (
	rulesPath  = "/api/v1/rules"
	alertsPath = "/api/v1/alerts"
)

var errRequestFailed = errors.New("request to rule evaluator failed")

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// RuleEvaluatorClient is a client for fetching rules and alerts from a rule evaluator.
type RuleEvaluatorClient struct {
	client httpClient
}

// NewRuleEvaluatorClient creates a new RuleEvaluatorClient.
func NewRuleEvaluatorClient(cli httpClient) *RuleEvaluatorClient {
	return &RuleEvaluatorClient{client: cli}
}

// RuleGroups fetches rule-groups from the rule evaluator.
func (r *RuleEvaluatorClient) RuleGroups(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.RuleGroup, error) {
	resp, err := r.callRuleEvaluator(ctx, baseURL, rulesPath, queryString)
	if err != nil {
		return nil, fmt.Errorf("calling rule evaluator failed with error: %w", err)
	}

	var ruleEvaluatorParsedResponse promapi.Response[promapi.RulesResponseData]
	if err := json.Unmarshal(resp, &ruleEvaluatorParsedResponse); err != nil {
		return nil, fmt.Errorf("unmarshalling response from rule evaluator failed with error: %w", err)
	}

	return ruleEvaluatorParsedResponse.Data.Groups, nil
}

// Alerts fetches alerts from the rule evaluator.
func (r *RuleEvaluatorClient) Alerts(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.Alert, error) {
	resp, err := r.callRuleEvaluator(ctx, baseURL, alertsPath, queryString)
	if err != nil {
		return nil, fmt.Errorf("calling rule evaluator failed with error: %w", err)
	}

	var ruleEvaluatorParsedResponse promapi.Response[promapi.AlertsResponseData]
	if err := json.Unmarshal(resp, &ruleEvaluatorParsedResponse); err != nil {
		return nil, fmt.Errorf("unmarshalling response from rule evaluator failed with error: %w", err)
	}

	return ruleEvaluatorParsedResponse.Data.Alerts, nil
}

// callRuleEvaluator calls the rule evaluator with the given query string and returns the response.
func (r *RuleEvaluatorClient) callRuleEvaluator(ctx context.Context, baseURL url.URL, endpoint, queryString string) ([]byte, error) {
	fullURL := url.URL{
		Scheme:   baseURL.Scheme,
		Host:     baseURL.Host,
		Path:     path.Join(baseURL.Path, endpoint),
		RawQuery: queryString,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("constructing request to rule evaluator %s failed with error: %w", baseURL.Path, err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("request to rule evaluator %s was canceled: %w", baseURL.Path, err)
		}

		return nil, fmt.Errorf("request to rule evaluator %s failed with error: %w", baseURL.Path, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request to rule evaluator %s failed with status code: %d. error: %w", baseURL.Path, resp.StatusCode, errRequestFailed)
	}

	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from rule evaluator %s failed with error: %w", baseURL.Path, err)
	}

	return rawResponse, nil
}
