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
	"time"

	gcm "cloud.google.com/go/monitoring/apiv3/v2"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
	gcmpb "google.golang.org/genproto/googleapis/monitoring/v3"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	// Blank import required to register GCP auth handlers to talk to GKE clusters.
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"

	"github.com/google/gpe-collector/pkg/operator"
	monitoringv1alpha1 "github.com/google/gpe-collector/pkg/operator/apis/monitoring/v1alpha1"
)

var (
	kubeconfig *rest.Config
	project    string
	cluster    string
	location   string
)

func TestMain(m *testing.M) {
	var configPath string
	if home := homedir.HomeDir(); home != "" {
		flag.StringVar(&configPath, "kubeconfig", filepath.Join(home, ".kube", "config"), "Path to the kubeconfig file.")
	} else {
		flag.StringVar(&configPath, "kubeconfig", "", "Path to the kubeconfig file.")
	}
	flag.StringVar(&project, "project", "", "The GCP project to write metrics to.")
	flag.StringVar(&location, "location", "", "The location metric data is written to (generally the location of the cluster).")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")

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

	// We could simply verify that the full collection chain works once. But validating
	// more fine-grained stages makes debugging a lot easier.
	t.Run("deployed", tctx.subtest(testCollectorDeployed))
	t.Run("self-monitoring", tctx.subtest(testCollectorSelfMonitoring))
}

// testCollectorDeployed does a high-level verification on whether the
// collector is deployed to the cluster.
func testCollectorDeployed(ctx context.Context, t *testContext) {
	err := wait.Poll(time.Second, 3*time.Minute, func() (bool, error) {
		ds, err := t.kubeClient.AppsV1().DaemonSets(t.namespace).Get(ctx, operator.CollectorName, metav1.GetOptions{})
		if err != nil {
			return false, errors.Errorf("getting collector DaemonSet failed: %s", err)
		}
		if ds.Status.DesiredNumberScheduled == 0 {
			return false, errors.New("expected at least one desired collector scheduled, got 0")
		}
		return ds.Status.NumberReady == ds.Status.DesiredNumberScheduled, nil
	})
	if err != nil {
		t.Fatalf("Waiting for DaemonSet deployment failed: %s", err)
	}
}

// testCollectorSelfMonitoring sets up service monitoring of the collector itself
// and waits for samples to become available in Cloud Monitoring.
func testCollectorSelfMonitoring(ctx context.Context, t *testContext) {
	// We rely on the default service account of the collector having write access to GCM.
	// This means it will only work on GKE where the default service account has the default
	// permissions.
	// For support of other environments, the operator will need to be extended by flags
	// to inject different service accounts or secrets.

	// The project, location and cluster name in which we look for the metric data must
	// be provided by the user. Check this only in this test so tests that don't need these
	// flags can still be run without them.
	// They can be configured on the operator but our current test setup (targeting GKE)
	// relies on the operator inferring them from the environment.
	if project == "" {
		t.Fatalf("no project specified (--project flag)")
	}
	if location == "" {
		t.Fatalf("no location specified (--location flag)")
	}
	if cluster == "" {
		t.Fatalf("no cluster name specified (--cluster flag)")
	}

	// The collector is an application just like any other. So we create a Service and
	// ServiceMonitoring for it.
	// The operator should configure the collector to scrape itself and its metrics
	// should show up in Cloud Monitoring shortly after.
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			// Use a non-standard name so we can be sure that the metric job label is set
			// from the service name rather than the numberous other places "collector" is used.
			Name: "collector-service",
			Labels: map[string]string{
				"app": operator.CollectorName,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": operator.CollectorName,
			},
			Ports: []corev1.ServicePort{
				{Name: "prometheus-http", Port: 9090},
				{Name: "reloader-http", Port: 9091},
			},
		},
	}
	_, err := t.kubeClient.CoreV1().Services(t.namespace).Create(ctx, svc, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector Servic e: %s", err)
	}

	// Setup scraping of two ports of the collectors.
	svcmon := &monitoringv1alpha1.ServiceMonitoring{
		ObjectMeta: metav1.ObjectMeta{
			Name: operator.CollectorName,
		},
		Spec: monitoringv1alpha1.ServiceMonitoringSpec{
			Selector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": operator.CollectorName,
				},
			},
			Endpoints: []monitoringv1alpha1.ScrapeEndpoint{
				{Port: intstr.FromString("prometheus-http"), ScrapeInterval: "5s"},
				{Port: intstr.FromString("reloader-http"), ScrapeInterval: "5s"},
			},
		},
	}
	_, err = t.operatorClient.MonitoringV1alpha1().ServiceMonitorings(t.namespace).Create(ctx, svcmon, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("create collector ServiceMonitoring: %s", err)
	}

	// Wait for metric data to show up in Cloud Monitoring.
	metricClient, err := gcm.NewMetricClient(ctx)
	if err != nil {
		t.Fatalf("Create GCM metric client: %s", err)
	}
	defer metricClient.Close()

	pods, err := t.kubeClient.CoreV1().Pods(t.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=" + operator.CollectorName,
	})
	if err != nil {
		t.Fatalf("List collector pods: %s", err)
	}

	// See whether the `up` metric is written for each pod/port combination. It is set to 1 by
	// Prometheus on successful scraping of the target. Thereby we validate service discovery
	// configuration, config reload handling, as well as data export are correct.
	//
	// Make a single query for each pod/port combo as this is simpler than untangling the result
	// of a single query.
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	for _, pod := range pods.Items {
		for _, port := range []string{"prometheus-http", "reloader-http"} {
			t.Logf("Poll up metric for pod %q and port %q", pod.Name, port)

			err = wait.PollImmediateUntil(3*time.Second, func() (bool, error) {
				now := time.Now()

				// Validate the majority of labels being set correctly by filtering along them.
				iter := metricClient.ListTimeSeries(ctx, &gcmpb.ListTimeSeriesRequest{
					Name: fmt.Sprintf("projects/%s", project),
					Filter: fmt.Sprintf(`
				resource.type = "prometheus_target" AND
				resource.labels.project_id = "%s" AND
				resource.labels.cluster = "%s" AND
				resource.labels.namespace = "%s" AND
				resource.labels.job = "%s" AND
				resource.labels.instance = "%s:%s" AND
				metric.type = "external.googleapis.com/gpe/up/gauge"
				`,
						project, cluster, t.namespace, "collector-service", pod.Name, port,
					),
					Interval: &gcmpb.TimeInterval{
						EndTime:   timestamppb.New(now),
						StartTime: timestamppb.New(now.Add(-10 * time.Second)),
					},
				})
				series, err := iter.Next()
				if err == iterator.Done {
					t.Logf("No data, retrying...")
					return false, nil
				} else if err != nil {
					return false, errors.Wrap(err, "querying metrics failed")
				}
				if v := series.Points[len(series.Points)-1].Value.GetDoubleValue(); v != 1 {
					t.Logf("Up still %v, retrying...", v)
					return false, nil
				}
				// We expect exactly one result.
				series, err = iter.Next()
				if err != iterator.Done {
					return false, errors.Errorf("expected iterator to be done but got error %q and series %v", err, series)
				}
				return true, nil
			}, ctx.Done())
			if err != nil {
				t.Fatalf("Waiting for collector metrics to appear in Cloud Monitoring failed: %s", err)
			}
		}
	}
}
