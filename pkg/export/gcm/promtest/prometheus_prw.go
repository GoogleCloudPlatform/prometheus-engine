package promtest

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
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

type promViaProxyBackend struct {
	image      string
	proxyImage string
	gcmSA      []byte

	g *recordGatherer
	p *e2emon.Prometheus
}

// PrometheusViaProxyWithGCM returns a backend with:
// * OSS Prometheus to scrape test samples and using PRW 2.0 to stream data.
// * prw2gcm proxy to convert PRW 2.0 and send to GCM (GCM does not support PRW yet).
// * GCM used for querying.
//
// Samples are scraped with speed of 0.20 sample per second, which allows 2h
// data backfill in GCM.
func PrometheusViaProxyWithGCM(image string, proxyImage string, gcmSA []byte) Backend {
	return &promViaProxyBackend{image: image, proxyImage: proxyImage, gcmSA: gcmSA}
}

func (p *promViaProxyBackend) Ref() string { return "prometheus-prw2gcm" }

func (p *promViaProxyBackend) start(t testing.TB, env e2e.Environment) (v1.API, map[string]string) {
	t.Helper()

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

	ctx := context.Background()
	creds, err := google.CredentialsFromJSON(ctx, p.gcmSA, gcm.DefaultAuthScopes()...)
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

	p.p = newPrometheus(env, p.Ref(), p.image, net.JoinHostPort(env.HostAddr(), port), nil)
	prw2gcm := newPRW2GCMProxy(env, p.Ref()+"-proxy", p.proxyImage, p.gcmSA)
	if err := e2e.StartAndWaitReady(p.p, prw2gcm); err != nil {
		t.Fatalf("can't start Prometheus and/or prw2gcm proxy: %v", err)
	}

	return v1.NewAPI(cl), map[string]string{
		"namespace":  p.Ref() + "-proxy",
		"job":        "test",
		"project_id": creds.ProjectID,
	}
}

func newPRW2GCMProxy(env e2e.Environment, name string, image string, gcmSA []byte) e2e.Runnable {
	ports := map[string]int{"http": 9091}

	f := env.Runnable(name).WithPorts(ports).Future()
	if err := os.WriteFile(filepath.Join(f.Dir(), "gcm-sa.json"), gcmSA, 0600); err != nil {
		return &e2emon.Prometheus{Runnable: e2e.NewFailedRunnable(name, fmt.Errorf("create prometheus config failed: %w", err))}
	}

	args := map[string]string{
		"-listen-address":                  fmt.Sprintf(":%d", ports["http"]),
		"-gcm.credentials-file":            filepath.Join(f.Dir(), "gcm-sa.json"),
		"-unsafe.allow-classic-histograms": "",
		"--log.format":                     "json",
		"--log.level":                      "info",
	}

	bArgs := e2e.BuildArgs(args)
	return e2emon.AsInstrumented(f.Init(e2e.StartOptions{
		Image:     image,
		Command:   e2e.NewCommand(bArgs[0], bArgs[1:]...),
		Readiness: e2e.NewHTTPReadinessProbe("http", "/-/ready", 200, 200),
		//User:      strconv.Itoa(os.Getuid()),
	}), "http")
}

func (p *promViaProxyBackend) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration) {
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
			return fmt.Errorf("backend %v didn't scrape the target enough number of times, got %v, expected %v", p.Ref(), iter, len(p.g.plannedScrapes))
		}
		return nil
	}); err != nil {
		t.Fatal(t.Name(), err, "within expected time", timeout)
	}
}
