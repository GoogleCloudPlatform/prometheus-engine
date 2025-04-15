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
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"testing"
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/export"
	"github.com/efficientgo/e2e"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/compliance/promqle2e"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// LocalExportGCMBackend represents locally imported export pkg with GCM as a
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
type LocalExportGCMBackend struct {
	Name  string
	GCMSA []byte
}

func (l LocalExportGCMBackend) Ref() string { return l.Name }

func (l LocalExportGCMBackend) StartAndWaitReady(t testing.TB, _ e2e.Environment) promqle2e.RunningBackend {
	t.Helper()

	ctx := t.Context()

	creds, err := google.CredentialsFromJSON(ctx, l.GCMSA, gcm.DefaultAuthScopes()...)
	if err != nil {
		t.Fatalf("create credentials from JSON: %s", err)
	}

	// Fake, does not matter.
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
		CredentialsFromJSON: l.GCMSA,
	}
	exporterOpts.DefaultUnsetFields()
	e, err := export.New(ctx, log.NewJSONLogger(os.Stderr), nil, exporterOpts, export.NopLease())
	if err != nil {
		t.Fatalf("create exporter: %v", err)
	}

	// Apply empty config, so resources labels are attached.
	if err := e.ApplyConfig(&config.DefaultConfig, nil); err != nil {
		t.Fatalf("apply config: %v", err)
	}

	labelsByRef := map[storage.SeriesRef]labels.Labels{}
	e.SetLabelsByIDFunc(func(ref storage.SeriesRef) labels.Labels {
		return labelsByRef[ref]
	})

	go func() {
		if err := e.Run(); err != nil {
			t.Logf("running exporter: %s", err)
		}
	}()
	return &runningLocalExportWithGCM{
		api:         v1.NewAPI(cl),
		e:           e,
		labelsByRef: labelsByRef,
		collectionLabels: map[string]string{
			"cluster":    cluster,
			"location":   location,
			"project_id": creds.ProjectID,
		},
	}
}

type runningLocalExportWithGCM struct {
	api              v1.API
	collectionLabels map[string]string

	e *export.Exporter

	// NOTE(bwplotka): Not guarded by mutex, so it has to be synced with Exporter.Export.
	labelsByRef map[storage.SeriesRef]labels.Labels
}

func (l *runningLocalExportWithGCM) API() v1.API {
	return l.api
}

func (l *runningLocalExportWithGCM) CollectionLabels() map[string]string {
	return l.collectionLabels
}

func (l *runningLocalExportWithGCM) IngestSamples(ctx context.Context, t testing.TB, recorded [][]*dto.MetricFamily) {
	t.Helper()

	for _, mfs := range recorded {
		if ctx.Err() != nil {
			return // cancel
		}

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
		// as exporter does not need to cache anything for this test.
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
				_, currMeta.Type = tp.Type()
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
