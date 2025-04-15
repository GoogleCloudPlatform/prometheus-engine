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
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/compliance/promqle2e"
	"github.com/stretchr/testify/require"
)

func setupBackends(t testing.TB) (promqle2e.PrometheusBackend, PrometheusForkGCMBackend, LocalExportGCMBackend) {
	// target --PromProto--> Prometheus.
	prom := promqle2e.PrometheusBackend{
		Name:  "prom",
		Image: "quay.io/prometheus/prometheus:v3.2.0",
	}
	// target --PromProto--> Prometheus GMP fork --GCM API--> GCM.
	promForkGCM := PrometheusForkGCMBackend{
		Name:  "prom-fork-gcm",
		Image: "gke.gcr.io/prometheus-engine/prometheus:v2.45.3-gmp.10-gke.0",
		GCMSA: GCMServiceAccountOrFail(t),
	}
	// local prometheus-engine/pkg/export code --GCM API--> GCM.
	localExportGCM := LocalExportGCMBackend{
		Name:  "local-export-gcm",
		GCMSA: GCMServiceAccountOrFail(t),
	}
	return prom, promForkGCM, localExportGCM
}

// TestExportGCM_PrometheusCounter_NoCT tests a basic counter sample behaviour
// with a known CT limitation across 3 ingestion flows:
// * target --PromProto--> Prometheus (referencing, ideal, OSS behaviour).
// * local prometheus-engine/pkg/export code --GCM API--> GCM.
// * target --PromProto--> Prometheus GMP fork --GCM API--> GCM.
//
// The main goal is to have a basic acceptance test on the non-trivial behaviours across multiple ingestion pipelines.
// Currently, this test is for manual run only; to run add GCM_SECRET envvar containing GCM API read and write access (and adjust timeout).
//
// TODO(bwplotka): Add vanilla Prometheus cases which will be possible soon:
// * target --PromProto--> Prometheus vanilla --PRW 2.0--> OpenTelemetry Collector --GCM API--> GCM.
// * target --PromProto--> Prometheus vanilla --PRW 2.0--> GCM.
//
// See also https://github.com/GoogleCloudPlatform/opentelemetry-operations-collector/pull/271 for
// the same test with various OpenTelemetry pipelines e.g.
// * target --PromProto--> OpenTelemetry Collector --GCM API--> GCM.
func TestExportGCM_PrometheusCounter_NoCT(t *testing.T) {
	const interval = 15 * time.Second

	prom, promForkGCM, localExportGCM := setupBackends(t)

	pt := promqle2e.NewScrapeStyleTest(t)
	pt.SetCurrentTime(time.Now().Add(-10 * time.Minute)) // We only do a few scrapes, so -10m buffer is enough.

	//nolint:promlinter // Test metric.
	counter := promauto.With(pt.Registerer()).NewCounterVec(prometheus.CounterOpts{
		Name:        "promqle2e_test_counter_total",
		Help:        "Test counter used by promqle2e test framework for acceptance tests.",
		ConstLabels: map[string]string{"repo": "github.com/GoogleCloudPlatform/prometheus-engine"},
	}, []string{"foo"})
	var c prometheus.Counter

	// No metric expected, counterVec empty.
	pt.RecordScrape(interval)

	c = counter.WithLabelValues("bar")
	c.Add(200)
	pt.RecordScrape(interval).
		Expect(c, 200, prom)
	// Nothing is expected for GCM due to cannibalization required if the target does not emit CT (which this metric does not).
	// See https://cloud.google.com/stackdriver/docs/managed-prometheus/troubleshooting#counter-sums
	// TODO(bwplotka): Fix with b/259261536.

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 10, localExportGCM).
		Expect(c, 10, promForkGCM).
		Expect(c, 210, prom)

	c.Add(40)
	pt.RecordScrape(interval).
		Expect(c, 50, localExportGCM).
		Expect(c, 50, promForkGCM).
		Expect(c, 250, prom)

	// Reset to 0 (simulating instrumentation resetting metric or restarting target).
	counter.Reset()
	c = counter.WithLabelValues("bar")
	pt.RecordScrape(interval).
		// NOTE(bwplotka): This and following discrepancies are expected due to
		// GCM PromQL layer using MQL with delta alignment. What we get as a raw
		// counter is already reset-normalized (b/305901765) (plus cannibalization).
		Expect(c, 50, localExportGCM).
		Expect(c, 50, promForkGCM).
		Expect(c, 0, prom)

	c.Add(150)
	pt.RecordScrape(interval).
		Expect(c, 200, localExportGCM).
		Expect(c, 200, promForkGCM).
		Expect(c, 150, prom)

	// Reset to 0 with addition.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(20)
	pt.RecordScrape(interval).
		Expect(c, 220, localExportGCM).
		Expect(c, 220, promForkGCM).
		Expect(c, 20, prom)

	c.Add(50)
	pt.RecordScrape(interval).
		Expect(c, 270, localExportGCM).
		Expect(c, 270, promForkGCM).
		Expect(c, 70, prom)

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 280, localExportGCM).
		Expect(c, 280, promForkGCM).
		Expect(c, 80, prom)

	// Tricky reset case, unnoticeable reset for Prometheus without created timestamp as well.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(600)
	pt.RecordScrape(interval).
		Expect(c, 800, localExportGCM).
		Expect(c, 800, promForkGCM).
		Expect(c, 600, prom)

	// Prometheus SDK used for replies actually emit CTs.
	// Remove all CTs explicitly to test the logic for non-provided CTs in the Prometheus ecosystem.
	pt.Transform(func(recordings [][]*dto.MetricFamily) [][]*dto.MetricFamily {
		for i := range recordings {
			for j := range recordings[i] {
				for k := range recordings[i][j].GetMetric() {
					if recordings[i][j].Metric[k].GetCounter() == nil {
						t.Fatalf("all recorded metrics should be counters")
					}
					recordings[i][j].Metric[k].Counter.CreatedTimestamp = nil
				}
			}
		}
		return recordings
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	t.Cleanup(cancel)
	pt.Run(ctx)
}

func TestExportGCM_PrometheusCounter_WithCT(t *testing.T) {
	const interval = 15 * time.Second

	prom, promForkGCM, localExportGCM := setupBackends(t)

	pt := promqle2e.NewScrapeStyleTest(t)
	pt.SetCurrentTime(time.Now().Add(-10 * time.Minute)) // We only do a few scrapes, so -10m buffer is enough.

	//nolint:promlinter // Test metric.
	counter := promauto.With(pt.Registerer()).NewCounterVec(prometheus.CounterOpts{
		Name:        "promqle2e_test_counter_total",
		Help:        "Test counter used by promqle2e test framework for acceptance tests.",
		ConstLabels: map[string]string{"repo": "github.com/GoogleCloudPlatform/prometheus-engine"},
	}, []string{"foo"})
	var c prometheus.Counter

	// TODO(bwplotka): Sadly all backends we test don't actually use CT yet, this will change soon, update it.

	// No metric expected, counterVec empty.
	pt.RecordScrape(interval)

	c = counter.WithLabelValues("bar")
	c.Add(200)
	pt.RecordScrape(interval).
		Expect(c, 200, prom)
	// Nothing is expected for GCM due to cannibalization required if the target does not emit CT (which this metric does not).
	// See https://cloud.google.com/stackdriver/docs/managed-prometheus/troubleshooting#counter-sums
	// TODO(bwplotka): Fix with b/259261536.

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 10, localExportGCM).
		Expect(c, 10, promForkGCM).
		Expect(c, 210, prom)

	c.Add(40)
	pt.RecordScrape(interval).
		Expect(c, 50, localExportGCM).
		Expect(c, 50, promForkGCM).
		Expect(c, 250, prom)

	// Reset to 0 (simulating instrumentation resetting metric or restarting target).
	counter.Reset()
	c = counter.WithLabelValues("bar")
	pt.RecordScrape(interval).
		// NOTE(bwplotka): This and following discrepancies are expected due to
		// GCM PromQL layer using MQL with delta alignment. What we get as a raw
		// counter is already reset-normalized (b/305901765) (plus cannibalization).
		Expect(c, 50, localExportGCM).
		Expect(c, 50, promForkGCM).
		Expect(c, 0, prom)

	c.Add(150)
	pt.RecordScrape(interval).
		Expect(c, 200, localExportGCM).
		Expect(c, 200, promForkGCM).
		Expect(c, 150, prom)

	// Reset to 0 with addition.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(20)
	pt.RecordScrape(interval).
		Expect(c, 220, localExportGCM).
		Expect(c, 220, promForkGCM).
		Expect(c, 20, prom)

	c.Add(50)
	pt.RecordScrape(interval).
		Expect(c, 270, localExportGCM).
		Expect(c, 270, promForkGCM).
		Expect(c, 70, prom)

	c.Add(10)
	pt.RecordScrape(interval).
		Expect(c, 280, localExportGCM).
		Expect(c, 280, promForkGCM).
		Expect(c, 80, prom)

	// Tricky reset case, unnoticeable reset for Prometheus without created timestamp as well.
	counter.Reset()
	c = counter.WithLabelValues("bar")
	c.Add(600)
	pt.RecordScrape(interval).
		Expect(c, 800, localExportGCM).
		Expect(c, 800, promForkGCM).
		Expect(c, 600, prom)

	// Prometheus SDK supports CTs. This "transform" validates that invariance.
	pt.Transform(func(recordings [][]*dto.MetricFamily) [][]*dto.MetricFamily {
		for i := range recordings {
			for j := range recordings[i] {
				for k := range recordings[i][j].GetMetric() {
					if recordings[i][j].Metric[k].GetCounter() == nil {
						t.Fatalf("all recorded metrics should be counters")
					}
					require.NotNil(t, recordings[i][j].Metric[k].Counter.CreatedTimestamp)
				}
			}
		}
		return recordings
	})

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	t.Cleanup(cancel)
	pt.Run(ctx)
}
