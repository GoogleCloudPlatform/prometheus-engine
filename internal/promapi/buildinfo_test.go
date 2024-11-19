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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-kit/log"
	promapiv1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/stretchr/testify/require"
)

func TestBuildinfoHandlerFunc(t *testing.T) {
	t.Parallel()

	handleFunc := BuildinfoHandlerFunc(log.NewNopLogger(), "frontend", "v1.2.3")
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status/buildinfo", nil)

	handleFunc(recorder, req)
	require.Equal(t, http.StatusOK, recorder.Code)

	// unmarshal into promapiv1.PrometheusVersion object
	resp := Response[promapiv1.PrometheusVersion]{}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &resp))
	defer recorder.Result().Body.Close()

	require.Equal(t, exposedVersion, resp.Data.Version)
	require.Equal(t, "gmp/frontend-v1.2.3", resp.Data.Revision)
}
