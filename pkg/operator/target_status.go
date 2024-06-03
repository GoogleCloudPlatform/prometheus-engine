// Copyright 2022 Google LLC
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

package operator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/api"
	prometheusv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/clock"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	targetStatusDuration = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "prometheus_engine_target_status_duration",
		Help: "A metric indicating how long it took to fetch the complete target status.",
	}, []string{})

	// Minimum duration between polls.
	minPollDuration = 10 * time.Second
)

// Responsible for fetching the targets given a pod.
type getTargetFn func(ctx context.Context, logger logr.Logger, httpClient *http.Client, port int32, pod *corev1.Pod) (*prometheusv1.TargetsResult, error)

// targetStatusReconciler to hold cached client state and source channel.
type targetStatusReconciler struct {
	ch         chan<- event.GenericEvent
	opts       Options
	getTarget  getTargetFn
	clock      clock.Clock
	logger     logr.Logger
	httpClient *http.Client
	kubeClient client.Client
}

// setupTargetStatusPoller sets up a reconciler that polls and populate target
// statuses whenever it receives an event.
func setupTargetStatusPoller(op *Operator, registry prometheus.Registerer, httpClient *http.Client) error {
	if err := registry.Register(targetStatusDuration); err != nil {
		return err
	}

	ch := make(chan event.GenericEvent, 1)

	reconciler := &targetStatusReconciler{
		ch:         ch,
		opts:       op.opts,
		getTarget:  getTarget,
		logger:     op.logger,
		httpClient: httpClient,
		kubeClient: op.manager.GetClient(),
		clock:      clock.RealClock{},
	}

	err := ctrl.NewControllerManagedBy(op.manager).
		Named("target-status").
		// controller-runtime requires a For clause of the manager otherwise
		// this controller will fail to build at runtime when calling
		// `Complete`. The reconcile loop doesn't strictly need to watch a
		// particular resource as it's performing polling against a channel
		// source. We use the DaemonSet here, as it's the closest thing to what
		// we're reconciling (i.e. the collector DaemonSet).
		For(
			&appsv1.DaemonSet{},
			// For the (rare) cases where the collector DaemonSet is deleted and
			// re-created we don't want this event to reconcile into the
			// polling-based control loop.
			builder.WithPredicates(predicate.NewPredicateFuncs(func(_ client.Object) bool {
				return false
			})),
		).
		WatchesRawSource(&source.Channel{
			Source: ch,
		}, &handler.EnqueueRequestForObject{}).
		Complete(reconciler)
	if err != nil {
		return fmt.Errorf("create target status controller: %w", err)
	}

	// Start the controller only once.
	if err := op.manager.Add(manager.RunnableFunc(func(context.Context) error {
		reconciler.ch <- event.GenericEvent{
			Object: &appsv1.DaemonSet{},
		}
		return nil
	})); err != nil {
		return fmt.Errorf("unable to start target status controller: %w", err)
	}

	return nil
}

// shouldPoll verifies if polling collectors is configured or necessary.
func shouldPoll(ctx context.Context, cfgNamespacedName types.NamespacedName, kubeClient client.Client) (bool, error) {
	// Check if target status is enabled.
	var config monitoringv1.OperatorConfig
	if err := kubeClient.Get(ctx, cfgNamespacedName, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	if !config.Features.TargetStatus.Enabled {
		return false, nil
	}

	// No need to poll if there's no PodMonitorings.
	var podMonitoringList monitoringv1.PodMonitoringList
	if err := kubeClient.List(ctx, &podMonitoringList); err != nil {
		return false, err
	} else if len(podMonitoringList.Items) == 0 {
		var clusterPodMonitoringList monitoringv1.ClusterPodMonitoringList
		if err := kubeClient.List(ctx, &clusterPodMonitoringList); err != nil {
			return false, err
		} else if len(clusterPodMonitoringList.Items) == 0 {
			return false, nil
		}
	}
	return true, nil
}

// Reconcile polls the collector pods, fetches and aggregates target status and
// upserts into each PodMonitoring's Status field.
func (r *targetStatusReconciler) Reconcile(ctx context.Context, _ reconcile.Request) (reconcile.Result, error) {
	timer := r.clock.NewTimer(minPollDuration)

	now := time.Now()

	cfgNamespacedName := types.NamespacedName{
		Name:      NameOperatorConfig,
		Namespace: r.opts.PublicNamespace,
	}

	if should, err := shouldPoll(ctx, cfgNamespacedName, r.kubeClient); err != nil {
		r.logger.Error(err, "should poll")
	} else if should {
		if err := pollAndUpdate(ctx, r.logger, r.opts, r.httpClient, r.getTarget, r.kubeClient); err != nil {
			r.logger.Error(err, "poll and update")
		} else {
			// Only log metrics if target polling was successful.
			duration := time.Since(now)
			targetStatusDuration.WithLabelValues().Set(float64(duration.Milliseconds()))
		}
	}

	// Check if we beat the timer, otherwise wait.
	select {
	case <-ctx.Done():
		break
	case <-timer.C():
		r.ch <- event.GenericEvent{
			Object: &appsv1.DaemonSet{},
		}
	}

	return reconcile.Result{}, nil
}

// pollAndUpdate fetches and updates the target status in each collector pod.
func pollAndUpdate(ctx context.Context, logger logr.Logger, opts Options, httpClient *http.Client, getTarget getTargetFn, kubeClient client.Client) error {
	targets, err := fetchTargets(ctx, logger, opts, httpClient, getTarget, kubeClient)
	if err != nil {
		return err
	}

	return updateTargetStatus(ctx, logger, kubeClient, targets)
}

// fetchTargets retrieves the Prometheus targets using the given target function
// for each collector pod.
func fetchTargets(ctx context.Context, logger logr.Logger, opts Options, httpClient *http.Client, getTarget getTargetFn, kubeClient client.Client) ([]*prometheusv1.TargetsResult, error) {
	namespace := opts.OperatorNamespace
	var ds appsv1.DaemonSet
	if err := kubeClient.Get(ctx, client.ObjectKey{
		Name:      NameCollector,
		Namespace: namespace,
	}, &ds); err != nil {
		return nil, err
	}

	selector, err := metav1.LabelSelectorAsSelector(ds.Spec.Selector)
	if err != nil {
		return nil, err
	}

	var port *int32
	for _, container := range ds.Spec.Template.Spec.Containers {
		if isPrometheusContainer(&container) {
			port = getPrometheusPort(&container)
			if port != nil {
				break
			}
		}
	}
	if port == nil {
		return nil, errors.New("unable to detect Prometheus port")
	}

	pods, err := getPrometheusPods(ctx, kubeClient, opts, selector)
	if err != nil {
		return nil, err
	}

	// Set up pod job queue and jobs
	podDiscoveryCh := make(chan prometheusPod)
	wg := sync.WaitGroup{}
	wg.Add(int(opts.TargetPollConcurrency))

	// Must be unbounded or else we deadlock.
	targetCh := make(chan *prometheusv1.TargetsResult)

	for range opts.TargetPollConcurrency {
		// Wrapper function so we can defer in this scope.
		go func() {
			defer wg.Done()
			for prometheusPod := range podDiscoveryCh {
				// Fetch operation is blocking.
				target, err := getTarget(ctx, logger, httpClient, prometheusPod.port, prometheusPod.pod)
				if err != nil {
					logger.Error(err, "failed to fetch target", "pod", prometheusPod.pod.GetName())
				}
				// nil represents being unable to reach a target.
				targetCh <- target
			}
		}()
	}

	// Unbuffered channels are blocking so make sure we end the goroutine processing them.
	go func() {
		for _, pod := range pods {
			podDiscoveryCh <- prometheusPod{
				port: *port,
				pod:  pod,
			}
		}

		// Must close so jobs aren't waiting on the channel indefinitely.
		close(podDiscoveryCh)

		// Close target after we're sure all targets are queued.
		wg.Wait()
		close(targetCh)
	}()

	results := make([]*prometheusv1.TargetsResult, 0)
	for target := range targetCh {
		results = append(results, target)
	}

	return results, nil
}

func patchPodMonitoringStatus(ctx context.Context, kubeClient client.Client, object client.Object, status *monitoringv1.PodMonitoringStatus) error {
	patchStatus := map[string]interface{}{
		"endpointStatuses": status.EndpointStatuses,
	}
	patchObject := map[string]interface{}{"status": patchStatus}

	patchBytes, err := json.Marshal(patchObject)
	if err != nil {
		return fmt.Errorf("unable to marshall status: %w", err)
	}
	patch := client.RawPatch(types.MergePatchType, patchBytes)
	if err := kubeClient.Status().Patch(ctx, object, patch); err != nil {
		return fmt.Errorf("unable to patch status: %w", err)
	}
	return nil
}

// updateTargetStatus populates the status object of each pod using the given
// Prometheus targets.
func updateTargetStatus(ctx context.Context, logger logr.Logger, kubeClient client.Client, targets []*prometheusv1.TargetsResult) error {
	endpointMap, err := buildEndpointStatuses(targets)
	if err != nil {
		return err
	}

	var errs []error
	for job, endpointStatuses := range endpointMap {
		pm, err := getObjectByScrapeJobKey(job)
		if err != nil {
			errs = append(errs, fmt.Errorf("building target: %s: %w", job, err))
			continue
		}
		if pm == nil {
			// Skip hard-coded jobs which we do not patch.
			continue
		}
		pm.GetPodMonitoringStatus().EndpointStatuses = endpointStatuses

		if err := patchPodMonitoringStatus(ctx, kubeClient, pm, pm.GetPodMonitoringStatus()); err != nil {
			// Save and log any error encountered while patching the status.
			// We don't want to prematurely return if the error was transient
			// as we should continue patching all statuses before exiting.
			errs = append(errs, err)
			logger.Error(err, "patching status", "job", job, "gvk", pm.GetObjectKind().GroupVersionKind())
		}
	}

	return errors.Join(errs...)
}

func getPrometheusPods(ctx context.Context, kubeClient client.Client, opts Options, selector labels.Selector) ([]*corev1.Pod, error) {
	var podList corev1.PodList
	if err := kubeClient.List(ctx, &podList, client.InNamespace(opts.OperatorNamespace), client.MatchingLabelsSelector{
		Selector: selector,
	}); err != nil {
		return nil, err
	}
	pods := podList.Items

	podsFiltered := make([]*corev1.Pod, 0)
	for _, pod := range pods {
		if isPrometheusPod(&pod) {
			podsFiltered = append(podsFiltered, pod.DeepCopy())
		}
	}

	return podsFiltered, nil
}

func getTarget(ctx context.Context, _ logr.Logger, httpClient *http.Client, port int32, pod *corev1.Pod) (*prometheusv1.TargetsResult, error) {
	if pod.Status.PodIP == "" {
		return nil, errors.New("pod does not have IP allocated")
	}
	podURL := fmt.Sprintf("http://%s:%d", pod.Status.PodIP, port)
	client, err := api.NewClient(api.Config{
		Address: podURL,
		Client:  httpClient,
	})
	if err != nil {
		return nil, fmt.Errorf("unable to create Prometheus client: %w", err)
	}
	v1api := prometheusv1.NewAPI(client)
	targetsResult, err := v1api.Targets(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to fetch targets: %w", err)
	}

	return &targetsResult, nil
}

type prometheusPod struct {
	port int32
	pod  *corev1.Pod
}

func isPrometheusPod(pod *corev1.Pod) bool {
	for _, container := range pod.Spec.Containers {
		if isPrometheusContainer(&container) {
			return true
		}
	}
	return false
}

func isPrometheusContainer(container *corev1.Container) bool {
	return container.Name == CollectorPrometheusContainerName
}

func getPrometheusPort(container *corev1.Container) *int32 {
	for _, containerPort := range container.Ports {
		// In the future, we could fall back to reading the command line args.
		if containerPort.Name == CollectorPrometheusContainerPortName {
			// Make a copy.
			return ptr.To(containerPort.ContainerPort)
		}
	}
	return nil
}
