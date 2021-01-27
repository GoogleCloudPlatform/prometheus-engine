// Package e2e contains tests that validate the behavior of gpe-operator against a cluster.
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
	"path/filepath"
	"syscall"
	"testing"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
)

var kubeconfig *rest.Config

func TestMain(m *testing.M) {
	var configPath string
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&configPath, "kubeconfig", filepath.Join(home, ".kube", "config"), "Path to the kubeconfig file.")
	} else {
		flag.StringVar(&configPath, "kubeconfig", "", "Path to the kubeconfig file.")
	}
	flag.Parse()

	var err error
	kubeconfig, err = clientcmd.BuildConfigFromFlags("", configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Building kubeconfig failed:", err)
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

func TestCollectorDeployment(t *testing.T) {
	tctx := newTestContext(t)

	t.Run("deployed", tctx.subtest(testCollectorDeployed))
	t.Run("self-monitoring", tctx.subtest(testCollectorSelfMonitoring))
}

// TestCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(t *testContext) {
	t.Log("TODO")
}

func testCollectorSelfMonitoring(t *testContext) {
	t.Log("TODO")
}
