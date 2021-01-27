package e2e

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/google/gpe-collector/pkg/operator"
	clientset "github.com/google/gpe-collector/pkg/operator/generated/clientset/versioned"
)

// testContext manages shared state for a group of test. Test contexts are isolated
// based on a unqiue namespace. This requires that no test affects or can be affected by
// resources outside of the namespace managed by the context.
// The cluster must be left in a clean state after the cleanup handler completed successfully.
type testContext struct {
	*testing.T

	namespace string

	kubeClient     kubernetes.Interface
	operatorClient clientset.Interface
}

func newTestContext(t *testing.T) *testContext {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		t.Fatalf("Build Kubernetes clientset: %s", err)
	}
	operatorClient, err := clientset.NewForConfig(kubeconfig)
	if err != nil {
		t.Fatalf("Build operator clientset: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tctx := &testContext{
		T:              t,
		namespace:      fmt.Sprintf("gpe-test-%s", strings.ToLower(t.Name())),
		kubeClient:     kubeClient,
		operatorClient: operatorClient,
	}
	// The testing package runs cleanup on a best-effort basis. Thus we have a fallback
	// cleanup of namespaces in TestMain.
	t.Cleanup(cancel)
	t.Cleanup(func() { tctx.cleanupNamespace() })

	if err := tctx.createBaseResources(); err != nil {
		t.Fatalf("create test namespace: %s", err)
	}

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	logger = log.With(logger, "test", t.Name())

	op, err := operator.New(logger, kubeconfig, operator.Options{
		Namespace: tctx.namespace,
	})
	if err != nil {
		t.Fatalf("instantiating operator failed: %s", err)
	}

	go func() {
		if err := op.Run(ctx); err != nil {
			// Since we aren't in the main test goroutine we cannot fail with Fatal here.
			t.Errorf("running operator failed: %s", err)
		}
	}()

	return tctx
}

// createBaseResources creates resources the operator requires to exist already.
// These are resources which don't depend on runtime state and can thus be deployed
// statically, allowing to run the operator without critical write permissions.
func (tctx *testContext) createBaseResources() error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tctx.namespace,
			// Apply a consistent label to make it easy manually cleanup in case
			// something went wrong with the test cleanup.
			Labels: map[string]string{
				"gpe-operator-test": "true",
			},
		},
	}
	// This will also fail is the namespace already exists, thereby detecting if a previous
	// test run wasn't cleaned up correctly.
	ns, err := tctx.kubeClient.CoreV1().Namespaces().Create(context.TODO(), ns, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrapf(err, "create namespace %q", tctx.namespace)
	}

	svcAccount := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: operator.CollectorName},
	}
	_, err = tctx.kubeClient.CoreV1().ServiceAccounts(tctx.namespace).Create(context.TODO(), svcAccount, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create collector service account")
	}

	// The cluster role expected to exist already.
	const clusterRoleName = operator.DefaultNamespace + ":" + operator.CollectorName

	roleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName + ":" + tctx.namespace,
			// Tie to the namespace so the binding gets deleted alongside it, even though
			// it's an cluster-wide resource.
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Kind:       "Namespace",
					Name:       ns.Name,
					UID:        ns.UID,
				},
			},
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
				Name:      operator.CollectorName,
			},
		},
	}
	_, err = tctx.kubeClient.RbacV1().ClusterRoleBindings().Create(context.TODO(), roleBinding, metav1.CreateOptions{})
	if err != nil {
		return errors.Wrap(err, "create cluster role binding")
	}
	return nil
}

func (tctx *testContext) cleanupNamespace() {
	err := tctx.kubeClient.CoreV1().Namespaces().Delete(context.TODO(), tctx.namespace, metav1.DeleteOptions{})
	if err != nil {
		tctx.Fatalf("cleanup namespace %q: %s", tctx.namespace, err)
	}
}

// subtest derives a new test function from a function accepting a test context.
func (tctx *testContext) subtest(f func(*testContext)) func(*testing.T) {
	return func(t *testing.T) {
		childCtx := *tctx
		childCtx.T = t
		f(&childCtx)
	}
}

// cleanupAllNamespaces deletes all namespaces created as part of test contexts.
func cleanupAllNamespaces(ctx context.Context) error {
	kubeClient, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return errors.Wrap(err, "build Kubernetes clientset")
	}
	namespaces, err := kubeClient.CoreV1().Namespaces().List(ctx, metav1.ListOptions{
		LabelSelector: "gpe-operator-test=true",
	})
	if err != nil {
		return errors.Wrap(err, "delete namespaces by label")
	}
	for _, ns := range namespaces.Items {
		fmt.Println("delete ns", ns)
		if err := kubeClient.CoreV1().Namespaces().Delete(ctx, ns.Name, metav1.DeleteOptions{}); err != nil {
			fmt.Fprintf(os.Stderr, "deleting namespace %q failed: %s\n", ns.Name, err)
		}
	}
	return nil
}
