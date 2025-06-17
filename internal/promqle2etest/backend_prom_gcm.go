// Copyright 2025 Google LLC
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

package promqle2etest

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/compliance/promqle2e"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var _ promqle2e.Backend = PrometheusForkGCMBackend{}

// GCMServiceAccountOrFail gets the Google SA JSON content from GCM_SECRET
// environment variable or fails.
func GCMServiceAccountOrFail(t testing.TB) []byte {
	saJSON := []byte(os.Getenv("GCM_SECRET"))
	if len(saJSON) == 0 {
		t.Fatal("GCMServiceAccountOrFail: no GCM_SECRET env var provided, can't run the test")
	}
	return saJSON
}

// PrometheusForkGCMBackend represents a Prometheus GMP fork scraping
// metrics and pushing to GCM API for consumption.
// This generally follows https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-unmanaged.
type PrometheusForkGCMBackend struct {
	Image string
	Name  string
	GCMSA []byte
}

func (p PrometheusForkGCMBackend) Ref() string {
	return p.Name
}

// newPrometheus creates a new Prometheus runnable.
func newPrometheus(env e2e.Environment, name string, image string, scrapeTargetAddress string, flagOverride func(dir string) map[string]string) *e2emon.Prometheus {
	ports := map[string]int{"http": 9090}

	f := env.Runnable(name).WithPorts(ports).Future()
	config := fmt.Sprintf(`
global:
  external_labels:
    collector: %v
scrape_configs:
- job_name: 'test'
  scrape_interval: 5s
  scrape_timeout: 5s
  static_configs:
  - targets: [%s]
  metric_relabel_configs:
  - regex: instance
    action: labeldrop
`, name, scrapeTargetAddress)
	if err := os.WriteFile(filepath.Join(f.Dir(), "prometheus.yml"), []byte(config), 0600); err != nil {
		return &e2emon.Prometheus{Runnable: e2e.NewFailedRunnable(name, fmt.Errorf("create prometheus config failed: %w", err))}
	}

	args := map[string]string{
		"--web.listen-address":               fmt.Sprintf(":%d", ports["http"]),
		"--config.file":                      filepath.Join(f.Dir(), "prometheus.yml"),
		"--storage.tsdb.path":                f.Dir(),
		"--enable-feature=exemplar-storage":  "",
		"--enable-feature=native-histograms": "",
		"--storage.tsdb.no-lockfile":         "",
		"--storage.tsdb.retention.time":      "1d",
		"--storage.tsdb.wal-compression":     "",
		"--storage.tsdb.min-block-duration":  "2h",
		"--storage.tsdb.max-block-duration":  "2h",
		"--web.enable-lifecycle":             "",
		"--log.format":                       "json",
		"--log.level":                        "info",
	}
	if flagOverride != nil {
		args = e2e.MergeFlagsWithoutRemovingEmpty(args, flagOverride(f.Dir()))
	}

	p := e2emon.AsInstrumented(f.Init(e2e.StartOptions{
		Image:     image,
		Command:   e2e.NewCommandWithoutEntrypoint("prometheus", e2e.BuildArgs(args)...),
		Readiness: e2e.NewHTTPReadinessProbe("http", "/-/ready", 200, 200),
		User:      strconv.Itoa(os.Getuid()),
	}), "http")

	return &e2emon.Prometheus{
		Runnable:     p,
		Instrumented: p,
	}
}

func (p PrometheusForkGCMBackend) StartAndWaitReady(t testing.TB, env e2e.Environment) promqle2e.RunningBackend {
	t.Helper()

	ctx := t.Context()

	creds, err := google.CredentialsFromJSON(ctx, p.GCMSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		t.Fatalf("create credentials from JSON: %s", err)
	}

	// Fake, does not matter.
	cluster := "pe-github-action"
	location := "europe-west3-a"

	cl, err := api.NewClient(api.Config{
		Address: fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", creds.ProjectID),
		Client:  oauth2.NewClient(ctx, creds.TokenSource),
	})
	if err != nil {
		t.Fatalf("create Prometheus client: %s", err)
	}

	replayer := promqle2e.StartIngestByScrapeReplayer(t, env)
	prom := newPrometheus(env, p.Name, p.Image, replayer.Endpoint(env), func(dir string) map[string]string {
		if err := os.WriteFile(filepath.Join(dir, "gcm-sa.json"), p.GCMSA, 0600); err != nil {
			t.Fatalf("write JSON creds: %s", err)
		}

		// Flags as per https://cloud.google.com/stackdriver/docs/managed-prometheus/setup-unmanaged#gmp-binary.
		return map[string]string{"--export.label.project-id": creds.ProjectID,
			"--export.label.location":   location,
			"--export.label.cluster":    cluster,
			"--export.credentials-file": filepath.Join(dir, "gcm-sa.json"),
		}
	})
	if err := e2e.StartAndWaitReady(prom); err != nil {
		t.Fatal(err)
	}

	return promqle2e.NewRunningScrapeReplayBasedBackend(
		replayer,
		map[string]string{
			"cluster":    cluster,
			"location":   location,
			"project_id": creds.ProjectID,
			"collector":  p.Name,
			"job":        "test",
		},
		v1.NewAPI(cl),
	)
}
