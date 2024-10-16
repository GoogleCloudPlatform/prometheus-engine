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

package rule

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

var errRequestFailed = errors.New("request to endpoint failed")

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// client is a client for fetching rules and alerts from a Prometheus-compatible endpoint.
type client struct {
	client httpClient
}

// newClient creates a new client.
func newClient(c httpClient) *client {
	return &client{client: c}
}

// RuleGroups fetches rule-groups from a single endpoint.
func (r *client) RuleGroups(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.RuleGroup, error) {
	resp, err := r.call(ctx, baseURL, rulesPath, queryString)
	if err != nil {
		return nil, fmt.Errorf("calling endpoint failed with error: %w", err)
	}

	var parsedResp promapi.Response[promapi.RulesResponseData]
	if err := json.Unmarshal(resp, &parsedResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response from endpoint failed with error: %w", err)
	}

	return parsedResp.Data.Groups, nil
}

// Alerts fetches alerts from the endpoint.
func (r *client) Alerts(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.Alert, error) {
	resp, err := r.call(ctx, baseURL, alertsPath, queryString)
	if err != nil {
		return nil, fmt.Errorf("calling endpoint failed with error: %w", err)
	}

	var parsedResp promapi.Response[promapi.AlertsResponseData]
	if err := json.Unmarshal(resp, &parsedResp); err != nil {
		return nil, fmt.Errorf("unmarshalling response from endpoint failed with error: %w", err)
	}

	return parsedResp.Data.Alerts, nil
}

// call calls the server with the given query string and returns the response.
func (r *client) call(ctx context.Context, baseURL url.URL, endpoint, queryString string) ([]byte, error) {
	fullURL := url.URL{
		Scheme:   baseURL.Scheme,
		Host:     baseURL.Host,
		Path:     path.Join(baseURL.Path, endpoint),
		RawQuery: queryString,
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("constructing request to endpoint %s failed with error: %w", baseURL.Path, err)
	}

	resp, err := r.client.Do(req)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return nil, fmt.Errorf("request to endpoint %s was canceled: %w", baseURL.Path, err)
		}

		return nil, fmt.Errorf("request to endpoint %s failed with error: %w", baseURL.Path, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request to endpoint %s failed with status code: %d. error: %w", baseURL.Path, resp.StatusCode, errRequestFailed)
	}

	defer resp.Body.Close()
	rawResponse, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from endpoint %s failed with error: %w", baseURL.Path, err)
	}

	return rawResponse, nil
}
