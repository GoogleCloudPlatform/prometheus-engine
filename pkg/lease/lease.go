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
		Name: "prometheus_engine_lease_holder",
		Help: "A boolean metric indicating whether the lease with the given key is currently held.",
	}, []string{"key"})
)

// Lease implements a lease on time ranges for different backends.
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
	}
	leaseHolder.WithLabelValues(lock.Describe()).Set(0)

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
	return l.lock.Range()
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
// range during which the lease was owned.
type wrappedLock struct {
	resourcelock.Interface

	mtx        sync.Mutex
	start, end time.Time
	owned      bool
}

func newWrappedLock(lock resourcelock.Interface) *wrappedLock {
	return &wrappedLock{Interface: lock}
}

// Create attempts to create a Lease
func (l *wrappedLock) Create(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.Interface.Create(ctx, ler)
	l.update(ler, err)
	return err
}

// Update will update an existing Lease spec.
func (l *wrappedLock) Update(ctx context.Context, ler resourcelock.LeaderElectionRecord) error {
	err := l.Interface.Update(ctx, ler)
	l.update(ler, err)
	return err
}

// update the cached state on the create/update result for the record.
func (l *wrappedLock) update(ler resourcelock.LeaderElectionRecord, err error) {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	// Update causes an error due to transient failure or due to conflicts, i.e. because
	// someone else is holding the lease. If it is nil, we are the lease holder.
	l.owned = err == nil
	if l.owned {
		l.start = ler.AcquireTime.Time
		l.end = ler.RenewTime.Time.Add(time.Duration(ler.LeaseDurationSeconds) * time.Second)
	} else {
		l.start, l.end = time.Time{}, time.Time{}
	}
}

func (l *wrappedLock) Range() (start, end time.Time, owned bool) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	return l.start, l.end, l.owned
}
