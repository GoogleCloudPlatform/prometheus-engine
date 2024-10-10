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
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/GoogleCloudPlatform/prometheus-engine/internal/promapi"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
)

var errAllRuleEvaluatorsFailed = errors.New("all rules evaluators failed")

// RuleRetriever is an interface for fetching rules and alerts from a rule evaluator.
type RuleRetriever interface {
	RuleGroups(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.RuleGroup, error)
	Alerts(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.Alert, error)
}

// RuleEvaluatorForwarder forwards requests to the rules and alerts endpoints of multiple rule evaluators.
type RuleEvaluatorForwarder struct {
	logger                log.Logger
	ruleEvaluatorBaseURLs []url.URL
	ruleRetriever         RuleRetriever
}

// NewRuleEvaluatorForwarder creates a new RuleEvaluatorForwarder.
func NewRuleEvaluatorForwarder(logger log.Logger, ruleEvaluatorBaseURLs []url.URL, ruleRetriever RuleRetriever) *RuleEvaluatorForwarder {
	return &RuleEvaluatorForwarder{
		logger:                logger,
		ruleEvaluatorBaseURLs: ruleEvaluatorBaseURLs,
		ruleRetriever:         ruleRetriever,
	}
}

// ForwardToRuleEvaluatorsRulesEndpoint forwards requests to the rules endpoint of multiple rule evaluators.
func (r *RuleEvaluatorForwarder) ForwardToRuleEvaluatorsRulesEndpoint() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		rules, err := parallelCallRuleEvaluator[*promapiv1.RuleGroup](req.Context(), r.logger, r.ruleEvaluatorBaseURLs, req.URL.RawQuery, r.ruleRetriever.RuleGroups)
		if err != nil {
			r.handleError(w, req, err)
			return
		}

		promapi.WriteSuccessResponse(r.logger, w, http.StatusOK, req.URL.Path, promapi.RulesResponseData{Groups: rules})
	}
}

// ForwardToRuleEvaluatorsAlertsEndpoint forwards requests to the alerts endpoint of multiple rule evaluators.
func (r *RuleEvaluatorForwarder) ForwardToRuleEvaluatorsAlertsEndpoint() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		alerts, err := parallelCallRuleEvaluator[*promapiv1.Alert](req.Context(), r.logger, r.ruleEvaluatorBaseURLs, req.URL.RawQuery, r.ruleRetriever.Alerts)
		if err != nil {
			r.handleError(w, req, err)
			return
		}

		promapi.WriteSuccessResponse(r.logger, w, http.StatusOK, req.URL.Path, promapi.AlertsResponseData{Alerts: alerts})
	}
}

// parallelCallRuleEvaluator calls the rule evaluator endpoints in parallel and returns the combined results.
func parallelCallRuleEvaluator[T *promapiv1.Alert | *promapiv1.RuleGroup](
	ctx context.Context,
	logger log.Logger,
	ruleEvaluatorBaseURLs []url.URL,
	rawQuery string,
	retrieverFunc func(context.Context, url.URL, string) ([]T, error),
) ([]T, error) {
	if len(ruleEvaluatorBaseURLs) == 0 {
		_ = level.Warn(logger).Log("msg", "tried to fetch rules/alerts, no rule evaluators configured")
		return []T{}, nil
	}

	resultChan, errChan := make(chan []T), make(chan error)
	wg := sync.WaitGroup{}
	{ // Parallel call to all rule evaluators and shove all results into a channel
		for _, baseURL := range ruleEvaluatorBaseURLs {
			wg.Add(1)
			go func(baseURL url.URL) {
				defer wg.Done()

				result, err := retrieverFunc(ctx, baseURL, rawQuery)
				if err != nil {
					errChan <- fmt.Errorf("retrieving alerts from %s failed: %w", baseURL.String(), err)
					return
				}

				resultChan <- result
			}(baseURL)
		}
	}

	{ // Wait for all rule evaluators to finish and close the channels
		go func() {
			wg.Wait()
			close(resultChan)
			close(errChan)
		}()
	}

	var results []T
	var errs []error
	{ // Collect results and errors from the channels
		for resultChan != nil || errChan != nil {
			select {
			case result, ok := <-resultChan:
				if !ok {
					resultChan = nil
					continue
				}
				results = append(results, result...)
			case err, ok := <-errChan:
				if !ok {
					errChan = nil
					continue
				}
				errs = append(errs, err)
			}
		}
	}

	{ // Error Handling
		if len(errs) != 0 {
			if len(errs) == len(ruleEvaluatorBaseURLs) {
				_ = level.Error(logger).Log("msg", "all rules evaluators failed", "errors", errs)
				return nil, errAllRuleEvaluatorsFailed
			}
			_ = level.Warn(logger).Log("msg", "some rules evaluators failed", "errors", errs)
		}
	}

	return results, nil
}

// handleError writes an error response to the client based on the error.
func (r *RuleEvaluatorForwarder) handleError(w http.ResponseWriter, req *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		promapi.WriteError(r.logger, w, promapi.ErrorCanceled, err.Error(), http.StatusGatewayTimeout, req.URL.Path)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		promapi.WriteError(r.logger, w, promapi.ErrorTimeout, err.Error(), http.StatusGatewayTimeout, req.URL.Path)
		return
	}
	promapi.WriteError(r.logger, w, promapi.ErrorInternal, err.Error(), http.StatusInternalServerError, req.URL.Path)
}
