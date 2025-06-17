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
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/efficientgo/core/runutil"
	"github.com/efficientgo/e2e"
	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/oklog/ulid"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/timestamp"
)

// Backend represents a metric backend pipeline that can be started.
type Backend interface {
	// Ref returns unique representation of the backend.
	Ref() string

	// StartAndWaitReady starts all pieces that has to be started (until ready) to
	// obtain the running backend e.g. docker containers.
	StartAndWaitReady(t testing.TB, env e2e.Environment) RunningBackend
}

// RunningBackend represents a running metric backend, so any
// collection and storage pipeline that serves PromQL e.g.:
// * Prometheus
// * Prometheus + Cortex
// * Prometheus + Thanos
// * Prometheus + vendor
// * otel-col + vendor
// , etc.
type RunningBackend interface {
	// API returns PromQL client connected to the backend.
	API() v1.API
	// CollectionLabels returns any extra labels that are expected to be added
	// by the backend's collection (e.g. job label for Prometheus).
	// This will be added to expected labels when comparing query result with the
	// expectations.
	CollectionLabels() map[string]string

	// IngestSamples ingests the recorded data (series with ordered samples)
	// to the backend so it can be queried later. It's up to the backend how
	// it's ingested e.g.
	// *  NewPrometheusBackend offers both scrape based (via RecordedGatherer
	// with explicit timestamps and OpenMetrics) and Remote Write based flow.
	IngestSamples(ctx context.Context, t testing.TB, recorded [][]*dto.MetricFamily)
}

type ScrapeStyleTest struct {
	t *testing.T

	testID string
	opts   []e2e.EnvironmentOption

	backends               map[string]Backend
	expectationsPerBackend map[string]model.Matrix
	scrapeRecordings       [][]*dto.MetricFamily

	currentTime time.Time
	minTime     time.Time
	maxTime     time.Time

	registry *prometheus.Registry
}

// NewScrapeStyleTest returns test instance that allows scrape style test.
// It allows registering metrics and recording fake scrapes with the expected values.
// That data will be then injected to expected backends and compared with the
// backend QueryInstant `<metric>[10h]` PromQL query.
//
// All test functionality fails tests on error.
// TODO(bwplotka): Consider wrapping e2e.EnvironmentOptions with our own for stability.
func NewScrapeStyleTest(t *testing.T, opts ...e2e.EnvironmentOption) *ScrapeStyleTest {
	t.Helper()

	return &ScrapeStyleTest{
		t:                      t,
		backends:               map[string]Backend{},
		expectationsPerBackend: map[string]model.Matrix{},
		registry:               prometheus.NewRegistry(),
		opts:                   opts,

		// testID is a unique ID for the test. It will also appear as "promqle2e_test" Prometheus
		// label. Cardinality of this is obvious, but even for 100 test runs a day with
		// 100 cases, 10 samples each, that's "only" 10k series / 0.1 million samples a day.
		testID: fmt.Sprintf("%v: %v", t.Name(), ulid.MustNew(ulid.Now(), rand.New(rand.NewSource(time.Now().UnixNano()))).String()),
		// Set the currentTime to the retrospective start time for the ordered, sample data.
		// 1h in the past is typically a good default as e.g.
		// * Prometheus/Thanos/Cortex/Mimir allows generally ~2h (head time)
		// * Google Monarch allows 24h.
		//
		// It should give buffer for a fair amount of samples from -1h to now.
		// -10h is the limit for now, given the hardcoded <metric>[10h] query this framework uses.
		currentTime: time.Now().Add(-1 * time.Hour),
	}
}

// SetCurrentTime allows setting the current timestamp to a new value, which will
// be used on explicit timestamp for the record samples.
// By default, it starts with time.Now()-1h.
func (t *ScrapeStyleTest) SetCurrentTime(currTime time.Time) {
	t.currentTime = currTime
}

func (t *ScrapeStyleTest) Registerer() prometheus.Registerer {
	return prometheus.WrapRegistererWith(map[string]string{"promqle2e_test": t.testID}, t.registry)
}

// RegisterBackends registers new backends for the test.
// This is optional, every backend used in the RecordScrape(...).Expect(...) method
// will be automatically registered.
func (t *ScrapeStyleTest) RegisterBackends(bs ...Backend) {
	t.t.Helper()

	for _, b := range bs {
		_, ok := t.backends[b.Ref()]
		if ok {
			t.t.Fatal("duplicate backend ref found", b.Ref())
		}
		t.backends[b.Ref()] = b
	}
}

// ExpectationsRecorder allows recording results for a scrape.
// TODO(bwplotka): Add histogram support.
type ExpectationsRecorder interface {
	// Expect sets an expectation for metric to have a float value of val for backend b.
	// NOTE(bwplotka): For multiple backends on the same test that reuse the same PromQL storage, CollectionLabels
	// has to be unique.
	Expect(metric prometheus.Metric, val float64, b Backend) ExpectationsRecorder
}

// RecordScrape records a scrape with the chained expectations for a metric and backend.
// The after parameter controls the sample explicit timestamp to use in this scrape.
// See also SetCurrentTime. This allows a limited backfilling without waiting.
//
// Run will replay those scraped record in real scrape interval e.g. 1s for Prometheus
// but explicit timestamp can shape the samples for test purposes.
//
// A good "after" time would be a stable fake "scrape" interval e.g., 15-30 seconds.
//
// 0 or negative after durations (or variations of SetCurrentTime use) can cause backends
// to return "out of order" or "too old samples" errors.
func (t *ScrapeStyleTest) RecordScrape(after time.Duration) ExpectationsRecorder {
	t.t.Helper()

	t.currentTime = t.currentTime.Add(after)
	if !t.minTime.Equal(time.Time{}) {
		if t.currentTime.After(t.minTime.Add(10 * time.Hour)) {
			t.t.Fatalf("after applying duration %q we would end up in the current time %q outside of [%v, %v] spanning over 10h; this will not work with <metric>[10h] query this framework uses", after, t.currentTime, t.minTime, t.maxTime)
		}
	} else {
		t.minTime = t.currentTime
	}
	if !t.maxTime.Equal(time.Time{}) {
		if t.currentTime.Before(t.maxTime.Add(-10 * time.Hour)) {
			t.t.Fatalf("after applying duration %q we would end up in the current time %q outside of [%v, %v] spanning over 10h; this will not work with <metric>[10h] query this framework uses", after, t.currentTime, t.minTime, t.maxTime)
		}
	} else {
		t.maxTime = t.currentTime
	}

	if t.currentTime.After(t.maxTime) {
		t.maxTime = t.currentTime
	}
	if t.currentTime.Before(t.minTime) {
		t.minTime = t.currentTime
	}

	// Get current registry state.
	mfs, err := t.registry.Gather()
	if err != nil {
		t.t.Fatal(err)
	}

	// Inject fake time.
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			m.TimestampMs = proto.Int64(timestamp.FromTime(t.currentTime))
		}
	}

	t.scrapeRecordings = append(t.scrapeRecordings, mfs)
	return t
}

func (t *ScrapeStyleTest) Expect(metric prometheus.Metric, val float64, b Backend) ExpectationsRecorder {
	t.t.Helper()

	m := dto.Metric{}

	_ = metric.Write(&m)
	if m.GetSummary() != nil || m.GetHistogram() != nil {
		// TODO(bwplotka): Implement an alternative.
		t.t.Fatal("It's not practical to use Expect against histograms and summaries.")
	}

	modelMetric := toModelMetric(metric)
	sample := model.SamplePair{
		Timestamp: model.TimeFromUnixNano(t.currentTime.UnixNano()),
		Value:     model.SampleValue(val),
	}

	for _, ss := range t.expectationsPerBackend[b.Ref()] {
		if ss.Metric.Equal(modelMetric) {
			ss.Values = append(ss.Values, sample)
			return t
		}
	}

	// New backend, load dynamically.
	t.backends[b.Ref()] = b
	t.expectationsPerBackend[b.Ref()] = append(t.expectationsPerBackend[b.Ref()], &model.SampleStream{
		Metric: modelMetric,
		Values: []model.SamplePair{sample},
	})
	return t
}

// Transform allows performing any transformations on top of the scrape recorded data e.g. adding/removing CT.
func (t *ScrapeStyleTest) Transform(transformFn func([][]*dto.MetricFamily) [][]*dto.MetricFamily) {
	t.scrapeRecordings = transformFn(t.scrapeRecordings)
}

// Run performs the test, by actually injecting the recorded data and expecting
// the PromQL output based on RecordScrape(...).Expect(...) methods.
// Only once and one of this can be run per NewScrapeStyleTest.
func (t *ScrapeStyleTest) Run(ctx context.Context) {
	tt := t.t
	tt.Helper()

	if len(t.backends) == 0 {
		tt.Fatal("no backends specified, at least has to be registered, either on RecordScrape(...).Expect or on RegisterBackends e.g. promqe2e.PrometheusBackend")
	}

	e, err := e2e.New(t.opts...)
	tt.Cleanup(e.Close)
	if err != nil {
		tt.Fatal(err)
	}

	// Start backends.
	running := map[string]RunningBackend{}
	for _, b := range t.backends {
		running[b.Ref()] = b.StartAndWaitReady(tt, e)
	}

	// Inject recorded data to our backends. This might take a while, depending on the
	// backend implementations.
	for ref, b := range running {
		tt.Run(fmt.Sprintf("backend=%v", ref), func(tt *testing.T) {
			tt.Parallel()

			tLogf(tt, "Injecting samples to backend %q\n", ref)
			b.IngestSamples(ctx, tt, t.scrapeRecordings)

			exp := t.expectationsPerBackend[ref]
			for _, m := range exp {
				metric := m.Metric
				tt.Run(fmt.Sprintf("metric=%v", metric.String()), func(tt *testing.T) {
					tt.Parallel()
					t.fatalOnUnexpectedPromQLResults(ctx, tt, ref, b, m)
				})
			}
		})
	}
}

func tLogf(tt testing.TB, format string, args ...any) {
	// TODO(bwplotka): Is this trully streaming? Consider tee-ing to std as well?
	tt.Logf(format, args...)
}

// fatalOnUnexpectedPromQLResults fails the test if gathered expected samples for given non histogram,
// non-summary metrics does not match <metric>[10h] samples from given backend PromQL API for
// instant query.
func (t *ScrapeStyleTest) fatalOnUnexpectedPromQLResults(ctx context.Context, tt testing.TB, ref string, b RunningBackend, expected *model.SampleStream) {
	tt.Helper()

	expectedMetric := expected.Metric.Clone()
	expectedMetric["promqle2e_test"] = model.LabelValue(t.testID)
	for k, v := range b.CollectionLabels() {
		expectedMetric[model.LabelName(k)] = model.LabelValue(v)
	}
	exp := model.Matrix{{
		Metric: expectedMetric,
		Values: expected.Values,
	}}

	query := fmt.Sprintf(`%s[10h]`, expectedMetric.String())
	tLogf(tt, "Checking if PromQL instant query for %v at %v matches expected samples for %v backend\n", query, t.maxTime, ref)
	var (
		lastDiff      string
		sameDiffTimes int
		got           model.Matrix
	)
	if err := runutil.RetryWithLog(log.NewJSONLogger(os.Stderr), 10*time.Second, ctx.Done(), func() error {
		value, warns, err := b.API().Query(ctx, query, t.maxTime)
		if err != nil {
			return fmt.Errorf("instant query %s at %v: %w", query, t.maxTime, err)
		}
		if len(warns) > 0 {
			tLogf(tt, "Got query warnings: %v\n", warns)
		}

		if value.Type() != model.ValMatrix {
			return fmt.Errorf("expected matrix, got %v", value.Type())
		}

		got = value.(model.Matrix)
		if cmp.Equal(exp, got) {
			return nil
		}

		diff := cmp.Diff(exp, got)
		if exp.Len() > 0 && got.Len() == 0 {
			return errors.New("resulted Matrix is empty, but expected some data")
		}
		if lastDiff == diff {
			if sameDiffTimes > 3 {
				// Likely nothing will change, abort.
				tLogf(tt, "got %v; diff %v\n", got, lastDiff)
				tt.Error(fmt.Errorf("resulted Matrix is different than expected: %v\n", diff))
				return nil
			}
			sameDiffTimes++
		} else {
			lastDiff = diff
			sameDiffTimes = 0
		}
		return errors.New("resulted Matrix is different than expected (diff, if any, will be printed at the end)")
	}); err != nil {
		if lastDiff != "" {
			tLogf(tt, "got %v; diff %v\n", got, lastDiff)
		}
		tt.Error(err)
	}
}

func toModelMetric(metric prometheus.Metric) model.Metric {
	m := dto.Metric{}
	//nolint:errcheck
	metric.Write(&m)

	ret := model.Metric{}
	for _, p := range m.Label {
		ret[model.LabelName(p.GetName())] = model.LabelValue(p.GetValue())
	}

	// Write gives us only labels, not metric name. Since we use internal SDK methods
	// (although public) we have to hack Desc() to get full metric name.
	// TODO(bwplotka): Add public GetName() to Desc.
	name := strings.TrimPrefix(metric.Desc().String(), "Desc{fqName: \"")
	i := strings.Index(name, "\"")

	ret[model.MetricNameLabel] = model.LabelValue(name[:i])
	return ret
}

// recordedGatherer is a prometheus.Gatherer capable to "play" the recorded metric state
// with fixed timestamps to backfill data into Prometheus compatible system.
type recordedGatherer struct {
	i               int
	recordedScrapes [][]*dto.MetricFamily
	mu              sync.Mutex
}

func newRecordedGatherer() *recordedGatherer {
	return &recordedGatherer{i: -1}
}

func (g *recordedGatherer) Gather() (ret []*dto.MetricFamily, _ error) {
	g.mu.Lock()
	ret = nil
	if g.i > -1 && g.i < len(g.recordedScrapes) {
		ret = g.recordedScrapes[g.i]
		g.i++
	}
	g.mu.Unlock()
	return ret, nil
}

// IngestByScrapeReplayer is a goroutine that expose scrape endpoint that will
// replay recorded samples on scrape. Only one scrape can touch this endpoint
// for an accurate recording.
//
// It implements RunningBackend.IngestSamples which injects data to scrape.
// Before or after the replay the scrape target exposes no metrics.
//
// This is essential when testing scraping part of the collectors like Prometheus
// or OpenTelemetry Collector.
type IngestByScrapeReplayer struct {
	g    *recordedGatherer
	port string
}

// StartIngestByScrapeReplayer starts and returns the replayer. It finishes when env is closing.
// See IngestByScrapeReplayer for details.
func StartIngestByScrapeReplayer(t testing.TB, env e2e.Environment) *IngestByScrapeReplayer {
	t.Helper()

	g := newRecordedGatherer()

	// Setup local HTTP server with OpenMetrics /metrics page.
	m := http.NewServeMux()
	m.Handle("/metrics", promhttp.HandlerFor(g, promhttp.HandlerOpts{
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

	// Start the server.
	s := http.Server{Handler: m}
	go func() { _ = s.Serve(list) }()
	env.AddCloser(func() { _ = s.Close() })

	return &IngestByScrapeReplayer{g: g, port: port}
}

// Endpoint returns host:port address where the server listens for scrapes.
func (r *IngestByScrapeReplayer) Endpoint(env e2e.Environment) string {
	return net.JoinHostPort(env.HostAddr(), r.port)
}

func (r *IngestByScrapeReplayer) IngestSamples(ctx context.Context, t testing.TB, recorded [][]*dto.MetricFamily) {
	t.Helper()

	r.g.mu.Lock()
	r.g.i = 0
	r.g.recordedScrapes = recorded
	r.g.mu.Unlock()

	if err := runutil.RetryWithLog(log.NewJSONLogger(os.Stderr), 10*time.Second, ctx.Done(), func() error {
		r.g.mu.Lock()
		iter := r.g.i
		r.g.mu.Unlock()

		if iter < len(r.g.recordedScrapes) {
			return fmt.Errorf("backend didn't scrape the target enough number of times, got %v, expected %v", iter, len(r.g.recordedScrapes))
		}
		return nil
	}); err != nil {
		t.Fatal(t.Name(), err, "within expected time")
	}
}

type runningScrapeReplayBasedBackend struct {
	replayer         *IngestByScrapeReplayer
	collectionLabels map[string]string

	api v1.API
}

// NewRunningScrapeReplayBasedBackend is a helper function for crafting
// RunningBackend from scrape replayer, expected collection labels and Prometheus API client.
func NewRunningScrapeReplayBasedBackend(
		replayer *IngestByScrapeReplayer,
		collectionLabels map[string]string,
		api v1.API,
) RunningBackend {
	return &runningScrapeReplayBasedBackend{
		replayer:         replayer,
		collectionLabels: collectionLabels,
		api:              api,
	}
}

func (b *runningScrapeReplayBasedBackend) API() v1.API {
	return b.api
}

func (b *runningScrapeReplayBasedBackend) CollectionLabels() map[string]string {
	return b.collectionLabels
}

func (b *runningScrapeReplayBasedBackend) IngestSamples(ctx context.Context, t testing.TB, recorded [][]*dto.MetricFamily) {
	b.replayer.IngestSamples(ctx, t, recorded)
}
