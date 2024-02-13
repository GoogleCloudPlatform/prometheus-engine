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

package promtest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/thanos-io/thanos/pkg/runutil"
)

type promBackend struct {
	image string

	g *recordGatherer
	p *e2emon.Prometheus
}

func Prometheus(image string) Backend {
	return &promBackend{image: image}
}

func (p *promBackend) Ref() string { return "prometheus" }

// recordGatherer is a prometheus.Gatherer capable to "play" the recorded metric state
// with fixed timestamps to backfill data into Prometheus compatible system.
type recordGatherer struct {
	i              int
	plannedScrapes [][]*dto.MetricFamily
	mu             sync.Mutex
}

func (g *recordGatherer) Gather() (ret []*dto.MetricFamily, _ error) {
	g.mu.Lock()
	ret = nil
	if g.i > -1 && g.i < len(g.plannedScrapes) {
		ret = g.plannedScrapes[g.i]
		g.i++
	}
	g.mu.Unlock()

	return ret, nil
}

func (p *promBackend) start(t testing.TB, env e2e.Environment) (v1.API, map[string]string) {
	t.Helper()

	const name = "prometheus"

	m := http.NewServeMux()
	p.g = &recordGatherer{i: -1}
	m.Handle("/metrics", promhttp.HandlerFor(p.g, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	// Listen on all addresses, since we need to connect to it from docker container.
	list, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}

	_, port, err := net.SplitHostPort(list.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	s := http.Server{Handler: m}

	go func() { _ = s.Serve(list) }()
	env.AddCloser(func() { _ = s.Close() })

	p.p = newPrometheus(env, name, p.image, net.JoinHostPort(env.HostAddr(), port), nil)
	if err := e2e.StartAndWaitReady(p.p); err != nil {
		t.Fatalf("can't start Prometheus: %v", err)
	}

	cl, err := api.NewClient(api.Config{Address: "http://" + p.p.Endpoint("http")})
	if err != nil {
		t.Fatalf("create Prometheus client: %s", err)
	}

	return v1.NewAPI(cl), map[string]string{"job": "test"}
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

func (p *promBackend) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration) {
	t.Helper()

	p.g.mu.Lock()
	p.g.i = 0
	p.g.plannedScrapes = scrapeRecordings
	p.g.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)

	if err := runutil.RetryWithLog(log.NewJSONLogger(os.Stderr), 10*time.Second, ctx.Done(), func() error {
		p.g.mu.Lock()
		iter := p.g.i
		p.g.mu.Unlock()

		if iter < len(p.g.plannedScrapes) {
			return fmt.Errorf("backend didn't scrape the target enough number of times, got %v, expected %v", iter, len(p.g.plannedScrapes))
		}
		return nil
	}); err != nil {
		t.Fatal(t.Name(), err, "within expected time", timeout)
	}
}
