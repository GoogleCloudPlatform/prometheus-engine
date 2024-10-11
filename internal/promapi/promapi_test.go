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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/require"
)

type recursiveStruct struct {
	Recursive *recursiveStruct `json:"recursive"`
}

func Test_writeResponse(t *testing.T) {
	t.Parallel()

	recursor := &recursiveStruct{}
	recursor.Recursive = recursor

	type testCase struct {
		name             string
		httpResponseCode int
		resp             Response[GenericResponseData]
		wantBody         string
		wantStatus       int
	}
	tests := []testCase{
		{
			name:             "happy path rulesResponseData",
			httpResponseCode: http.StatusOK,
			resp: Response[GenericResponseData]{
				Data: RulesResponseData{Groups: []*promapiv1.RuleGroup{}},
			},
			wantBody:   `{"status":"","data":{"groups":[]}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:             "happy path alertsResponseData",
			httpResponseCode: http.StatusOK,
			resp: Response[GenericResponseData]{
				Data: AlertsResponseData{Alerts: []*promapiv1.Alert{}},
			},
			wantBody:   `{"status":"","data":{"alerts":[]}}`,
			wantStatus: http.StatusOK,
		},
		{
			name:             "happy path string response data",
			httpResponseCode: http.StatusOK,
			resp: Response[GenericResponseData]{
				Data: "foo bar baz qux",
			},
			wantBody:   `{"status":"","data":"foo bar baz qux"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:             "json marshalling error returns prom-api compatible error",
			httpResponseCode: http.StatusOK,
			resp: Response[GenericResponseData]{
				Data: recursor, // recursive struct will cause json marshalling error
			},
			wantBody:   `{"status":"error","ErrorType":"internal","error":"failed to marshal Response"}`,
			wantStatus: http.StatusInternalServerError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			writeResponse[GenericResponseData](log.NewNopLogger(), recorder, tt.httpResponseCode, "", tt.resp)
			require.JSONEq(t, tt.wantBody, recorder.Body.String())
			require.Equal(t, tt.wantStatus, recorder.Code)
		})
	}
}

func TestWriteSuccessResponse(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name             string
		httpResponseCode int
		responseData     RulesResponseData
		wantBody         string
		wantStatus       int
	}
	tests := []testCase{
		{
			name:             "happy path",
			httpResponseCode: http.StatusOK,
			responseData:     RulesResponseData{Groups: []*promapiv1.RuleGroup{}},
			wantBody:         `{"status":"success","data":{"groups":[]}}`,
			wantStatus:       http.StatusOK,
		},
		{
			name:             "empty responseData",
			httpResponseCode: http.StatusOK,
			responseData:     RulesResponseData{},
			wantBody:         `{"status":"success","data":{"groups":null}}`,
			wantStatus:       http.StatusOK,
		},
		{
			name:             "adheres to status code",
			httpResponseCode: http.StatusTeapot,
			responseData:     RulesResponseData{},
			wantBody:         `{"status":"success","data":{"groups":null}}`,
			wantStatus:       http.StatusTeapot,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			WriteSuccessResponse(log.NewNopLogger(), recorder, tt.httpResponseCode, "", tt.responseData)

			require.JSONEq(t, tt.wantBody, recorder.Body.String())
			require.Equal(t, tt.wantStatus, recorder.Code)
		})
	}
}

func TestWriteErrorResponse(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name             string
		httpResponseCode int
		errType          ErrorType
		errMsg           string
		wantBody         string
		wantStatus       int
	}
	tests := []testCase{
		{
			name:             "happy path",
			httpResponseCode: http.StatusInternalServerError,
			errType:          ErrorInternal,
			errMsg:           "foo error message",
			wantBody:         `{"status":"error","errorType":"internal","error":"foo error message"}`,
			wantStatus:       http.StatusInternalServerError,
		},
		{
			name:             "empty responseData",
			httpResponseCode: http.StatusOK,
			errType:          ErrorNone,
			errMsg:           "bar error message",
			wantBody:         `{"status":"error","error":"bar error message"}`,
			wantStatus:       http.StatusOK,
		},
		{
			name:             "adheres to status code",
			httpResponseCode: http.StatusTeapot,
			errType:          ErrorTimeout,
			errMsg:           "baz error message",
			wantBody:         `{"status":"error","errorType":"timeout","error":"baz error message"}`,
			wantStatus:       http.StatusTeapot,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			recorder := httptest.NewRecorder()
			WriteError(log.NewNopLogger(), recorder, tt.errType, tt.errMsg, tt.httpResponseCode, "")

			//require.JSONEq(t, tt.wantBody, recorder.Body.String())
			require.Equal(t, tt.wantBody, recorder.Body.String())
			require.Equal(t, tt.wantStatus, recorder.Code)
		})
	}
}
