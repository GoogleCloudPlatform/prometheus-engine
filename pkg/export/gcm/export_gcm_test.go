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
			Expect(0, c, export).
			Expect(0, c, prom)

		c.Add(150)
		scrape(interval).
			Expect(150, c, export).
			Expect(150, c, prom)

		// Reset to 0 with addition.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(20)
		scrape(interval).
			Expect(20, c, export).
			Expect(20, c, prom)

		c.Add(50)
		scrape(interval).
			Expect(70, c, export).
			Expect(70, c, prom)

		c.Add(10)
		scrape(interval).
			Expect(80, c, export).
			Expect(80, c, prom)

		// Tricky reset case, unnoticeable reset for Prometheus without created timestamp as well.
		counter.Reset()
		c = counter.WithLabelValues("bar")
		c.Add(600)
		scrape(interval).
			Expect(600, c, export).
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
