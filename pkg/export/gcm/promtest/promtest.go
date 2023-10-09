package promtest

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/efficientgo/e2e"
	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"github.com/oklog/ulid"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/thanos-io/thanos/pkg/runutil"
)

// GCMServiceAccountOrFail gets the Google SA JSON content from GCM_SECRET
// environment variable or fails.
func GCMServiceAccountOrFail(t testing.TB) []byte {
	// TODO(bwplotka): Move it to https://cloud.google.com/build CI.
	saJSON := []byte(os.Getenv("GCM_SECRET"))
	if len(saJSON) == 0 {
		t.Fatal("newExportGCM: no GCM_SECRET env var provided, can't run the test")
	}
	return saJSON
}

// ingestionTest represents tests that allows validating Prometheus "backend", so
// scraper (agent/collector) and DB that serves PromQL e.g.:
// * export pkg code in this repo and GCM.
// * GMP collector (built export pkg attached to Prometheus fork) for ingestion and querying.
// * GMP collector (built export pkg attached to Prometheus fork) and GCM for querying.
// * Prometheus.
//
// This is essentially a simplified compliance tests focused on GCM case and with
// quick feedback loop (compared to hours with https://github.com/prometheus/compliance/tree/main/promql).
type ingestionTest struct {
	t testing.TB

	testID string

	env e2e.Environment

	backends               map[string]backend
	expectationsPerBackend map[string]model.Matrix

	currTime time.Time
}

type backend struct {
	api     v1.API
	extLset map[string]string
	b       Backend
}

// NewIngestionTest takes testing T and returns test instance as well as registry
// that can be used to register test metrics.
//
// NOTE(bwplotka): All test functionality fails tests on error (instead of returning
// error).
func NewIngestionTest(t testing.TB, backends []Backend) *ingestionTest {
	t.Helper()

	// TODO(bwplotka): Consider lazy creation of docker env?
	e, err := e2e.New()
	t.Cleanup(e.Close)
	if err != nil {
		t.Fatal(err)
	}

	it := &ingestionTest{
		t: t,
		// Use ULID as a unique label per test run.
		// NOTE(bwplotka): Cardinality is obvious, but even for 100 test runs a day with
		// 100 cases, 10 samples each, that's "only" 10k series / 0.1 million samples a day.
		testID: ulid.MustNew(ulid.Now(), rand.New(rand.NewSource(time.Now().UnixNano()))).String(),

		// 1h in the past as Monarch allows 24h, but Prometheus allows 2h (plus some buffer).
		// It should give buffer for a fair amount of samples from -1h to now.
		// TODO(bwplotka): This won't work with GMP collector that has 10m block size.
		// This means when injecting sample we have to wait a bit or move this -1h
		// to -8m rather.
		currTime: time.Now().Add(-1 * time.Hour),

		backends:               map[string]backend{},
		expectationsPerBackend: map[string]model.Matrix{},
	}

	for _, b := range backends {
		api, extLset := b.start(t, e)
		it.backends[b.Ref()] = backend{b: b, api: api, extLset: extLset}
	}
	if len(it.backends) == 0 {
		t.Fatal("no backends specified, at least has to be passed e.g. promtest.Prometheus or promtest.LocalExportWithGCM")
	}

	return it
}

type Backend interface {
	Ref() string
	start(t testing.TB, env e2e.Environment) (api v1.API, extraLset map[string]string)
	injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, timeout time.Duration)
}

type ExpectationsRecorder interface {
	Expect(val float64, metric prometheus.Metric, b Backend) ExpectationsRecorder

	// TODO(bwplotka): Add histogram support.
}

type Scrape func(after time.Duration) ExpectationsRecorder

// RecordScrapes allows recording SDK and expected samples for interesting cases.
func (it *ingestionTest) RecordScrapes(recordingFunc func(r prometheus.Registerer, scrape Scrape)) {
	fmt.Printf("%s Recording scrapes for test=%q\n", it.t.Name(), it.testID)
	r := prometheus.NewRegistry()

	var scrapeRecordings [][]*dto.MetricFamily

	recordingFunc(prometheus.WrapRegistererWith(map[string]string{"test": it.testID}, r), func(after time.Duration) ExpectationsRecorder {
		it.t.Helper()

		if after <= 0 {
			it.t.Fatal(errors.New("scrape 'after' parameter can't be negative or zero"))
		}
		if it.currTime.Add(after).After(time.Now()) {
			it.t.Fatal(errors.New("sum of all scrape 'after' parameters can be beyond 10 hours"))
		}
		it.currTime = it.currTime.Add(after)

		mfs, err := r.Gather()
		if err != nil {
			it.t.Fatal(err)
		}

		// Inject a fixed timestamps as a way to backfill/inject samples with
		// larger (e.g. 30 seconds) intervals without waiting.
		for _, mf := range mfs {
			for _, m := range mf.GetMetric() {
				m.TimestampMs = proto.Int64(timestamp.FromTime(it.currTime))
			}
		}
		scrapeRecordings = append(scrapeRecordings, mfs)

		return &ingestionTestExpRecorder{it: it, currTime: it.currTime}
	})

	// Once recorded, let's inject those to our backends.
	// This might take a while.
	// TODO(bwplotka): Potential for concurrency here.
	for _, b := range it.backends {
		fmt.Printf("%s Injecting samples to %v backend\n", it.t.Name(), b.b.Ref())
		b.b.injectScrapes(it.t, scrapeRecordings, 2*time.Minute)
	}
}

type ingestionTestExpRecorder struct {
	it       *ingestionTest
	currTime time.Time
}

func (ir *ingestionTestExpRecorder) Expect(val float64, metric prometheus.Metric, b Backend) ExpectationsRecorder {
	m := dto.Metric{}
	metric.Write(&m)
	if m.GetSummary() != nil || m.GetHistogram() != nil {
		// TODO(bwplotka): Implement an alternative.
		ir.it.t.Fatal("It's not practical to use equalsGCMPromQuery against histograms and summaries.")
	}

	modelMetric := toModelMetric(metric)
	for _, ss := range ir.it.expectationsPerBackend[b.Ref()] {
		if ss.Metric.Equal(modelMetric) {
			ss.Values = append(ss.Values, model.SamplePair{
				Timestamp: model.TimeFromUnixNano(ir.currTime.UnixNano()),
				Value:     model.SampleValue(val),
			})
			return ir
		}
	}

	// More labels, like external labels (cluster etc.) and test labels will be
	// injected during expectGCMResults call. The same with sorting.
	ir.it.expectationsPerBackend[b.Ref()] = append(ir.it.expectationsPerBackend[b.Ref()], &model.SampleStream{
		Metric: modelMetric,
		Values: []model.SamplePair{{
			Timestamp: model.TimeFromUnixNano(ir.currTime.UnixNano()),
			Value:     model.SampleValue(val),
		}},
	})

	return ir
}

func (it *ingestionTest) preparedExpectedMatrix(exp model.Matrix, metric model.Metric, extLabels map[string]string) model.Matrix {
	m := metric.Clone()
	m["test"] = model.LabelValue(it.testID)
	for k, v := range extLabels {
		m[model.LabelName(k)] = model.LabelValue(v)
	}

	for _, e := range exp {
		if !e.Metric.Equal(metric) {
			continue
		}

		return model.Matrix{{
			Metric: m,
			Values: e.Values,
		}}
	}
	return nil
}

// FatalOnUnexpectedPromQLResults fails the test if gathered expected samples for given non histogram,
// non-summary metric does not match <metric>[2h] samples from given backend PromQL API for
// instant query.
func (it *ingestionTest) FatalOnUnexpectedPromQLResults(b Backend, metric prometheus.Metric, timeout time.Duration) {
	it.t.Helper()

	m := dto.Metric{}
	metric.Write(&m)

	if m.GetSummary() != nil || m.GetHistogram() != nil {
		// TODO(bwplotka): Implement alternative.
		it.t.Fatal("It's not practical to use equalsGCMPromQuery against histograms and summaries.")
	}

	bMeta, ok := it.backends[b.Ref()]
	if !ok {
		it.t.Fatalf("%s backend not seen before? Did you pass it in NewIngestionTest?", b.Ref())
	}

	modelMetric := toModelMetric(metric)
	exp := it.preparedExpectedMatrix(it.expectationsPerBackend[b.Ref()], modelMetric, bMeta.extLset)
	if exp == nil {
		it.t.Fatalf("expected metric %v, not found in expected Matrix. Did you use scrape(...).expect(...) method?", modelMetric.String())
	}

	modelMetric["test"] = model.LabelValue(it.testID)
	query := fmt.Sprintf(`%s[10h]`, modelMetric.String())

	fmt.Printf("%s Checking if PromQL instant query for %v matches expected samples for %v backend\n", it.t.Name(), query, b.Ref())

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	it.t.Cleanup(cancel)

	var lastDiff string
	var sameDiffTimes int
	if err := runutil.RetryWithLog(log.NewJSONLogger(os.Stderr), 10*time.Second, ctx.Done(), func() error {
		value, warns, err := bMeta.api.Query(ctx, query, it.currTime.Add(1*time.Second))
		if err != nil {
			return fmt.Errorf("instant query %s for %v %w", query, it.currTime.Add(1*time.Second), err)
		}
		if len(warns) > 0 {
			fmt.Println(it.t.Name(), "Warnings:", warns)
		}

		if value.Type() != model.ValMatrix {
			return fmt.Errorf("expected matrix, got %v", value.Type())
		}

		if cmp.Equal(exp, value.(model.Matrix)) {
			return nil
		}

		diff := cmp.Diff(exp, value.(model.Matrix))
		if lastDiff == diff {
			if sameDiffTimes > 3 {
				// Likely nothing will change, abort.
				fmt.Println(it.t.Name(), lastDiff)
				it.t.Error(errors.New("resulted Matrix is different than expected (see printed diff)"))
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
			fmt.Println(it.t.Name(), lastDiff)
		}
		it.t.Error(err)
	}
}

func toModelMetric(metric prometheus.Metric) model.Metric {
	m := dto.Metric{}
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
