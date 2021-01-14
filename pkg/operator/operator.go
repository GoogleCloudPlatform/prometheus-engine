package operator

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"

	monitoringv1alpha1 "github.com/google/gpe-collector/pkg/operator/apis/operator/v1alpha1"
	clientset "github.com/google/gpe-collector/pkg/operator/generated/clientset/versioned"
	informers "github.com/google/gpe-collector/pkg/operator/generated/informers/externalversions"
)

// Operator to implement managed collection for Google Prometheus Engine.
type Operator struct {
	logger log.Logger

	// Informers that maintain a cache of cluster resources and call configured
	// event handlers on changes.
	informerServiceMonitoring cache.SharedIndexInformer
	// State changes are enqueued into a rate limited work queue, which ensures
	// the operator does not get overloaded and multiple changes to the same resource
	// are not handled in parallel, leading to semantic race conditions.
	queue workqueue.RateLimitingInterface
}

// New instantiates a new Operator.
func New(logger log.Logger, clientConfig *rest.Config) (*Operator, error) {
	operatorClient, err := clientset.NewForConfig(clientConfig)
	if err != nil {
		return nil, errors.Wrap(err, "build operator clientset")
	}
	informerFactory := informers.NewSharedInformerFactory(operatorClient, time.Minute)

	op := &Operator{
		logger:                    logger,
		informerServiceMonitoring: informerFactory.Monitoring().V1alpha1().ServiceMonitorings().Informer(),
		queue:                     workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "GPEOperator"),
	}

	op.informerServiceMonitoring.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: op.enqueueObject,
		UpdateFunc: func(oldObj, newObj interface{}) {
			old := oldObj.(*monitoringv1alpha1.ServiceMonitoring)
			new := newObj.(*monitoringv1alpha1.ServiceMonitoring)
			// Periodic resync will send update events for all known ServiceMonitorings.
			// Two different versions of the same object will differ in resource version.
			if old.ResourceVersion != new.ResourceVersion {
				op.enqueueObject(new)
			}
		},
		DeleteFunc: op.enqueueObject,
	})

	return op, nil
}

// enqueueObject enqueues the object for reconciliation. Only the key is enqueued
// as the queue consumer should retrieve the most recent cache object once it gets to process
// to not process stale state.
func (o *Operator) enqueueObject(obj interface{}) {
	key, err := cache.MetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	o.queue.Add(key)
}

// Run the reconciliation loop of the operator.
func (o *Operator) Run(ctx context.Context) error {
	defer utilruntime.HandleCrash()

	level.Info(o.logger).Log("msg", "starting GPE operator")

	go o.informerServiceMonitoring.Run(ctx.Done())

	level.Info(o.logger).Log("msg", "waiting for informer caches to sync")

	syncCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	ok := cache.WaitForNamedCacheSync("GPEOperator", syncCtx.Done(), o.informerServiceMonitoring.HasSynced)
	cancel()
	if !ok {
		return errors.New("aborted while waiting for informer caches to sync")
	}

	// Process work items until context is canceled.
	go func() {
		<-ctx.Done()
		o.queue.ShutDown()
	}()

	for o.processNextItem(ctx) {
	}
	return nil
}

func (o *Operator) processNextItem(ctx context.Context) bool {
	key, quit := o.queue.Get()
	if quit {
		return false
	}
	defer o.queue.Done(key)

	if err := o.sync(ctx, key.(string)); err == nil {
		// Drop item from rate limit tracking as we successfully processed it.
		// If the item is enqueued again, we'll immediately process it.
		o.queue.Forget(key)
	} else {
		utilruntime.HandleError(errors.Wrap(err, fmt.Sprintf("sync for %q failed", key)))
		// Requeue the item with backoff to retry on transient errors.
		o.queue.AddRateLimited(key)
	}
	return true
}

func (o *Operator) sync(ctx context.Context, key string) error {
	// TODO(freinartz): apply actual operator logic here.
	fmt.Printf("syncing cluster state for ServiceMonitoring %q", key)
	return nil
}
