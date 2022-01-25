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
	"os"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"k8s.io/apimachinery/pkg/util/uuid"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
)

var (
	leaseHolder = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_engine_lease_is_held",
		Help: "A boolean metric indicating whether the lease with the given key is currently held.",
	}, []string{"key"})

	leaseFailingOpen = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_engine_lease_failing_open",
		Help: "A boolean metric indicating whether the lease is currently in fail-open state.",
	}, []string{"key"})
)

// Lease implements a lease on time ranges for different backends.
// If the lease backend has intermittent failure, the lease will attempt
// to gracefully fail open by extending the lease of the most recent lease holder.
// This is done in best-effort manner.
type Lease struct {
	logger log.Logger
	opts   Options

	lock           *wrappedLock
	elector        *leaderelection.LeaderElector
	onLeaderChange func()
}

type Options struct {
	// LeaseDuration is the duration that non-leader candidates will
	// wait to force acquire leadership. This is measured against time of
	// last observed ack.
	//
	// A client needs to wait a full LeaseDuration without observing a change to
	// the record before it can attempt to take over. When all clients are
	// shutdown and a new set of clients are started with different names against
	// the same leader record, they must wait the full LeaseDuration before
	// attempting to acquire the lease. Thus LeaseDuration should be as short as
	// possible (within your tolerance for clock skew rate) to avoid a possible
	// long waits in the scenario.
	//
	// Defaults to 15 seconds.
	LeaseDuration time.Duration
	// RenewDeadline is the duration that the acting master will retry
	// refreshing leadership before giving up.
	//
	// Defaults to 10 seconds.
	RenewDeadline time.Duration
	// RetryPeriod is the duration the LeaderElector clients should wait
	// between tries of actions.
	//
	// Defaults to 2 seconds.
	RetryPeriod time.Duration
}

func NewKubernetes(
	logger log.Logger,
	metrics prometheus.Registerer,
	config *rest.Config,
	namespace, name string,
	opts *Options,
) (*Lease, error) {
	if namespace == "" || name == "" {
		return nil, errors.New("namespace and name are required for lease")
	}
	// Leader id, needs to be unique
	id, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	id = id + "_" + string(uuid.NewUUID())

	// Construct clients for leader election
	config = rest.CopyConfig(config)
	rest.AddUserAgent(config, "leader-election")

	corev1Client, err := corev1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	coordinationClient, err := coordinationv1client.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	lock, err := resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace, name,
		corev1Client, coordinationClient,
		resourcelock.ResourceLockConfig{Identity: id},
	)
	if err != nil {
		return nil, err
	}
	return New(logger, metrics, lock, opts)
}

func New(
	logger log.Logger,
	metrics prometheus.Registerer,
	lock resourcelock.Interface,
	opts *Options,
) (*Lease, error) {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	if opts == nil {
		opts = &Options{}
	}
	if opts.LeaseDuration == 0 {
		opts.LeaseDuration = 15 * time.Second
	}
	if opts.RetryPeriod == 0 {
		opts.RetryPeriod = 2 * time.Second
	}
	if opts.RenewDeadline == 0 {
		opts.RenewDeadline = 10 * time.Second
	}
	if metrics != nil {
		metrics.Register(leaseHolder)
		metrics.Register(leaseFailingOpen)
	}
	leaseHolder.WithLabelValues(lock.Describe()).Set(0)
	leaseFailingOpen.WithLabelValues(lock.Describe()).Set(0)

	wlock := newWrappedLock(lock)

	lease := &Lease{
		logger:         logger,
		lock:           wlock,
		onLeaderChange: func() {},
		opts:           *opts,
	}

	var err error
	// We use the Kubernetes client-go leader implementation to drive the lease logic.
	// The lock itself however may be implemented against any consistent backend.
	lease.elector, err = leaderelection.NewLeaderElector(leaderelection.LeaderElectionConfig{
		Lock:          wlock,
		LeaseDuration: opts.LeaseDuration,
		RetryPeriod:   opts.RetryPeriod,
		RenewDeadline: opts.RenewDeadline,
		// The purpose of our lease is to determine time ranges for which a leader sends
		// sample data. We cannot be certain that we never sent data for a later in-range
		// timestamp already. Thus releasing the lease on cancel would produce possible
		// overlaps.
		ReleaseOnCancel: false,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: func(context.Context) {
				lease.onLeaderChange()
				leaseHolder.WithLabelValues(lock.Describe()).Set(1)
			},
			OnStoppedLeading: func() {
				lease.onLeaderChange()
				leaseHolder.WithLabelValues(lock.Describe()).Set(0)
			},
		},
	})
	if err != nil {
		return nil, err
	}
	return lease, nil
}

func (l *Lease) Range() (start, end time.Time, ok bool) {
	// If we've previously been the leader but the end timestamp expired, it means we
	// couldn't successfully communicate with the backend to extend the lease or determine
	// that someone else got it.
	// We fail open by pretending that we did extend the lease until we
	// can either extend/reacquire the lease or observe that someone else acquired it.
	//
	// This ensures that transient backend downtimes are generally unnoticeable. It does however
	// not protect against correlated failures, e.g. if all leaders restart while the
	// backend is unavailable. This should rarely be an issue.
	//
	// Also letting non-leader replicas fail open would handle more cases gracefully.
	// However, it also has a ramining risk of leaving the replicas jointly in a bad state:
	// Suppose replica A acquires the lease and writes samples with start timestamp T.
	// Replica B starts but cannot reach the backend, it fails open despite not being the
	// leader before and writes with start timestamp T+1.
	// Now B reaches the lease backend, cannot get the lease and stops sending data. Replica
	// A will keep sending data as the leader but has an older start timestamp, that causes
	// write conflicts. It will indefinitely not be able to write cumulative samples.
	//
	// We could possibly address this in the future by customizing the lease implementation
	// to consider each leader candidates' earliest possible start timestamp and force-acquire
	// the lease if it is more recent than the one of the current leader.
	// For now our taken approach prevents this, as we do rely on a previously agreed-upon start
	// timestamp during a failure scenario.

	// IsLeader checks whether the last observed record matches the own identity.
	// It does not check timestamps and thus keeps returning true if we were the leader
	// previously and currently cannot talk to the backend.
	if !l.elector.IsLeader() {
		return time.Time{}, time.Time{}, false
	}
	start, end = l.lock.lastRange()
	now := time.Now()

	if end.Before(now) {
		leaseFailingOpen.WithLabelValues(l.lock.Describe()).Set(1)
		end = now.Add(l.opts.LeaseDuration)
	} else {
		leaseFailingOpen.WithLabelValues(l.lock.Describe()).Set(0)
	}
	return start, end, true
}

// Run starts trying to acquire and hold the lease until the context is canceled.
func (l *Lease) Run(ctx context.Context) {
	// The elector blocks until it acquired the lease once but exits
	// when losing it. Thus we need to run it in a loop.
	for {
		select {
		case <-ctx.Done():
			return
		default:
			l.elector.Run(ctx)
		}
	}
}

// OnLeaderChange sets a callback that's invoked when the leader of the lease changes.
func (l *Lease) OnLeaderChange(f func()) {
	l.onLeaderChange = f
}

// wrappedLock wraps a LeaseLock implementation and caches the time
// range of the last successful update of the lease record.
type wrappedLock struct {
	resourcelock.Interface

	mtx        sync.Mutex
	start, end time.Time
}

func newWrappedLock(lock resourcelock.Interface) *wrappedLock {
	return &wrappedLock{Interface: lock}
}

// Create attempts to create a leader election record.
func (l *wrappedLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.Interface.Create(ctx, ler)
	l.update(ler, err)
	return err
}

// Update will update an existing leader election record.
func (l *wrappedLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.Interface.Update(ctx, ler)
	l.update(ler, err)
	return err
}

// update the cached state on the create/update result for the record.
func (l *wrappedLock) update(ler resourcelock.LeaderElectionRecord, err error) {
	// If the update was successful, the lease is owned by us and we can update the range.
	if err != nil {
		return
	}
	l.mtx.Lock()
	defer l.mtx.Unlock()

	l.start = ler.AcquireTime.Time
	l.end = ler.RenewTime.Time.Add(time.Duration(ler.LeaseDurationSeconds) * time.Second)
}

func (l *wrappedLock) lastRange() (time.Time, time.Time) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.start, l.end
}
