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

	//nolint:promlinter // Test metric.
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
		t.Run(b.Ref(), func(t *testing.T) {
			t.Parallel()

			it.FatalOnUnexpectedPromQLResults(b, c, 2*time.Minute)
		})
	}
}

func TestExport(t *testing.T) {
	const interval = 30 * time.Second

	prom := promtest.Prometheus("quay.io/prometheus/prometheus:v2.47.2")
	// TODO(bwplotka): Take upstream once https://github.com/prometheus/prometheus/pull/14395
	// is merged. Locally built image for now.
	promViaProxy := promtest.PrometheusViaProxyWithGCM("prometheus:v2.54-dev-prw2.0-rc.1", "prom2gcm:v0.14.0-dev1", promtest.GCMServiceAccountOrFail(t))
	export := promtest.LocalExportWithGCM(promtest.GCMServiceAccountOrFail(t))

	it := promtest.NewIngestionTest(t, []promtest.Backend{prom, promViaProxy, export})

	// TODO(bwplotka): Add more metric types.

	//nolint:promlinter // Test metric.
	gauge := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "pe_test_gauge",
		Help: "Test gauge used by prometheus-engine export GCM acceptance tests.",
	}, []string{"foo"})
	var g prometheus.Gauge

	//nolint:promlinter // Test metric.
	counter := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "pe_test_counter_total",
		Help: "Test counter used by prometheus-engine export GCM acceptance tests.",
	}, []string{"foo"})
	var c prometheus.Counter

	it.RecordScrapes(func(r prometheus.Registerer, scrape promtest.Scrape) {
		r.MustRegister(gauge, counter)

		// No metric.
		scrape(interval)

		g = gauge.WithLabelValues("bar1")
		g.Set(124.2)
		c = counter.WithLabelValues("bar2")
		c.Add(200)

		scrape(interval).
			Expect(124.2, g, prom).
			Expect(124.2, g, promViaProxy).
			Expect(124.2, g, export).
			Expect(200, c, prom)
		// promViaProxy: For c nothing is expected due to Prometheus not setting CT currently.
		// export: For c nothing is expected for GMP due to cannibalization.
		// See https://cloud.google.com/stackdriver/docs/managed-prometheus/troubleshooting#counter-sums
		// TODO(bwplotka): Fix with b/259261536.

		g.Set(-29991.1214)
		c.Add(10)

		scrape(interval).
			Expect(-29991.1214, g, prom).
			Expect(-29991.1214, g, promViaProxy).
			Expect(-29991.1214, g, export).
			Expect(10, c, export).
			Expect(210, c, prom)
		// promViaProxy: For c nothing is expected due to Prometheus not setting CT currently.
	})

	// Expect what we set we want for each backend.
	for _, b := range []promtest.Backend{export, promViaProxy, prom} {
		t.Run(b.Ref(), func(t *testing.T) {
			t.Parallel()

			it.FatalOnUnexpectedPromQLResults(b, g, 2*time.Minute)
			it.FatalOnUnexpectedPromQLResults(b, c, 2*time.Minute)
		})
	}
}
