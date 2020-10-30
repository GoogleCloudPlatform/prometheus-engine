// Copyright 2020 Google Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//     http://www.apache.org/licenses/LICENSE-2.0
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package export

import (
	"context"
	"fmt"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/go-kit/kit/log"
	"github.com/google/gpe-collector/export/exportctx"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb/record"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
)

// Exporter converts Prometheus samples into Cloud Monitoring samples and exporst them.
type Exporter struct {
	logger      log.Logger
	metric      *monitoring.MetricClient
	seriesCache *seriesCache
	builder     *sampleBuilder
}

// New returns a new Cloud Monitoring Exporter.
func New(logger log.Logger) (*Exporter, error) {
	metricClient, err := monitoring.NewMetricClient(context.Background())
	if err != nil {
		return nil, err
	}

	seriesCache := newSeriesCache(logger, metricsPrefix)
	go seriesCache.run(context.Background())

	e := &Exporter{
		logger:      logger,
		metric:      metricClient,
		seriesCache: seriesCache,
		builder:     &sampleBuilder{series: seriesCache},
	}
	return e, nil
}

// Generally global state is not a good approach and actively discouraged throughout
// the Prometheus code bases. However, this is the most practical way to inject the export
// path into lower layers of Prometheus without touching an excessive amount of functions
// in our fork to propagate it.
var globalExporter *Exporter

// InitGlobal initializes the global instance of the GCM exporter.
func InitGlobal(logger log.Logger) (err error) {
	globalExporter, err = New(logger)
	return err
}

// Global returns the global instance of the GCM exporter.
func Global() *Exporter {
	if globalExporter == nil {
		panic("Global GCM exporter used before initialization.")
	}
	return globalExporter
}

// SetLabelsByIDFunc injects a function that can be used to retrieve a label set
// based on a series ID we got through exported sample records.
// Must be called before any call to Export is made.
func (e *Exporter) SetLabelsByIDFunc(f func(uint64) labels.Labels) {
	e.seriesCache.getLabelsByRef = f
}

// Export enqueues the samples to be written to Cloud Monitoring.
func (e *Exporter) Export(ctx context.Context, samples []record.RefSample) {
	target := ctx.Value(exportctx.KeyTarget).(*scrape.Target)
	if target == nil {
		panic("Target missing in context")
	}
	var (
		sample *monitoring_pb.TimeSeries
		hash   uint64
		err    error
	)
	for len(samples) > 0 {
		sample, hash, samples, err = e.builder.next(ctx, target, samples)
		if err != nil {
			panic(err)
		}
		if sample == nil {
			continue
		}
		fmt.Println("sample", hash, sample)
	}
}

// Close the Cloud Monitoring exporter after finishing pending writes.
func (e *Exporter) Close() error {
	return e.metric.Close()
}
