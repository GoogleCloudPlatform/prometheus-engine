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

var errAllEndpointsFailed = errors.New("all endpoint failed")

// Retriever is an interface for fetching rules and alerts.
type retriever interface {
	RuleGroups(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.RuleGroup, error)
	Alerts(ctx context.Context, baseURL url.URL, queryString string) ([]*promapiv1.Alert, error)
}

// Proxy fan-outs requests to multiple endpoints serving rules and alerts.
// Results are un-sorted and concatenated as-is. In case of errors from any endpoint,
// warning log and partial results are returned.
type Proxy struct {
	logger    log.Logger
	endpoints []url.URL
	client    retriever
}

// NewProxy creates a new proxy.
func NewProxy(logger log.Logger, c httpClient, ruleEndpoints []url.URL) *Proxy {
	return &Proxy{
		logger:    logger,
		endpoints: ruleEndpoints,
		client:    newClient(c),
	}
}

func (p *Proxy) RuleGroups(w http.ResponseWriter, req *http.Request) {
	rules, err := fanoutForward[*promapiv1.RuleGroup](req.Context(), p.logger, p.endpoints, req.URL.RawQuery, p.client.RuleGroups)
	if err != nil {
		p.handleError(w, req, err)
		return
	}

	promapi.WriteSuccessResponse(p.logger, w, http.StatusOK, req.URL.Path, promapi.RulesResponseData{Groups: rules})
}

func (p *Proxy) Alerts(w http.ResponseWriter, req *http.Request) {
	alerts, err := fanoutForward[*promapiv1.Alert](req.Context(), p.logger, p.endpoints, req.URL.RawQuery, p.client.Alerts)
	if err != nil {
		p.handleError(w, req, err)
		return
	}

	promapi.WriteSuccessResponse(p.logger, w, http.StatusOK, req.URL.Path, promapi.AlertsResponseData{Alerts: alerts})
}

// fanoutForward calls the endpoints in parallel and returns the combined results.
func fanoutForward[T *promapiv1.Alert | *promapiv1.RuleGroup](
	ctx context.Context,
	logger log.Logger,
	ruleEndpoints []url.URL,
	rawQuery string,
	retrieveFn func(context.Context, url.URL, string) ([]T, error),
) ([]T, error) {
	if len(ruleEndpoints) == 0 {
		_ = level.Warn(logger).Log("msg", "tried to fetch rules/alerts, no endpoints (--rules.target-urls) configured")
		return []T{}, nil
	}

	var (
		wg                  = sync.WaitGroup{}
		resultChan, errChan = make(chan []T), make(chan error)
		results             []T
		errs                []error
	)

	// Parallel call to all endpoints.
	for _, baseURL := range ruleEndpoints {
		wg.Go(func() {
			result, err := retrieveFn(ctx, baseURL, rawQuery)
			if err != nil {
				errChan <- fmt.Errorf("retrieving alerts from %s failed: %w", baseURL.String(), err)
				return
			}
			resultChan <- result
		})
	}

	go func() {
		// Wait for all rule evaluators to finish and close the channels.
		wg.Wait()
		close(resultChan)
		close(errChan)
	}()

	// Collect results and errors from the channels.
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

	if len(errs) != 0 {
		if len(errs) == len(ruleEndpoints) {
			_ = level.Error(logger).Log("msg", "all endpoints failed", "errors", errs)
			return nil, errAllEndpointsFailed
		}
		_ = level.Warn(logger).Log("msg", "some endpoints failed; potentially partial result", "errors", errs)
	}
	// TODO(bwplotka): Sort?
	return results, nil
}

// handleError writes an error response to the client based on the error.
func (p *Proxy) handleError(w http.ResponseWriter, req *http.Request, err error) {
	if errors.Is(err, context.Canceled) {
		promapi.WriteError(p.logger, w, promapi.ErrorCanceled, err.Error(), http.StatusGatewayTimeout, req.URL.Path)
		return
	}
	if errors.Is(err, context.DeadlineExceeded) {
		promapi.WriteError(p.logger, w, promapi.ErrorTimeout, err.Error(), http.StatusGatewayTimeout, req.URL.Path)
		return
	}
	promapi.WriteError(p.logger, w, promapi.ErrorInternal, err.Error(), http.StatusInternalServerError, req.URL.Path)
}
