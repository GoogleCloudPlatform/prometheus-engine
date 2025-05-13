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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/efficientgo/e2e"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"golang.org/x/oauth2"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/go-kit/log"
	"golang.org/x/oauth2/google"
)

type localExportWithGCM struct {
	gcmSA []byte

	e *export.Exporter
	// NOTE(bwplotka): Not guarded by mutex, so it has to be synced with Exporter.Export.
	labelsByRef map[storage.SeriesRef]labels.Labels
}

// LocalExportWithGCM represents locally imported export pkg with GCM as a
// backend. In particular this backend is mimicking our GMP collector, while
// allowing quickest feedback loop for experiment and using IDE debuggers.
//
// In particular this backend uses following data models, which is what our
// Prometheus fork is doing when scraping any scrape target:
//
// * (Exposed to user) Go Prometheus SDK client types e.g. prometheus.NewCounter.
// * Go Prometheus SDK dto (Prometheus proto exposition format https://github.com/prometheus/client_model/blob/master/io/prometheus/client/metrics.proto).
// * Internal Prometheus parser (https://pkg.go.dev/github.com/prometheus/prometheus/pkg/textparse).
// * Internal Prometheus TSDB (head block) dto (e.g. records https://pkg.go.dev/github.com/prometheus/prometheus@v0.47.2/tsdb/record).
// * GCM monitoring API proto ingested by Monarch.
func LocalExportWithGCM(gcmSA []byte) Backend {
	return &localExportWithGCM{gcmSA: gcmSA}
}

func (l *localExportWithGCM) Ref() string { return "export-pkg-with-gcm" }

func (l *localExportWithGCM) start(t testing.TB, _ e2e.Environment) (v1.API, map[string]string) {
	t.Helper()

	ctx, cancel := context.WithCancel(signals.SetupSignalHandler())
	t.Cleanup(cancel)

	creds, err := google.CredentialsFromJSON(ctx, l.gcmSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		t.Fatalf("create credentials from JSON: %s", err)
	}

	l.labelsByRef = map[storage.SeriesRef]labels.Labels{}

	cluster := "pe-github-action"
	location := "europe-west3-a"

	cl, err := api.NewClient(api.Config{
		Address: fmt.Sprintf("https://monitoring.googleapis.com/v1/projects/%s/location/global/prometheus", creds.ProjectID),
		Client:  oauth2.NewClient(ctx, creds.TokenSource),
	})
	if err != nil {
		t.Fatalf("create Prometheus client: %s", err)
	}

	exporterOpts := export.ExporterOpts{
		UserAgentEnv:        "pe-github-action-test",
		Cluster:             cluster,
		Location:            location,
		ProjectID:           creds.ProjectID,
		CredentialsFromJSON: l.gcmSA,
	}
	exporterOpts.DefaultUnsetFields()
	l.e, err = export.New(ctx, log.NewJSONLogger(os.Stderr), nil, exporterOpts, export.NopLease())
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}

	// Apply empty config, so resources labels are attached.
	if err := l.e.ApplyConfig(&config.DefaultConfig, nil); err != nil {
		t.Fatalf("apply config: %v", err)
	}
	l.e.SetLabelsByIDFunc(func(ref storage.SeriesRef) labels.Labels {
		return l.labelsByRef[ref]
	})

	go func() {
		if err := l.e.Run(); err != nil {
			t.Logf("running exporter: %s", err)
		}
	}()

	return v1.NewAPI(cl), map[string]string{
		"cluster":    cluster,
		"location":   location,
		"project_id": creds.ProjectID,
	}
}

func (l *localExportWithGCM) injectScrapes(t testing.TB, scrapeRecordings [][]*dto.MetricFamily, _ time.Duration) {
	t.Helper()

	for _, mfs := range scrapeRecordings {
		// Encode gathered metric family as proto Prometheus exposition format, decode as internal
		// Prometheus textparse format to have metrics how Prometheus would have
		// before append. We don't use dto straight away due to quite complex code
		// for generating multi counter metrics like legacy histograms and summaries.

		b := bytes.Buffer{}
		enc := expfmt.NewEncoder(&b, expfmt.NewFormat(expfmt.TypeProtoDelim))
		for _, mf := range mfs {
			if err := enc.Encode(mf); err != nil {
				t.Fatal(err)
			}
		}
		if closer, ok := enc.(expfmt.Closer); ok {
			if err := closer.Close(); err != nil {
				t.Fatal(err)
			}
		}
		tp, err := textparse.New(b.Bytes(), string(expfmt.NewFormat(expfmt.TypeProtoDelim)), true)
		if err != nil {
			t.Fatal(err)
		}

		// Iterate over textparse parser results and mimic Prometheus scrape loop
		// with exporter.Export injection.

		// It's fine to start ref from 0 and clean labelsByRef for every Export invocation,
		// as exporter does not need to further (after conversions).
		l.labelsByRef = map[storage.SeriesRef]labels.Labels{}
		ref := uint64(0)

		var (
			currMeta export.MetricMetadata
			batch    []record.RefSample
			metadata = map[string]export.MetricMetadata{}
		)

		for {
			et, err := tp.Next()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				t.Fatal(err)
			}

			switch et {
			case textparse.EntryType:
				_, currType := tp.Type()
				// TODO(pintohutch): this conversion is a temporary hack until
				// we bump prometheus to bring in: https://github.com/prometheus/common/commit/445c21097872f2b117863ee4667c32075a8a5291
				currMeta.Type = model.MetricType(string(currType))
				continue
			case textparse.EntryHelp:
				mName, mHelp := tp.Help()
				currMeta.Metric, currMeta.Help = string(mName), string(mHelp)
				continue
			case textparse.EntryUnit:
				// Proto format won't give us that anyway.
				continue
			case textparse.EntryComment:
				continue
			case textparse.EntryHistogram:
				// TODO(bwplotka): Sparse histogram would be here TBD.
				panic("not implemented")
			default:
			}

			// TODO(bwplotka): Support exemplars and created timestamp.
			t := timestamp.FromTime(time.Now())
			_, parsedTimestamp, val := tp.Series()
			if parsedTimestamp != nil {
				t = *parsedTimestamp
			}
			metadata[currMeta.Metric] = currMeta

			lset := labels.New()
			_ = tp.Metric(&lset)
			l.labelsByRef[storage.SeriesRef(ref)] = lset

			batch = append(batch, record.RefSample{
				Ref: chunks.HeadSeriesRef(ref), V: val, T: t,
			})
			ref++
		}

		l.e.Export(func(metric string) (export.MetricMetadata, bool) {
			m, ok := metadata[metric]
			return m, ok
		}, batch, nil)
	}
}
