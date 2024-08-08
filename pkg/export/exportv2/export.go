package exportv2

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	monitoring_pb "cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	writev2 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/prompb/io/prometheus/write/v2"
	"github.com/go-kit/log"
)

type errorWithHTTPCode struct {
	error
	code int
}

func (e *errorWithHTTPCode) HTTPCode() int {
	if e == nil || e.code == 0 {
		return http.StatusInternalServerError
	}
	return e.code
}

func newHTTPError(err error, code int) error {
	if err == nil {
		return nil
	}
	return &errorWithHTTPCode{
		error: err, code: code,
	}
}

type Exporter struct {
	gcmClient *monitoring.MetricClient
	logger    log.Logger
}

func NewExporter(gcmClient *monitoring.MetricClient, logger log.Logger) *Exporter {
	return &Exporter{gcmClient: gcmClient, logger: log.With(logger, "component", "exportv2")}
}

// ExportPRW converts given PRW 2.0 request into one or more GCM v3 gRPC requests.
// This method is synchronous, it waits until all GCM requests finishes.
// NOTE(bwplotka): Async implementation is possible, but more complex, YAGNI for now.
func (e *Exporter) ExportPRW(ctx context.Context, req *writev2.Request, allowClassicHistograms bool) *errorWithHTTPCode {
	if err := e.exportPRW(ctx, req, allowClassicHistograms); err != nil {
		var ew *errorWithHTTPCode
		if errors.As(err, &ew) { // TODO(bwplotka): Does this support wrapped errors?
			return ew
		}
		return &errorWithHTTPCode{
			error: err, code: http.StatusInternalServerError,
		}
	}
	return nil
}

func (e *Exporter) exportPRW(ctx context.Context, req *writev2.Request, allowClassicHistograms bool) error {
	qm := startQueueManager(ctx, e.gcmClient)

	var errs []error
	// TODO(bwplotka): Consider local concurrency (GOMAXPROCS workers etc).
	for _, ts := range req.Timeseries {
		if ctx.Err() != nil {
			qm.flush()
			return ctx.Err()
		}

		if

		if err := exportTimeSeries(ts, req.Symbols, qm.enqueue); err != nil {
			errs = append(errs, fmt.Errorf("conversion to GCM failed, skipping: %w", err))
		}
	}
	qm.flush() // Flush waits until all requests are completed.

	return errors.Join(append(errs, qm.errors()...)...)
}

type queueManager struct {
	ctx       context.Context
	queueCh   chan *monitoring_pb.TimeSeries
	gcmClient *monitoring.MetricClient
	wg        sync.WaitGroup

	errs []error
}

func startQueueManager(ctx context.Context, gcmClient *monitoring.MetricClient) *queueManager {
	q := &queueManager{
		ctx:       ctx,
		queueCh:   make(chan *monitoring_pb.TimeSeries, 10),
		gcmClient: gcmClient,
	}
	q.wg.Add(1)

	go q.run()
	return q
}

const maxBatchSize = 100

func (q *queueManager) run() {
	defer q.wg.Done()

	batch := make([]*monitoring_pb.TimeSeries, 0, maxBatchSize)
	for {
		// Ignore checking context, we expect flush method to tell us when to stop.
		ts, ok := <-q.queueCh
		if ok {
			batch = append(batch, ts)
		}

		if !ok || len(batch) == maxBatchSize {
			// https://cloud.google.com/monitoring/api/ref_v3/rpc/google.monitoring.v3 200 objects per request max.
			if err := q.gcmClient.CreateTimeSeries(q.ctx, &monitoring_pb.CreateTimeSeriesRequest{
				Name:       "",
				TimeSeries: batch,
			}); err != nil {
				q.errs = append(q.errs, fmt.Errorf("GCM batch send failed for %v series; no more retries!; %w", len(batch), err))
			}
			batch = batch[:0]
		}

		if !ok {
			return
		}
	}
}

func (q *queueManager) enqueue(ts *monitoring_pb.TimeSeries) {
	q.queueCh <- ts
}

func (q *queueManager) flush() {
	close(q.queueCh)
	q.wg.Wait()
}

func (q *queueManager) errors() []error {
	return q.errs
}
