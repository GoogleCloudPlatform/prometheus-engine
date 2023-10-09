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
		// TODO(bwplotka): Fix with b/259261536.

		c.Add(10)
		scrape(interval).
			Expect(10, c, export).
			Expect(210, c, prom)

		c.Add(50)
		scrape(interval).
			Expect(60, c, export).
			Expect(260, c, prom)

		// Reset to 0, then add something that still should indicate "decreased counter"
		// in absolute values, but our current cannibalization logic is wrong here.
		// It's perhaps unusual that reset happens without scrape target restart
		// so, it was easy to forget about this case.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(250)
		scrape(interval).
			// TODO(bwplotka): Very odd, export logic is fine, added b/305901765
			Expect(310, c, export).
			Expect(250, c, prom)

		c.Add(50)
		scrape(interval).
			Expect(360, c, export).
			Expect(300, c, prom)

		// Reset to 0 again, our export does not detect it again.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(100)
		scrape(interval).
			// TODO(bwplotka): Very odd, export logic is fine, added b/305901765
			Expect(460, c, export).
			Expect(100, c, prom)

		c.Add(50)
		scrape(interval).
			Expect(510, c, export).
			Expect(150, c, prom)

		// Tricky reset case, unnoticeable reset for Prometheus without created timestamp as well.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(600)
		scrape(interval).
			// TODO(bwplotka): Even more odd, I would expect wrong 1110 value, not 960,
			// part of b/305901765
			Expect(960, c, export). // Also something is off, why 960, not 1110?
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
