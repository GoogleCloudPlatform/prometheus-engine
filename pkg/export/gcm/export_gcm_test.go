package gcm_test

import (
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export/gcm/promtest"
	"github.com/prometheus/client_golang/prometheus"
)

func TestExport_CounterReset(t *testing.T) {
	const interval = 30 * time.Second

	prom := promtest.Prometheus("quay.io/prometheus/prometheus:v2.47.2")
	export := promtest.LocalExportWithGCM(promtest.GCMServiceAccountOrFail(t))

	it := promtest.NewIngestionTest(t, []promtest.Backend{prom, export})

	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pe_test_counter_total",
		Help: "Test counter used by prometheus-engine export GCM acceptance tests.",
	}, []string{"foo"})
	var c prometheus.Counter

	it.RecordScrapes(func(r prometheus.Registerer, scrape promtest.Scrape) {
		r.MustRegister(counter)

		// No metric.
		scrape(interval)

		c = counter.WithLabelValues("bar")
		c.Add(200)

		scrape(interval).
			Expect(200, c, prom)
		// Nothing is expected for GMP due to cannibalization.
		// See https://cloud.google.com/stackdriver/docs/managed-prometheus/troubleshooting#counter-sums
		// TODO(bwplotka): Fix with b/259261536.

		c.Add(10)
		scrape(interval).
			Expect(10, c, export).
			Expect(210, c, prom)

		c.Add(40)
		scrape(interval).
			Expect(50, c, export).
			Expect(250, c, prom)

		// Reset to 0 (simulating instrumentation resetting metric or restarting target).
		counter.Reset()
		c = counter.WithLabelValues("bar")
		scrape(interval).
			// NOTE(bwplotka): This and following discrepancies are expected due to
			// GCM PromQL layer using MQL with delta alignment. What we get as a raw
			// counter is already reset-normalized (b/305901765) (plus cannibalization).
			Expect(50, c, export).
			Expect(0, c, prom)

		c.Add(150)
		scrape(interval).
			Expect(200, c, export).
			Expect(150, c, prom)

		// Reset to 0 with addition.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(20)
		scrape(interval).
			Expect(220, c, export).
			Expect(20, c, prom)

		c.Add(50)
		scrape(interval).
			Expect(270, c, export).
			Expect(70, c, prom)

		c.Add(10)
		scrape(interval).
			Expect(280, c, export).
			Expect(80, c, prom)

		// Tricky reset case, unnoticeable reset for Prometheus without created timestamp as well.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(600)
		scrape(interval).
			Expect(800, c, export).
			Expect(600, c, prom)
	})

	// Expect what we set we want for each backend.
	for _, b := range []promtest.Backend{export, prom} {
		b := b
		t.Run(b.Ref(), func(t *testing.T) {
			t.Parallel()

			it.FatalOnUnexpectedPromQLResults(b, c, 2*time.Minute)
		})
	}
}
