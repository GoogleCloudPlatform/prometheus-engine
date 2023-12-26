// Copyright 2023 Google LLC
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

package deployutil

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	"github.com/go-logr/logr"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func DeployGlobalResources(ctx context.Context, kubeClient client.Client) error {
	// We must deploy CRDs first, or else REST mapper will fail in subsequent calls.
	crdResources, err := crdResources(kubeClient.Scheme(), kubeClient.RESTMapper())
	if err != nil {
		return err
	}
	for _, obj := range crdResources {
		if _, err := controllerutil.CreateOrUpdate(ctx, kubeClient, obj, func() error { return nil }); err != nil {
			return err
		}
	}

	globalResources, _, err := operatorResources(kubeClient.Scheme(), kubeClient.RESTMapper())
	if err != nil {
		return err
	}
	for _, obj := range globalResources {
		if _, err := controllerutil.CreateOrUpdate(ctx, kubeClient, obj, func() error { return nil }); err != nil {
			return err
		}
	}
	return nil
}

type deployOptions struct {
	logger            logr.Logger
	operatorNamespace string
	publicNamespace   string
	userNamespace     string
	labelName         string
	labelValue        string
	projectID         string
	cluster           string
	location          string
}

func (opts *deployOptions) validate() {
	if opts.operatorNamespace == "" {
		opts.operatorNamespace = operator.DefaultOperatorNamespace
	}
	if opts.publicNamespace == "" {
		opts.publicNamespace = operator.DefaultPublicNamespace
	}
	if opts.userNamespace == "" {
		opts.publicNamespace = corev1.NamespaceDefault
	}
}

type DeployOption interface {
	applyToDeployOptions(*deployOptions)
}

type deployOptionFunc func(*deployOptions)

func (f deployOptionFunc) applyToDeployOptions(do *deployOptions) {
	f(do)
}

func WithOperatorNamespace(namespace string) deployOptionFunc {
	return deployOptionFunc(func(do *deployOptions) {
		do.operatorNamespace = namespace
	})
}

func WithPublicNamespace(namespace string) deployOptionFunc {
	return deployOptionFunc(func(do *deployOptions) {
		do.publicNamespace = namespace
	})
}

func WithUserNamespace(namespace string) deployOptionFunc {
	return deployOptionFunc(func(do *deployOptions) {
		do.userNamespace = namespace
	})
}

func WithLabels(name, value string) deployOptionFunc {
	return deployOptionFunc(func(do *deployOptions) {
		do.labelName = name
		do.labelValue = value
	})
}

func WithMeta(projectID, cluster, location string) deployOptionFunc {
	return deployOptionFunc(func(do *deployOptions) {
		do.projectID = projectID
		do.cluster = cluster
		do.location = location
	})
}

func DeployOperator(t testing.TB, ctx context.Context, restConfig *rest.Config, kubeClient client.Client, deployOpts ...DeployOption) error {
	opts := &deployOptions{}
	for _, opt := range deployOpts {
		opt.applyToDeployOptions(opts)
	}
	opts.validate()

	if err := createResources(t, context.Background(), restConfig, kubeClient, opts, func(obj client.Object) client.Object {
		switch obj := obj.(type) {
		case *corev1.Namespace:
			if obj.GetName() == opts.operatorNamespace || obj.GetName() == opts.publicNamespace {
				if obj.Labels == nil {
					obj.Labels = map[string]string{}
				}
				obj.Labels[opts.labelName] = opts.labelValue
			}
		case *appsv1.DaemonSet:
			if obj.GetName() == operator.NameCollector {
				if obj.Spec.Template.Labels == nil {
					obj.Spec.Template.Labels = map[string]string{}
				}
				obj.Spec.Template.Labels[opts.labelName] = opts.labelValue
				for i := range obj.Spec.Template.Spec.Containers {
					container := &obj.Spec.Template.Spec.Containers[i]
					if container.Name == operator.CollectorPrometheusContainerName {
						container.Args = append(container.Args, "--export.debug.disable-auth")
						break
					}
				}
			}
		case *appsv1.Deployment:
			if obj.GetName() == operator.NameOperator {
				container, err := kubeutil.DeploymentContainer(obj, "operator")
				if err != nil {
					t.Fatalf("unable to find operator container: %s", err)
				}
				container.Args = append(container.Args, fmt.Sprintf("--project-id=%s", opts.projectID))
				container.Args = append(container.Args, fmt.Sprintf("--location=%s", opts.location))
				container.Args = append(container.Args, fmt.Sprintf("--cluster=%s", opts.cluster))
				container.Args = append(container.Args, fmt.Sprintf("--operator-namespace=%s", opts.operatorNamespace))
				container.Args = append(container.Args, fmt.Sprintf("--public-namespace=%s", opts.publicNamespace))
			}
		}
		return obj
	}); err != nil {
		return err
	}

	t.Log("waiting for operator to be ready...")
	if err := waitForOperatorReady(ctx, kubeClient, opts.operatorNamespace); err != nil {
		t.Fatalf("waiting for operator to be ready: %s", err)
	}

	return nil
}

func waitForOperatorReady(ctx context.Context, kubeClient client.Client, namespace string) error {
	// The GMP operator doesn't have a proper readiness check.
	// First, ensure that the reconcile loop has run at least once.
	// if err := wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*2, true, func(ctx context.Context) (done bool, err error) {
	// 	configMap := corev1.ConfigMap{}
	// 	if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: operator.NameCollector}, &configMap); err != nil {
	// 		return false, nil
	// 	}
	// 	return true, nil
	// }); err != nil {
	// 	return fmt.Errorf("reconcile loop never ran: %w", err)
	// }
	// Next, ensure that the webhook CA was written so we can validate objects.
	if err := wait.PollUntilContextTimeout(ctx, time.Second*3, time.Minute*4, true, func(ctx context.Context) (done bool, err error) {
		webhookConfig := admissionregistrationv1.ValidatingWebhookConfiguration{}
		webhookConfigName := fmt.Sprintf("gmp-operator.%s.monitoring.googleapis.com", namespace)
		if err := kubeClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: webhookConfigName}, &webhookConfig); err != nil {
			return false, nil
		}
		if len(webhookConfig.Webhooks) == 0 {
			return false, nil
		}
		return len(webhookConfig.Webhooks[0].ClientConfig.CABundle) != 0, nil
	}); err != nil {
		return fmt.Errorf("webhook CA never written: %w", err)
	}
	return nil
}

func createResources(t testing.TB, ctx context.Context, restConfig *rest.Config, kubeClient client.Client, opts *deployOptions, normalizeFn func(client.Object) client.Object) error {
	_, localResources, err := operatorResources(kubeClient.Scheme(), kubeClient.RESTMapper())
	if err != nil {
		return err
	}
	localResources = append(localResources, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.userNamespace,
		},
	})
	resourceTransformer := operatorResourceTransformer{
		operatorNamespace: opts.operatorNamespace,
		publicNamespace:   opts.publicNamespace,
	}
	for _, obj := range localResources {
		switch obj := obj.(type) {
		case *appsv1.DaemonSet:
			// Set the label so we could self-scrape the correct DaemonSet.
			if obj.Spec.Template.Labels == nil {
				obj.Spec.Template.Labels = map[string]string{}
			}
			obj.Spec.Template.Labels[opts.labelName] = opts.labelValue
		}
		normalizeResourceNamespaceSelector(obj, opts.labelName, opts.labelValue)

		if err := normalizeResource(kubeClient.Scheme(), kubeClient.RESTMapper(), obj, &resourceTransformer); err != nil {
			return err
		}

		obj = normalizeFn(obj)
		if obj == nil {
			continue
		}

		if err := kubeClient.Create(ctx, obj); err != nil {
			return err
		}
	}
	return err
}

type embeddedObj struct {
	// gvk is the GVK of the visited object.
	gvk schema.GroupVersionKind
	// isNamespaced indicates whether the GVK is namespaced.
	isNamespaced bool
	// name is the name of the visited object, or nil for all objects of this type.
	name *string
	// namespace is the namespace of the visited object, or nil if it is not namespaced or for all
	// objects of this type.
	namespace *string
}

func normalizeResourceNamespaceSelector(obj client.Object, labelName, labelValue string) {
	switch obj := obj.(type) {
	case *corev1.Namespace:
		if obj.Labels == nil {
			obj.Labels = map[string]string{}
		}
		obj.Labels[labelName] = labelValue
	case *admissionregistrationv1.ValidatingWebhookConfiguration:
		for i := range obj.Webhooks {
			webhook := &obj.Webhooks[i]
			if webhook.NamespaceSelector == nil {
				webhook.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{},
				}
			}
			webhook.NamespaceSelector.MatchLabels[labelName] = labelValue
		}
	case *admissionregistrationv1.MutatingWebhookConfiguration:
		for i := range obj.Webhooks {
			webhook := &obj.Webhooks[i]
			if webhook.NamespaceSelector == nil {
				webhook.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: map[string]string{},
				}
			}
			webhook.NamespaceSelector.MatchLabels[labelName] = labelValue
		}
	}
}

type visitFunc func(opts embeddedObj) error

func visitObjectEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, obj client.Object, visit visitFunc) error {
	switch obj := obj.(type) {
	case *rbacv1.Role:
		return visitRoleSpecEmbedded(scheme, restMapper, obj.Rules, visit)
	case *rbacv1.ClusterRole:
		return visitRoleSpecEmbedded(scheme, restMapper, obj.Rules, visit)
	case *rbacv1.RoleBinding:
		return visitRoleBindingSpecEmbedded(scheme, restMapper, &obj.RoleRef, obj.Subjects, visit)
	case *rbacv1.ClusterRoleBinding:
		return visitRoleBindingSpecEmbedded(scheme, restMapper, &obj.RoleRef, obj.Subjects, visit)
	case *admissionregistrationv1.ValidatingWebhookConfiguration:
		for i := range obj.Webhooks {
			webhook := &obj.Webhooks[i]
			if err := visitWebhookClientConfigEmbedded(scheme, restMapper, &webhook.ClientConfig, visit); err != nil {
				return err
			}
		}
	case *admissionregistrationv1.MutatingWebhookConfiguration:
		for i := range obj.Webhooks {
			webhook := &obj.Webhooks[i]
			if err := visitWebhookClientConfigEmbedded(scheme, restMapper, &webhook.ClientConfig, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

func visitWebhookClientConfigEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, roleRef *admissionregistrationv1.WebhookClientConfig, visit visitFunc) error {
	if roleRef.Service == nil {
		return nil
	}
	gvk, err := apiutil.GVKForObject(&corev1.Service{}, scheme)
	if err != nil {
		return err
	}
	return visit(embeddedObj{
		gvk:          gvk,
		name:         &roleRef.Service.Name,
		namespace:    &roleRef.Service.Namespace,
		isNamespaced: true,
	})
}

func visitRoleBindingSpecEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, roleRef *rbacv1.RoleRef, subjects []rbacv1.Subject, visit visitFunc) error {
	embedded, err := roleRefEmbedded(scheme, restMapper, roleRef)
	if err != nil {
		return err
	}
	if err := visit(embedded); err != nil {
		return err
	}

	for i := range subjects {
		embedded, err := subjectEmbedded(scheme, restMapper, &subjects[i])
		if err != nil {
			return err
		}
		if err := visit(embedded); err != nil {
			return err
		}
	}
	return nil
}

func visitRoleSpecEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, rules []rbacv1.PolicyRule, visit visitFunc) error {
	for i := range rules {
		err := visitPolicyRuleEmbedded(scheme, restMapper, &rules[i], visit)
		if err != nil {
			return err
		}
	}
	return nil
}

func getGVKFromGroupResource(scheme *runtime.Scheme, restMapper meta.RESTMapper, gr schema.GroupResource) (schema.GroupVersionKind, error) {
	kind, err := restMapper.ResourceSingularizer(gr.Resource)
	if err != nil {
		return schema.GroupVersionKind{}, err
	}
	return getGVKFromGroupKind(scheme, schema.GroupKind{Group: gr.Group, Kind: kind})
}

func getGVKFromGroupKind(scheme *runtime.Scheme, gk schema.GroupKind) (schema.GroupVersionKind, error) {
	groupVersions := scheme.PrioritizedVersionsForGroup(gk.Group)
	if len(groupVersions) == 0 {
		return schema.GroupVersionKind{}, fmt.Errorf("no versions found for group %s", gk.Group)
	}

	return gk.WithVersion(groupVersions[0].Version), nil
}

func visitPolicyRuleEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, rule *rbacv1.PolicyRule, visit visitFunc) error {
	for _, resource := range rule.Resources {
		if resource == "*" {
			return errors.New("unable to normalize resource rule with wildcard")
		}
		for _, apiGroup := range rule.APIGroups {
			if apiGroup == "*" {
				return errors.New("unable to normalize resource rule with wildcard")
			}
			subresource := strings.Split(resource, "/")
			gvk, err := getGVKFromGroupResource(scheme, restMapper, schema.GroupResource{Group: apiGroup, Resource: subresource[0]})
			if err != nil {
				return err
			}
			isNamespaced, err := apiutil.IsGVKNamespaced(gvk, restMapper)
			if err != nil {
				return err
			}
			if len(rule.ResourceNames) == 0 {
				if err := visit(embeddedObj{
					gvk:          gvk,
					isNamespaced: isNamespaced,
				}); err != nil {
					return err
				}
			}
			for i := range rule.ResourceNames {
				name := &rule.ResourceNames[i]
				if err := visit(embeddedObj{
					gvk:          gvk,
					name:         name,
					isNamespaced: isNamespaced,
				}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func roleRefEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, roleRef *rbacv1.RoleRef) (embeddedObj, error) {
	gvk, err := getGVKFromGroupKind(scheme, schema.GroupKind{Group: roleRef.APIGroup, Kind: roleRef.Kind})
	if err != nil {
		return embeddedObj{}, err
	}
	isNamespaced, err := apiutil.IsGVKNamespaced(gvk, restMapper)
	if err != nil {
		return embeddedObj{}, err
	}

	return embeddedObj{
		gvk:          gvk,
		isNamespaced: isNamespaced,
		name:         &roleRef.Name,
		namespace:    nil,
	}, nil
}

func subjectEmbedded(scheme *runtime.Scheme, restMapper meta.RESTMapper, subject *rbacv1.Subject) (embeddedObj, error) {
	if subject.APIGroup == "" && (subject.Kind == "User" || subject.Kind == "Group") {
		subject.APIGroup = "rbac.authorization.k8s.io"
	}

	gvk, err := getGVKFromGroupKind(scheme, schema.GroupKind{Group: subject.APIGroup, Kind: subject.Kind})
	if err != nil {
		return embeddedObj{}, err
	}
	isNamespaced, err := apiutil.IsGVKNamespaced(gvk, restMapper)
	if err != nil {
		return embeddedObj{}, err
	}

	return embeddedObj{
		gvk:          gvk,
		isNamespaced: isNamespaced,
		name:         &subject.Name,
		namespace:    &subject.Namespace,
	}, nil
}

type resourceTransformer interface {
	TransformNamespace(namespace *string)
	TransformMetaNamespace(namespace *string)
}

type operatorResourceTransformer struct {
	operatorNamespace string
	publicNamespace   string
}

func (t *operatorResourceTransformer) TransformNamespace(namespace *string) {
	switch *namespace {
	case "gmp-system":
		*namespace = t.operatorNamespace
	case "gmp-public":
		*namespace = t.publicNamespace
	}
}

func (t *operatorResourceTransformer) TransformMetaNamespace(namespace *string) {
	switch *namespace {
	case "gmp-operator.gmp-system.monitoring.googleapis.com":
		*namespace = "gmp-operator." + t.operatorNamespace + ".monitoring.googleapis.com"
	default:
		if strings.HasPrefix(*namespace, "gmp-system:") {
			*namespace = t.operatorNamespace + ":" + strings.TrimPrefix(*namespace, "gmp-system:")
		}
	}
}

func normalizeResource(scheme *runtime.Scheme, restMapper meta.RESTMapper, obj client.Object, transformer resourceTransformer) error {
	if err := visitObjectEmbedded(scheme, restMapper, obj, func(opts embeddedObj) error {
		isNamespaced, err := apiutil.IsGVKNamespaced(opts.gvk, restMapper)
		if err != nil {
			return err
		}
		if isNamespaced && opts.namespace != nil {
			transformer.TransformNamespace(opts.namespace)
		}
		if !isNamespaced && opts.name != nil {
			transformer.TransformMetaNamespace(opts.name)
		}
		return nil
	}); err != nil {
		return err
	}

	if obj, ok := obj.(*corev1.Namespace); ok {
		transformer.TransformNamespace(&obj.Name)
		return nil
	}

	isNamespaced, err := apiutil.IsObjectNamespaced(obj, scheme, restMapper)
	if err != nil {
		return nil
	}
	if isNamespaced {
		namespace := obj.GetNamespace()
		transformer.TransformNamespace(&namespace)
		obj.SetNamespace(namespace)
		return nil
	}
	name := obj.GetName()
	transformer.TransformMetaNamespace(&name)
	obj.SetName(name)

	return nil
}
