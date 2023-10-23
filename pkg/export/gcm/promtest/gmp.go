package promtest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/efficientgo/e2e"
	e2emon "github.com/efficientgo/e2e/monitoring"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/thanos-io/thanos/pkg/runutil"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type gmpBackend struct {
	image string
	gcmSA []byte

	g *recordGatherer
	p *e2emon.Prometheus
}

// GMPWithGCM returns a Prometheus fork backend that scrapes test samples
// with speed of 0.20 sample per second, allows 2h data backfill and ships
// data to GCM which are then available for query within Prometheus API.
// TODO(bwplotka): Delete once Prometheus is able to ship to GCM (:
func GMPWithGCM(image string, gcmSA []byte) Backend {
	return &gmpBackend{image: image, gcmSA: gcmSA}
}

func (g *gmpBackend) Ref() string { return "gmp" }

func (g *gmpBackend) start(t testing.TB, env e2e.Environment) (v1.API, map[string]string) {
	t.Helper()

	const name = "gmp"

	m := http.NewServeMux()
	g.g = &recordGatherer{i: -1}
	m.Handle("/metrics", promhttp.HandlerFor(g.g, promhttp.HandlerOpts{
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

	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, g.gcmSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		t.Fatal(err)
	}

	cl, err := api.NewClient(api.Config{
		Address: fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", creds.ProjectID),
		Client:  oauth2.NewClient(ctx, creds.TokenSource),
	})
	if err != nil {
		t.Fatalf("create Prometheus client againt GCM: %s", err)
	}

	g.p = newCollector(env, name, g.image, net.JoinHostPort(env.HostAddr(), port), g.gcmSA, creds.ProjectID, nil)
	if err := e2e.StartAndWaitReady(g.p); err != nil {
		t.Fatalf("can't start Prometheus: %v", err)
	}

	return v1.NewAPI(cl), map[string]string{
		"namespace":  "gmp",
		"job":        "test",
		"cluster":    gcmCluster,
		"location":   gcmLocation,
		"project_id": creds.ProjectID,
	}
}

func newCollector(env e2e.Environment, name string, image string, scrapeTargetAddress string, gcmSA []byte, gcmProjID string, flagOverride map[string]string) *e2emon.Prometheus {
	if image == "" {
		image = "gke.gcr.io/prometheus-engine/prometheus:v2.41.0-gmp.5-gke.0"
	}
	ports := map[string]int{"http": 9090}

	f := env.Runnable(name).WithPorts(ports).Future()
	// TODO(bwplotka): We can make scrape faster due to "One or more TimeSeries could not be written: One or more points were written more frequently than the maximum sampling period configured for the metric.". We could fix it to speed up tests.
	config := fmt.Sprintf(`
global:
  external_labels:
    namespace: %v
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
	if err := os.WriteFile(filepath.Join(f.Dir(), "creds.json"), gcmSA, 0600); err != nil {
		return &e2emon.Prometheus{Runnable: e2e.NewFailedRunnable(name, fmt.Errorf("create cred file failed: %w", err))}
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
		"--export.user-agent-mode":           "unspecified",
		"--export.credentials-file":          filepath.Join(f.Dir(), "creds.json"),
		"--export.label.project-id":          gcmProjID,
		"--export.label.location":            gcmLocation,
		"--export.label.cluster":             gcmCluster,
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

func (g *gmpBackend) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration) {
	t.Helper()

	g.g.mu.Lock()
	g.g.i = 0
	g.g.plannedScrapes = scrapeRecordings
	g.g.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)

	if err := runutil.RetryWithLog(log.NewJSONLogger(os.Stderr), 10*time.Second, ctx.Done(), func() error {
		g.g.mu.Lock()
		iter := g.g.i
		g.g.mu.Unlock()

		if iter < len(g.g.plannedScrapes) {
			return fmt.Errorf("backend %v didn't scrape the target enough number of times, got %v, expected %v", g.Ref(), iter, len(g.g.plannedScrapes))
		}
		return nil
	}); err != nil {
		t.Fatal(t.Name(), err, "within expected time", timeout)
	}
}
