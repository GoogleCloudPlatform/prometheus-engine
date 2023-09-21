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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	clientset "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/generated/clientset/versioned"
)

const (
	collectorManifest    = "../cmd/operator/deploy/operator/10-collector.yaml"
	ruleEvalManifest     = "../cmd/operator/deploy/operator/11-rule-evaluator.yaml"
	alertmanagerManifest = "../cmd/operator/deploy/operator/12-alertmanager.yaml"
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
	if err := cleanupAllNamespaces(context.Background()); err != nil {
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
	// A list of owner references that can be attached to non-namespaced
	// test resources so that they'll get cleaned up on teardown.
	ownerReferences []metav1.OwnerReference

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
	// The testing package runs cleanup on a best-effort basis. Thus we have a fallback
	// cleanup of namespaces in TestMain.
	t.Cleanup(cancel)
	t.Cleanup(func() { tctx.cleanupNamespaces() })

	tctx.ownerReferences, err = tctx.createBaseResources(context.TODO())
	if err != nil {
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

// createBaseResources creates resources the operator requires to exist already.
// These are resources which don't depend on runtime state and can thus be deployed
// statically, allowing to run the operator without critical write permissions.
func (tctx *OperatorContext) createBaseResources(ctx context.Context) ([]metav1.OwnerReference, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tctx.namespace,
			// Apply a consistent label to make it easy manually cleanup in case
			// something went wrong with the test cleanup.
			Labels: map[string]string{
				"gmp-operator-test": "true",
			},
		},
	}
	pns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tctx.pubNamespace,
			// Apply a consistent label to make it easy manually cleanup in case
			// something went wrong with the test cleanup.
			Labels: map[string]string{
				"gmp-operator-test": "true",
			},
		},
	}
	// This will also fail is the namespace already exists, thereby detecting if a previous
	// test run wasn't cleaned up correctly.
	ns, err := tctx.kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create namespace %q: %w", ns, err)
	}
	_, err = tctx.kubeClient.CoreV1().Namespaces().Create(ctx, pns, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create namespace %q: %w", pns, err)
	}

	ors := []metav1.OwnerReference{
		{
			APIVersion: "v1",
			Kind:       "Namespace",
			Name:       ns.Name,
			UID:        ns.UID,
		},
	}

	svcAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: operator.NameCollector},
	}
	_, err = tctx.kubeClient.CoreV1().ServiceAccounts(tctx.namespace).Create(ctx, svcAccount, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create collector service account: %w", err)
	}

	// The cluster role expected to exist already.
	const clusterRoleName = operator.DefaultOperatorNamespace + ":" + operator.NameCollector

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName + ":" + tctx.namespace,
			// Tie to the namespace so the binding gets deleted alongside it, even though
			// it's an cluster-wide resource.
			OwnerReferences: ors,
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
		return nil, fmt.Errorf("create cluster role binding: %w", err)
	}

	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return nil, fmt.Errorf("read GCP service account file: %w", err)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name: "user-gcp-service-account",
			},
			Data: map[string][]byte{
				"key.json": b,
			},
		}
		_, err = tctx.kubeClient.CoreV1().Secrets(tctx.pubNamespace).Create(ctx, secret, metav1.CreateOptions{})
		if err != nil {
			return nil, fmt.Errorf("create GCP service account secret: %w", err)
		}
	}

	// Load workloads from YAML files and update the namespace to the test namespace.
	collectorBytes, err := os.ReadFile(collectorManifest)
	if err != nil {
		return nil, fmt.Errorf("read collector YAML: %w", err)
	}
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(collectorBytes, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decode collector: %w", err)
	}
	collector := obj.(*appsv1.DaemonSet)
	collector.Namespace = tctx.namespace

	_, err = tctx.kubeClient.AppsV1().DaemonSets(tctx.namespace).Create(ctx, collector, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create collector DaemonSet: %w", err)
	}
	evaluatorBytes, err := os.ReadFile(ruleEvalManifest)
	if err != nil {
		return nil, fmt.Errorf("read rule-evaluator YAML: %w", err)
	}

	obj, _, err = scheme.Codecs.UniversalDeserializer().Decode(evaluatorBytes, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("decode evaluator: %w", err)
	}
	evaluator := obj.(*appsv1.Deployment)
	evaluator.Namespace = tctx.namespace

	_, err = tctx.kubeClient.AppsV1().Deployments(tctx.namespace).Create(ctx, evaluator, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create rule-evaluator Deployment: %w", err)
	}

	alertmanagerBytes, err := os.ReadFile(alertmanagerManifest)
	if err != nil {
		return nil, fmt.Errorf("read alertmanager YAML: %w", err)
	}
	for _, doc := range strings.Split(string(alertmanagerBytes), "---") {
		obj, _, err = scheme.Codecs.UniversalDeserializer().Decode([]byte(doc), nil, nil)
		if err != nil {
			return nil, fmt.Errorf("deserializing alertmanager manifest: %w", err)
		}
		switch obj := obj.(type) {
		case *appsv1.StatefulSet:
			obj.Namespace = tctx.namespace
			if _, err := tctx.kubeClient.AppsV1().StatefulSets(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return nil, fmt.Errorf("create alertmanager statefulset: %w", err)
			}
		case *corev1.Secret:
			obj.Namespace = tctx.namespace
			if _, err := tctx.kubeClient.CoreV1().Secrets(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return nil, fmt.Errorf("create alertmanager secret: %w", err)
			}
		case *corev1.Service:
			obj.Namespace = tctx.namespace
			if _, err := tctx.kubeClient.CoreV1().Services(tctx.namespace).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
				return nil, fmt.Errorf("create alertmanager service: %w", err)
			}
		}
	}

	return ors, nil
}

func (tctx *OperatorContext) cleanupNamespaces() {
	err := tctx.kubeClient.CoreV1().Namespaces().Delete(context.TODO(), tctx.namespace, metav1.DeleteOptions{})
	if err != nil {
		tctx.Errorf("cleanup namespace %q: %s", tctx.namespace, err)
	}
	err = tctx.kubeClient.CoreV1().Namespaces().Delete(context.TODO(), tctx.pubNamespace, metav1.DeleteOptions{})
	if err != nil {
		tctx.Errorf("cleanup public namespace %q: %s", tctx.namespace, err)
	}
}

// subtest derives a new test function from a function accepting a test context.
func (tctx *OperatorContext) subtest(f func(context.Context, *OperatorContext)) func(*testing.T) {
	return func(t *testing.T) {
		childCtx := *tctx
		childCtx.T = t
		f(context.TODO(), &childCtx)
	}
}

// cleanupAllNamespaces deletes all namespaces created as part of test contexts.
func cleanupAllNamespaces(ctx context.Context) error {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("build Kubernetes clientset: %w", err)
	}
	namespaces, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "gmp-operator-test=true",
	})
	if err != nil {
		return fmt.Errorf("delete namespaces by label: %w", err)
	}
	for _, ns := range namespaces.Items {
		if err := kubeClient.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "deleting namespace %q failed: %s\n", ns.Name, err)
		}
	}
	return nil
}
