// Copyright 2025 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promqle2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

var _ Backend = PrometheusBackend{}

type PrometheusBackendMode = string

const (
	// PrometheusBackendModeOM that uses OpenMetrics recorded scrapes to ingest data.
	PrometheusBackendModeOM = PrometheusBackendMode("om")
	// PrometheusBackendModeRW2 that uses Remote Write 2.0 receiver to ingest data.
	PrometheusBackendModeRW2 = PrometheusBackendMode("prw2")
)

var defaultPrometheusBackend = PrometheusBackend{
	Image: "quay.io/prometheus/prometheus:v3.2.0",
	Name:  "prometheus",
	Mode:  PrometheusBackendModeOM,
}

// PrometheusBackend
// See defaultPrometheusBackend for defaults.
type PrometheusBackend struct {
	Image string
	Name  string
	Mode  PrometheusBackendMode
}

func (opts PrometheusBackend) Ref() string {
	if opts.Name == "" {
		return defaultPrometheusBackend.Name
	}
	return opts.Name
}

func (opts PrometheusBackend) StartAndWaitReady(t testing.TB, env e2e.Environment) RunningBackend {
	if opts.Image == "" {
		opts.Image = defaultPrometheusBackend.Image
	}
	if opts.Name == "" {
		opts.Name = defaultPrometheusBackend.Name
	}
	if opts.Mode == "" {
		opts.Mode = defaultPrometheusBackend.Mode
	}

	switch opts.Mode {
	case PrometheusBackendModeOM:
	case PrometheusBackendModeRW2:
		t.Fatal("not implemeted yet")
	default:
		t.Fatal("unknown mode", opts.Mode)
	}

	replayer := StartIngestByScrapeReplayer(t, env)

	// Create Prometheus container that scrapes our server.
	p := newPrometheus(env, opts.Name, opts.Image, replayer.Endpoint(env), nil)
	if err := e2e.StartAndWaitReady(p); err != nil {
		t.Fatalf("can't start %v: %v", opts.Name, err)
	}

	// Because of scrape config, we expect a job label on top of app labels.
	collectionLabels := map[string]string{"job": "test"}

	cl, err := api.NewClient(api.Config{Address: "http://" + p.Endpoint("http")})
	if err != nil {
		t.Fatalf("failed to create Prometheus client for %v: %s", opts.Name, err)
	}
	return NewRunningScrapeReplayBasedBackend(replayer, collectionLabels, v1.NewAPI(cl))
}

func newPrometheus(env e2e.Environment, name string, image string, scrapeTargetAddress string, flagOverride map[string]string) *e2emon.Prometheus {
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
		args = e2e.MergeFlagsWithoutRemovingEmpty(args, flagOverride)
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
