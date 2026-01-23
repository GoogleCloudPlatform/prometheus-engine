// Copyright 2025 Google LLC
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

//go:build image

package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/stretchr/testify/require"
)

const frontendImage = "docker.io/gmp/frontend"

// gcmServiceAccountOrFail gets the Google SA JSON content from GCM_SECRET
// environment variable or fails.
func gcmServiceAccountOrFail(t testing.TB) []byte {
	saJSON := []byte(os.Getenv("GCM_SECRET"))
	if len(saJSON) == 0 {
		t.Fatal("gcmServiceAccountOrFail: no GCM_SECRET env var provided, can't run the test")
	}
	return saJSON
}

func newFrontendContainer(e e2e.Environment, name string, gcmSA []byte) *e2emon.InstrumentedRunnable {
	ports := map[string]int{"http": 9090}

	f := e.Runnable(name).WithPorts(ports).Future()

	credsFile := filepath.Join(f.Dir(), "gcm.json")
	if err := os.WriteFile(credsFile, gcmSA, os.ModePerm); err != nil {
		return e2emon.AsInstrumented(e2e.NewFailedRunnable(name, err), "")
	}

	args := map[string]string{
		"--query.project-id":       "some project",
		"--query.credentials-file": credsFile,
		"--web.listen-address":     fmt.Sprintf(":%d", ports["http"]),
	}
	return e2emon.AsInstrumented(f.Init(e2e.StartOptions{
		Image:     frontendImage,
		Command:   e2e.NewCommand("", e2e.BuildArgs(args)...),
		Readiness: e2e.NewHTTPReadinessProbe("http", "/-/ready", 200, 200),
		User:      strconv.Itoa(os.Getuid()),
	}), "http")
}

// Regression test against https://github.com/GoogleCloudPlatform/prometheus-engine/issues/1806.
// This tests assumes it was invoked by `make image-test` that depends on `make frontend`.
// It requires GCM_SECRET envvar.
func TestFrontend_Image_UIServed(t *testing.T) {
	if os.Getenv("GCM_SECRET") == "" {
		t.Skip("This test requires GCM_SECRET; on CI only maintainers' PRs have it enabled")
	}

	e, err := e2e.New()
	require.NoError(t, err)
	t.Cleanup(e.Close)

	f := newFrontendContainer(e, "frontend1", gcmServiceAccountOrFail(t))
	require.NoError(t, e2e.StartAndWaitReady(f))

	for _, paths := range []string{
		"", // Root.
		"/graph",
	} {
		url := fmt.Sprintf("http://%v%v", f.Endpoint("http"), paths)
		t.Run(fmt.Sprintf("url=%v", url), func(t *testing.T) {
			req, err := http.NewRequest(http.MethodGet, url, http.NoBody)
			// Accept HTML, simulating browser.
			req.Header.Set("Accept", `text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7`)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)

			// Discard body, we don't need it.
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()

			// Ensure HTML response.
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "text/html; charset=utf-8", resp.Header.Get("Content-Type"))
		})
	}
}
