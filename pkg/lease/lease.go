// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lease

import (
	"context"
	"fmt"
	"sync"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	metric_pb "google.golang.org/genproto/googleapis/api/metric"
	monitoredres_pb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoring_pb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	timestamp_pb "google.golang.org/protobuf/types/known/timestamppb"
)

// Lease implements a lease on top of the Cloud Monitoring write API.
type Lease struct {
	logger    log.Logger
	client    *monitoring.MetricClient
	projectID string
	resource  *monitoredres_pb.MonitoredResource
	opts      Options

	mtx         sync.Mutex
	owned       bool // Whether the lease is currently owned.
	start, end  time.Time // Range for which the lease is held. Only valid if owned is true.
	changeHooks []func(time.Time, time.Time, bool)
}

type Options struct {
	// Non-standard GCM metric type to use for the lease series.
	MetricType string
	// Metric labels to use for the lease series.
	MetricLabels map[string]string
	// How long into the future to extend the lease if owned.
	ExtensionPeriod time.Duration
	// How frequently to check for lease availability and extend the lease.
	UpdatePeriod time.Duration
	// How frequently to retry transient errors.
	RetryPeriod time.Duration
}

// DefaultLeaseMetricType is the Cloud Monitoring metric type used for writing a lease.
const DefaultLeaseMetricType = "prometheus.googleapis.com/prometheus_engine_lease/counter"

func New(
	logger log.Logger,
	projectID string,
	client *monitoring.MetricClient,
	resource *monitoredres_pb.MonitoredResource,
	opts *Options,
) (*Lease, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	if client == nil {
		return nil, errors.New("no metric client configured")
	}
	if resource == nil {
		return nil, errors.New("lease resource must not be nil")
	}
	if opts == nil {
		opts = &Options{}
	}
	if opts.MetricType == "" {
		opts.MetricType = DefaultLeaseMetricType
	}
	if opts.ExtensionPeriod == 0 {
		opts.ExtensionPeriod = 30 * time.Second
	}
	if opts.UpdatePeriod == 0 {
		opts.UpdatePeriod = 10 * time.Second
	}
	if opts.RetryPeriod == 0 {
		opts.RetryPeriod = time.Second
	}
	return &Lease{
		logger:    logger,
		projectID: projectID,
		client:    client,
		resource:  resource,
		opts:      *opts,
	}, nil
}

func (l *Lease) Range() (start, end time.Time, ok bool) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.start, l.end, l.owned
}

// Register a new function that is called if the lease's ownership status
// changes. Hooks must not be blocking and should return quickly.
func (l *Lease) Register(h func(start, end time.Time, owned bool)) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.changeHooks = append(l.changeHooks, h)
}

func (l *Lease) setOwned(start, end time.Time) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	ownedBefore := l.owned
	l.owned = true
	l.start = start
	l.end = end

	if !ownedBefore {
		level.Info(l.logger).Log("msg", "gained lease ownership", "start", start)
	}
	for _, h := range l.changeHooks {
		h(l.start, l.end, l.owned)
	}
}

func (l *Lease) unsetOwned() {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	ownedBefore := l.owned
	l.owned = false
	l.start, l.end = time.Time{}, time.Time{}

	if !ownedBefore {
		return
	}
	level.Info(l.logger).Log("msg", "lost lease ownership")

	// Only invoke change hooks if the lease was owned before as otherwise no change
	// took place.
	for _, h := range l.changeHooks {
		h(l.start, l.end, l.owned)
	}
}

type stateFn func(context.Context) stateFn

// Run starts trying to acquire and hold the lease until the context is canceled or
// Stop() is called.
func (l *Lease) Run(ctx context.Context) {
	// Execute state machine.
	for state := l.stateFollow(); state != nil; state = state(ctx) {
		// Exit between state changes if necessary. This way we don't rely on
		// state functions to always check the context before returning.
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
	return
}

func (l *Lease) stateFollow() stateFn {
	return func(ctx context.Context) stateFn {
		l.unsetOwned()

		var (
			start = time.Now()
			end = start.Add(l.opts.UpdatePeriod / 2)
			sleep = l.opts.UpdatePeriod
		)
		level.Debug(l.logger).Log("msg", "await lease", "timestamp", start)

		// Check whether the lease is currently unclaimed and we can enter leading state.
		err := l.client.CreateTimeSeries(ctx, l.makeRequest(start, end))
		if err == nil {
			return l.stateLead(start)
		}
		// If it was not an expected write conflict error, treat it as a transient error and
		// retry more quickly. No need for exponential backoff as we're not overly latency sensitive.
		if status.Code(err) != codes.InvalidArgument {
			sleep = l.opts.RetryPeriod
			level.Error(l.logger).Log("msg", "unexpected error while awaiting lease, retrying...", "err", err)
		} else {
			level.Debug(l.logger).Log("msg", "await lease conflict", "err", err)
		}
		// Wait and try again.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sleep):
			return l.stateFollow()
		}
	}
}

func (l *Lease) stateLead(start time.Time) stateFn {
	return func(ctx context.Context) stateFn {
		// Extend the lease by the extension period.
		var (
			end   = time.Now().Add(l.opts.ExtensionPeriod)
			sleep = l.opts.UpdatePeriod
		)
		err := l.client.CreateTimeSeries(ctx, l.makeRequest(start, end))
		if err == nil {
			// We shorten the actual range of lease ownership by the duration at which followers
			// write into the future initially, thereby causing possible overlap.
			l.setOwned(start, end.Add(-l.opts.UpdatePeriod / 2))
			level.Debug(l.logger).Log("msg", "lease extension successful", "start", start, "end", end)
		} else if status.Code(err) == codes.InvalidArgument {
			// If we got here from stateFollow we may not quite own the lease yet as our initial
			// write may be superseded by a different replica.
			// If we did own it before, it may have run out before we got to successfully
			// extend it again.
			level.Debug(l.logger).Log("msg", "lease extension conflict", "start", start, "end", end, "err", err)
			return l.stateFollow()
		} else {
			sleep = l.opts.RetryPeriod
			level.Error(l.logger).Log("msg", "unexpected error while attempt lease, retrying...", "err", err)
		}
		// Wait and try again.
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(sleep):
			return l.stateLead(start)
		}
	}
}

func (l *Lease) makeRequest(start, end time.Time) *monitoring_pb.CreateTimeSeriesRequest {
	return &monitoring_pb.CreateTimeSeriesRequest{
		Name: fmt.Sprintf("projects/%s", l.projectID),
		TimeSeries: []*monitoring_pb.TimeSeries{
			{
				Metric: &metric_pb.Metric{
					Type:   l.opts.MetricType,
					Labels: l.opts.MetricLabels,
				},
				Resource:   l.resource,
				MetricKind: metric_pb.MetricDescriptor_CUMULATIVE,
				ValueType:  metric_pb.MetricDescriptor_DOUBLE,
				Points: []*monitoring_pb.Point{
					{
						Interval: &monitoring_pb.TimeInterval{
							StartTime: timestamp_pb.New(start),
							EndTime:   timestamp_pb.New(end),
						},
						Value: &monitoring_pb.TypedValue{
							Value: &monitoring_pb.TypedValue_DoubleValue{1},
						},
					},
				},
			},
		},
	}
}
