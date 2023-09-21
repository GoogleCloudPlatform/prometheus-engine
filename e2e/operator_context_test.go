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

// Package e2e contains tests that validate the behavior of gmp-operator against a cluster.
// To make tests simple and fast, the test suite runs the operator internally. The CRDs
// are expected to be installed out of band (along with the operator deployment itself in
// a real world setup).
package e2e

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	clientset "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
)

const (
	collectorManifest    = "../cmd/operator/deploy/operator/10-collector.yaml"
	ruleEvalManifest     = "../cmd/operator/deploy/operator/11-rule-evaluator.yaml"
	alertmanagerManifest = "../cmd/operator/deploy/operator/12-alertmanager.yaml"

	testLabel = "monitoring.googleapis.com/prometheus-test"
)

var (
	startTime    = time.Now().UTC()
	globalLogger = zap.New(zap.Level(zapcore.Level(-1)))

	kubeconfig        *rest.Config
	projectID         string
	cluster           string
	location          string
	skipGCM           bool
	gcpServiceAccount string
)

func init() {
	ctrl.SetLogger(globalLogger)
}

// TestMain injects custom flags and adds extra signal handling to ensure testing
// namespaces are cleaned after tests were executed.
func TestMain(m *testing.M) {
	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")

	flag.Parse()

	var err error
	kubeconfig, err = ctrl.GetConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Loading kubeconfig failed:", err)
		os.Exit(1)
	}

	go func() {
		os.Exit(m.Run())
	}()

	// If the process gets terminated by the user, the Go test package
	// doesn't ensure that test cleanup functions are run.
	// Deleting all namespaces ensures we don't leave anything behind regardless.
	// Non-namespaced resources are owned by a namespace and thus cleaned up
	// by Kubernetes' garbage collection.
	term := make(chan os.Signal, 1)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)

	<-term
	if err := cleanupResources(context.Background(), kubeconfig, ""); err != nil {
		fmt.Fprintln(os.Stderr, "Cleaning up namespaces failed:", err)
		os.Exit(1)
	}
}

// OperatorContext manages shared state for a group of test which tests interaction
// of operator and operated resources. OperatorContext mimics cluster with the
// GMP operator. It runs operator code directly in the background (as opposed to
// letting Kubernetes run it). It also deploys collector, rule and alertmanager
// manually for this context.
//
// Contexts are isolated based on an unique namespace. This requires that no
// test affects or can be affected by resources outside the namespace managed by the context.
// The cluster must be left in a clean state after the cleanup handler completed successfully.
type OperatorContext struct {
	*testing.T

	namespace, pubNamespace string

	kubeClient     kubernetes.Interface
	operatorClient clientset.Interface
}

func newOperatorContext(t *testing.T) *OperatorContext {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		t.Fatalf("Build Kubernetes clientset: %s", err)
	}
	operatorClient, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		t.Fatalf("Build operator clientset: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Create a namespace per test and run. This is to ensure that repeated runs of
	// tests don't falsify results. Either by old test resources not being cleaned up
	// (less likely) or metrics observed in GCP being from a previous run (more likely).
	namespace := fmt.Sprintf("gmp-test-%s-%s", strings.ToLower(t.Name()), startTime.Format("20060102-150405"))
	pubNamespace := fmt.Sprintf("%s-pub", namespace)

	tctx := &OperatorContext{
		T:              t,
		namespace:      namespace,
		pubNamespace:   pubNamespace,
		kubeClient:     kubeClient,
		operatorClient: operatorClient,
	}
	t.Cleanup(func() {
		if err := cleanupResources(ctx, kubeconfig, tctx.getSubTestLabelValue()); err != nil {
			t.Fatalf("unable to cleanup resources: %s", err)
		}
		cancel()
	})

	if err := tctx.createBaseResources(ctx); err != nil {
		t.Fatalf("create test namespace: %s", err)
	}

	op, err := operator.New(globalLogger, kubeconfig, operator.Options{
		ProjectID:         projectID,
		Cluster:           cluster,
		Location:          location,
		OperatorNamespace: tctx.namespace,
		PublicNamespace:   tctx.pubNamespace,
		ListenAddr:        ":10250",
	})
	if err != nil {
		t.Fatalf("instantiating operator: %s", err)
	}

	go func() {
		if err := op.Run(ctx, prometheus.NewRegistry()); err != nil {
			// Since we aren't in the main test goroutine we cannot fail with Fatal here.
			t.Errorf("running operator: %s", err)
		}
	}()

	return tctx
}

func (tctx *OperatorContext) getSubTestLabelValue() string {
	return strings.ReplaceAll(tctx.T.Name(), "/", ".")
}

func (tctx *OperatorContext) getSubTestLabels() map[string]string {
	return map[string]string{
		testLabel: tctx.getSubTestLabelValue(),
	}
}

// createBaseResources creates resources the operator requires to exist already.
// These are resources which don't depend on runtime state and can thus be deployed
// statically, allowing to run the operator without critical write permissions.
func (tctx *OperatorContext) createBaseResources(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   tctx.namespace,
			Labels: tctx.getSubTestLabels(),
		},
	}
	pns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   tctx.pubNamespace,
			Labels: tctx.getSubTestLabels(),
		},
	}
	// This will also fail is the namespace already exists, thereby detecting if a previous
	// test run wasn't cleaned up correctly.
	ns, err := tctx.kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create namespace %q: %w", ns, err)
	}
	_, err = tctx.kubeClient.CoreV1().Namespaces().Create(ctx, pns, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create namespace %q: %w", pns, err)
	}

	svcAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:   operator.NameCollector,
			Labels: tctx.getSubTestLabels(),
		},
	}
	_, err = tctx.kubeClient.CoreV1().ServiceAccounts(tctx.namespace).Create(ctx, svcAccount, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create collector service account: %w", err)
	}

	// The cluster role expected to exist already.
	const clusterRoleName = operator.DefaultOperatorNamespace + ":" + operator.NameCollector

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:   clusterRoleName + ":" + tctx.namespace,
			Labels: tctx.getSubTestLabels(),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			// The ClusterRole is expected to exist in the cluster already.
			Name: clusterRoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Namespace: tctx.namespace,
				Name:      operator.NameCollector,
			},
		},
	}
	_, err = tctx.kubeClient.RbacV1().ClusterRoleBindings().Create(ctx, roleBinding, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create cluster role binding: %w", err)
	}

	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return fmt.Errorf("read GCP service account file: %w", err)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:   "user-gcp-service-account",
				Labels: tctx.getSubTestLabels(),
			},
			Data: map[string][]byte{
				"key.json": b,
			},
		}
		_, err = tctx.kubeClient.CoreV1().Secrets(tctx.pubNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create GCP service account secret: %w", err)
		}
	}

	// Load workloads from YAML files and update the namespace to the test namespace.
	collectorBytes, err := os.ReadFile(collectorManifest)
	if err != nil {
		return fmt.Errorf("read collector YAML: %w", err)
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(collectorBytes, nil, nil)
	if err != nil {
		return fmt.Errorf("decode collector: %w", err)
	}
	collector := obj.(*appsv1.DaemonSet)
	collector.Namespace = tctx.namespace
	if collector.Labels == nil {
		collector.Labels = map[string]string{}
	}
	for k, v := range tctx.getSubTestLabels() {
		collector.Labels[k] = v
	}

	_, err = tctx.kubeClient.AppsV1().DaemonSets(tctx.namespace).Create(ctx, collector, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create collector DaemonSet: %w", err)
	}
	evaluatorBytes, err := os.ReadFile(ruleEvalManifest)
	if err != nil {
		return fmt.Errorf("read rule-evaluator YAML: %w", err)
	}

	obj, _, err = scheme.Codecs.UniversalDeserializer().Decode(evaluatorBytes, nil, nil)
	if err != nil {
		return fmt.Errorf("decode evaluator: %w", err)
	}
	evaluator := obj.(*appsv1.Deployment)
	evaluator.Namespace = tctx.namespace
	if evaluator.Labels == nil {
		evaluator.Labels = map[string]string{}
	}
	for k, v := range tctx.getSubTestLabels() {
		evaluator.Labels[k] = v
	}

	_, err = tctx.kubeClient.AppsV1().Deployments(tctx.namespace).Create(ctx, evaluator, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create rule-evaluator Deployment: %w", err)
	}

	alertmanagerBytes, err := os.ReadFile(alertmanagerManifest)
	if err != nil {
		return fmt.Errorf("read alertmanager YAML: %w", err)
	}
	for _, doc := range strings.Split(string(alertmanagerBytes), "---") {
		obj, _, err = scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			return fmt.Errorf("deserializing alertmanager manifest: %w", err)
		}
		switch obj := obj.(type) {
		case *appsv1.StatefulSet:
			obj.Namespace = tctx.namespace
			if obj.Labels == nil {
				obj.Labels = map[string]string{}
			}
			for k, v := range tctx.getSubTestLabels() {
				obj.Labels[k] = v
			}
			if _, err := tctx.kubeClient.AppsV1().StatefulSets(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create alertmanager statefulset: %w", err)
			}
		case *corev1.Secret:
			obj.Namespace = tctx.namespace
			if obj.Labels == nil {
				obj.Labels = map[string]string{}
			}
			for k, v := range tctx.getSubTestLabels() {
				obj.Labels[k] = v
			}
			if _, err := tctx.kubeClient.CoreV1().Secrets(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create alertmanager secret: %w", err)
			}
		case *corev1.Service:
			obj.Namespace = tctx.namespace
			if obj.Labels == nil {
				obj.Labels = map[string]string{}
			}
			for k, v := range tctx.getSubTestLabels() {
				obj.Labels[k] = v
			}
			if _, err := tctx.kubeClient.CoreV1().Services(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create alertmanager service: %w", err)
			}
		}
	}

	return nil
}

// subtest derives a new test function from a function accepting a test context.
func (tctx *OperatorContext) subtest(f func(context.Context, *OperatorContext)) func(*testing.T) {
	return func(t *testing.T) {
		childCtx := *tctx
		childCtx.T = t
		f(context.TODO(), &childCtx)
	}
}

func getGroupVersionKinds(discoveryClient discovery.DiscoveryInterface) ([]schema.GroupVersionKind, error) {
	_, resources, err := discoveryClient.ServerGroupsAndResources()
	if err != nil {
		return nil, err
	}
	var errs []error
	var gvks []schema.GroupVersionKind
	for _, resource := range resources {
		for _, api := range resource.APIResources {
			gv, err := schema.ParseGroupVersion(resource.GroupVersion)
			if err != nil {
				errs = append(errs, nil)
				continue
			}
			gvks = append(gvks, gv.WithKind(api.Kind))
		}
	}
	return gvks, errors.Join(errs...)
}

func getNamespaces(ctx context.Context, kubeClient client.Client) ([]string, error) {
	var namespaces []string
	var namespaceList corev1.NamespaceList
	if err := kubeClient.List(ctx, &namespaceList); err != nil {
		return nil, err
	}
	for _, namespace := range namespaceList.Items {
		namespaces = append(namespaces, namespace.Name)
	}
	return namespaces, nil
}

func isNamespaced(discovery discovery.DiscoveryInterface, gvk schema.GroupVersionKind) (bool, error) {
	resources, err := discovery.ServerResourcesForGroupVersion(gvk.GroupVersion().String())
	if err != nil {
		return false, err
	}
	for _, resource := range resources.APIResources {
		if resource.Kind == gvk.Kind {
			return resource.Namespaced, nil
		}
	}
	return false, fmt.Errorf("resource not discovered %s", gvk.String())
}

// isNoMatchError returns true if the error indicates that the type does not exist.
func isNoMatchError(err error, gvk schema.GroupVersionKind) bool {
	return err.Error() == fmt.Sprintf("no matches for kind %q in version %q", gvk.Kind, gvk.GroupVersion().String())
}

func labelListOptions(labelName, labelValue, namespace string) (client.ListOptions, error) {
	listOpts := client.ListOptions{}
	if labelValue == "" {
		req, err := labels.NewRequirement(labelName, selection.Exists, []string{})
		if err != nil {
			return listOpts, err
		}
		listOpts.LabelSelector = labels.NewSelector().Add(*req)
	} else {
		req, err := labels.NewRequirement(labelName, selection.Equals, []string{labelValue})
		if err != nil {
			return listOpts, err
		}
		listOpts.LabelSelector = labels.NewSelector().Add(*req)
	}
	if namespace != "" {
		listOpts.Namespace = namespace
	}
	return listOpts, nil
}

func cleanupResource(ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, labelValue, namespace string) error {
	listOpts, err := labelListOptions(testLabel, labelValue, namespace)
	if err != nil {
		return err
	}

	apiVersion, kind := gvk.ToAPIVersionAndKind()
	obj := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       kind,
		},
	}

	if err := kubeClient.DeleteAllOf(ctx, &obj, &client.DeleteAllOfOptions{
		ListOptions: listOpts,
	}); err != nil {
		if apierrors.IsMethodNotSupported(err) {
			// We are not allowed to touch Kubernetes-managed objects.
			return nil
		}
		if isNoMatchError(err, gvk) {
			// This is a meta-resource used in client-go and doesn't exist.
			return nil
		}
		return fmt.Errorf("unable to delete %s: %w", gvk.String(), err)
	}
	return nil
}

// cleanupResources cleans all resources created by tests. If no label value is provided, then all
// resources with the label are removed.
func cleanupResources(ctx context.Context, restConfig *rest.Config, labelValue string) error {
	kubeClient, err := client.New(restConfig, client.Options{})
	if err != nil {
		return err
	}
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return err
	}
	gvks, err := getGroupVersionKinds(discoveryClient)
	if err != nil {
		return err
	}

	namespaces, err := getNamespaces(ctx, kubeClient)
	if err != nil {
		return err
	}

	// The namespaces have to be deleted last, so skip those.
	namespaceGVK := schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	}

	var errs []error
	for _, gvk := range gvks {
		if namespaceGVK == gvk {
			continue
		}

		namespaced, err := isNamespaced(discoveryClient, gvk)
		if err != nil {
			return err
		}
		if namespaced {
			for _, namespace := range namespaces {
				if err := cleanupResource(ctx, kubeClient, gvk, labelValue, namespace); err != nil {
					errs = append(errs, err)
				}
			}
		} else {
			if err := cleanupResource(ctx, kubeClient, gvk, labelValue, ""); err != nil {
				errs = append(errs, err)
			}
		}
	}

	// DeleteAllOf does not work for namespaces, so we must delete individually.
	listOpts, err := labelListOptions(testLabel, labelValue, "")
	if err != nil {
		return err
	}
	namespaceList := corev1.NamespaceList{}
	if err := kubeClient.List(ctx, &namespaceList, &listOpts); err != nil {
		return err
	}
	for _, namespace := range namespaceList.Items {
		if err := kubeClient.Delete(ctx, &namespace, &client.DeleteOptions{}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
