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

package operator

import (
	"context"
	"fmt"

	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	autoscalingv1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	collectorVPAName = "collector"
)

type scalingReconciler struct {
	client client.Client
	opts   Options
}

func newScalingReconciler(c client.Client, opts Options) *scalingReconciler {
	return &scalingReconciler{
		client: c,
		opts:   opts,
	}
}

func setupScalingController(op *Operator) error {
	objFilterOperatorConfig := namespacedNamePredicate{
		namespace: op.opts.PublicNamespace,
		name:      NameOperatorConfig,
	}

	err := ctrl.NewControllerManagedBy(op.manager).
		Named("scaling").
		WithEventFilter(predicate.ResourceVersionChangedPredicate{}).
		For(
			&monitoringv1.OperatorConfig{},
			builder.WithPredicates(objFilterOperatorConfig),
		).
		Owns(&autoscalingv1.VerticalPodAutoscaler{}).
		Complete(newScalingReconciler(op.manager.GetClient(), op.opts))
	if err != nil {
		return fmt.Errorf("scaling controller: %w", err)
	}
	return nil
}

func (r *scalingReconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger, _ := logr.FromContext(ctx)
	logger.WithValues("scaling", req.NamespacedName).Info("reconciling scaling")

	var config monitoringv1.OperatorConfig
	if err := r.client.Get(ctx, req.NamespacedName, &config); apierrors.IsNotFound(err) {
		return reconcile.Result{}, deleteVPA(ctx, r.client, r.opts.OperatorNamespace)
	} else if err != nil {
		return reconcile.Result{}, fmt.Errorf("get operatorconfig: %w", err)
	}

	switch {
	case config.Scaling.VPA.Enabled:
		// Apply VPA
		if err := applyVPA(ctx, r.client, r.opts.OperatorNamespace); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{}, nil
	default:
		return reconcile.Result{}, deleteVPA(ctx, r.client, r.opts.OperatorNamespace)
	}
}

func applyVPA(ctx context.Context, c client.Client, namespace string) error {
	vpa := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      collectorVPAName,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, &vpa, func() error {
		vpa.Spec = autoscalingv1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscaling.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Name:       collectorVPAName,
			},
			UpdatePolicy: &autoscalingv1.PodUpdatePolicy{
				UpdateMode: ptr.To(autoscalingv1.UpdateModeAuto),
			},
			ResourcePolicy: &autoscalingv1.PodResourcePolicy{
				ContainerPolicies: []autoscalingv1.ContainerResourcePolicy{
					{
						ContainerName: "prometheus",
						Mode:          ptr.To(autoscalingv1.ContainerScalingModeAuto),
						MinAllowed: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("32Mi"),
						},
					},
					{
						ContainerName: "config-reloader",
						Mode:          ptr.To(autoscalingv1.ContainerScalingModeOff),
					},
				},
			},
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func deleteVPA(ctx context.Context, c client.Writer, namespace string) error {
	vpa := autoscalingv1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      collectorVPAName,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, &vpa); client.IgnoreNotFound(err) != nil {
		return err
	}
	return nil
}
