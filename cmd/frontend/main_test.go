// Copyright 2026 Google LLC
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
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/go-kit/log"
)

type mockRoundTripper struct {
	capturedRequest *http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.capturedRequest = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("mock response")),
	}, nil
}

func TestForward(t *testing.T) {
	logger := log.NewNopLogger()
	targetURL, err := url.Parse("https://monitoring.googleapis.com/v1/projects/my-project/location/global/prometheus")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		requestPath    string
		expectedStatus int
		expectedTarget string // only if expectedStatus is 200.
	}{
		{
			name:           "Normal request",
			requestPath:    "/api/v1/query",
			expectedStatus: http.StatusOK,
			expectedTarget: "https://monitoring.googleapis.com/v1/projects/my-project/location/global/prometheus/api/v1/query",
		},
		{
			name:           "Trailing slash",
			requestPath:    "/api/v1/query/",
			expectedStatus: http.StatusOK,
			expectedTarget: "https://monitoring.googleapis.com/v1/projects/my-project/location/global/prometheus/api/v1/query",
		},
		{
			name:           "Path traversal attempt with unencoded dots",
			requestPath:    "/api/v1/../v1/query",
			expectedStatus: http.StatusOK,
			expectedTarget: "https://monitoring.googleapis.com/v1/projects/my-project/location/global/prometheus/api/v1/query",
		},
		{
			name:           "Redundant slashes",
			requestPath:    "/api/v1//query",
			expectedStatus: http.StatusOK,
			expectedTarget: "https://monitoring.googleapis.com/v1/projects/my-project/location/global/prometheus/api/v1/query",
		},
		{
			name:           "Path traversal attempt with encoded dots",
			requestPath:    "/api/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/%2e%2e/projects/other-project/location/global/prometheus/api/v1/query",
			expectedStatus: http.StatusBadRequest, // Blocked by us.
		},
		{
			name:           "Path traversal attempt with unencoded dots",
			requestPath:    "/api/../../../../../../projects/other-project/location/global/prometheus/api/v1/query",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockRT := &mockRoundTripper{}
			forwardHandler := forward(logger, targetURL, mockRT)

			mux := http.NewServeMux()
			mux.Handle("/api/", forwardHandler)

			server := httptest.NewServer(mux)
			defer server.Close()

			// Custom client that does NOT follow redirects.
			client := &http.Client{}

			u, err := url.Parse(server.URL + tc.requestPath)
			if err != nil {
				t.Fatal(err)
			}
			req, err := http.NewRequest(http.MethodGet, u.String(), nil)
			if err != nil {
				t.Fatal(err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, resp.StatusCode)
			}

			if tc.expectedStatus == http.StatusOK {
				if mockRT.capturedRequest == nil {
					t.Fatal("Expected request to be forwarded, but it was not")
				}
				capturedURL := mockRT.capturedRequest.URL.String()
				if capturedURL != tc.expectedTarget {
					t.Errorf("Expected forwarded URL to be %q, got %q", tc.expectedTarget, capturedURL)
				}
			} else {
				if mockRT.capturedRequest != nil {
					t.Errorf("Expected request to be blocked/redirected, but it was forwarded to %q", mockRT.capturedRequest.URL.String())
				}
			}
		})
	}
}
