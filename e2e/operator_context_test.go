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
	"net/http"
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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	collectorManifest    = "../cmd/operator/deploy/operator/10-collector.yaml"
	ruleEvalManifest     = "../cmd/operator/deploy/operator/11-rule-evaluator.yaml"
	alertmanagerManifest = "../cmd/operator/deploy/operator/12-alertmanager.yaml"

	testLabel = "monitoring.googleapis.com/prometheus-test"
)

var (
	startTime    = time.Now().UTC()
	globalLogger = zap.New(zap.Level(zapcore.DebugLevel))

	kubeconfig        *rest.Config
	projectID         string
	cluster           string
	location          string
	skipGCM           bool
	gcpServiceAccount string
	portForward       bool
	leakResources     bool
	cleanup           bool
)

func init() {
	ctrl.SetLogger(globalLogger)

	// Allow tests to run on random webhook ports to allow parallelism.
	webhook.DefaultPort = 0
}

func newClient() (client.Client, error) {
	scheme, err := operator.NewScheme()
	if err != nil {
		return nil, fmt.Errorf("operator schema: %w", err)
	}

	return client.New(kubeconfig, client.Options{
		Scheme: scheme,
	})
}

func setRESTConfigDefaults(restConfig *rest.Config) {
	// https://github.com/kubernetes/client-go/issues/657
	// https://github.com/kubernetes/client-go/issues/1159
	// https://github.com/kubernetes/kubectl/blob/6fb6697c77304b7aaf43a520d30cb17563c69886/pkg/cmd/util/kubectl_match_version.go#L115
	defaultGroupVersion := &schema.GroupVersion{Group: "", Version: "v1"}
	if restConfig.GroupVersion == nil {
		restConfig.GroupVersion = defaultGroupVersion
	}
	if restConfig.APIPath == "" {
		restConfig.APIPath = "/api"
	}
	if restConfig.NegotiatedSerializer == nil {
		restConfig.NegotiatedSerializer = scheme.Codecs.WithoutConversion()
	}
	rest.SetKubernetesDefaults(restConfig)
}

// TestMain injects custom flags and adds extra signal handling to ensure testing
// namespaces are cleaned after tests were executed.
func TestMain(m *testing.M) {
	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")
	flag.BoolVar(&portForward, "port-forward", true, "Whether to port-forward Kubernetes HTTP requests.")
	flag.BoolVar(&leakResources, "leak-resources", true, "If set, prevents deleting resources. Useful for debugging.")
	flag.BoolVar(&cleanup, "cleanup-resources", true, "If set, cleans resources before running tests.")

	flag.Parse()

	if projectID == "" && cluster == "" && location == "" {
		clusterMeta, err := ExtractGKEClusterMeta()
		if err != nil {
			fmt.Fprintln(os.Stdout, "Unable to load GKE Cluster meta:", err)
		} else {
			projectID = clusterMeta.ProjectID
			cluster = clusterMeta.Cluster
			location = clusterMeta.Location
		}
	}

	var err error
	kubeconfig, err = ctrl.GetConfig()
	setRESTConfigDefaults(kubeconfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Loading kubeconfig failed:", err)
		os.Exit(1)
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build Kubernetes client:", err)
		os.Exit(1)
	}

	if cleanup {
		fmt.Fprintln(os.Stdout, "cleaning resources before tests...")
		if err := cleanupResources(context.Background(), kubeconfig, c, ""); err != nil {
			fmt.Fprintln(os.Stderr, "cleaning up failed:", err)
			os.Exit(1)
		}
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
	if leakResources {
		return
	}
	fmt.Fprintln(os.Stdout, "cleaning up abandoned resources...")
	if err := cleanupResources(context.Background(), kubeconfig, c, testLabel); err != nil {
		fmt.Fprintln(os.Stderr, "cleaning up failed:", err)
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

	kubeClient TrackingClient
}

func newOperatorContext(t *testing.T) *OperatorContext {
	c, err := newClient()
	if err != nil {
		t.Fatalf("Build Kubernetes client: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	// Create a namespace per test and run. This is to ensure that repeated runs of
	// tests don't falsify results. Either by old test resources not being cleaned up
	// (less likely) or metrics observed in GCP being from a previous run (more likely).
	namespace := fmt.Sprintf("gmp-test-%s-%s", strings.ToLower(t.Name()), startTime.Format("20060102-150405"))
	pubNamespace := fmt.Sprintf("%s-pub", namespace)

	tctx := &OperatorContext{
		T:            t,
		namespace:    namespace,
		pubNamespace: pubNamespace,
	}
	tctx.kubeClient = NewTrackingClient(NewLabelWriterClient(c, tctx.getSubTestLabels()))
	t.Cleanup(func() {
		if !leakResources {
			if err := tctx.kubeClient.Cleanup(ctx); err != nil {
				t.Fatalf("unable to cleanup resources: %s", err)
			}
		}
		cancel()
	})

	if err := createBaseResources(ctx, tctx.Client(), namespace, pubNamespace); err != nil {
		t.Fatalf("create resources: %s", err)
	}

	var httpClient *http.Client
	if portForward {
		var err error
		httpClient, err = PortForwardClient(t, kubeconfig, tctx.Client())
		if err != nil {
			t.Fatalf("creating HTTP client: %s", err)
		}
	}

	op, err := operator.New(globalLogger, kubeconfig, operator.Options{
		ProjectID:         projectID,
		Cluster:           cluster,
		Location:          location,
		OperatorNamespace: tctx.namespace,
		PublicNamespace:   tctx.pubNamespace,
		// Pick a random available port.
		ListenAddr:          ":0",
		CollectorHTTPClient: httpClient,
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

// createOperatorConfig creates an OperatorConfig, defaulting fields that aren't provided.
func (tctx *OperatorContext) createOperatorConfigFrom(ctx context.Context, opCfg monitoringv1.OperatorConfig) {
	if opCfg.Name == "" {
		opCfg.Name = operator.NameOperatorConfig
	}
	if opCfg.Namespace == "" {
		opCfg.Namespace = tctx.pubNamespace
	}

	if gcpServiceAccount != "" {
		opCfg.Collection.Credentials = &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "user-gcp-service-account",
			},
			Key: "key.json",
		}
	}

	// Create a copy which wil represents the current object if it already exists.
	obj := opCfg.DeepCopy()
	if _, err := controllerutil.CreateOrUpdate(ctx, tctx.Client(), obj, func() error {
		// For updates, we need the resource version in the object meta to match. Replace everything else.
		opCfg.ObjectMeta = obj.ObjectMeta
		*obj = opCfg
		return nil
	}); err != nil {
		tctx.Fatalf("create OperatorConfig: %s", err)
	}
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
func createBaseResources(ctx context.Context, kubeClient client.Client, opNamespace, publicNamespace string) error {
	if err := createNamespaces(ctx, kubeClient, opNamespace, publicNamespace); err != nil {
		return err
	}

	if err := createGCPSecretResources(ctx, kubeClient, opNamespace); err != nil {
		return err
	}
	if err := createCollectorResources(ctx, kubeClient, opNamespace); err != nil {
		return err
	}
	if err := createAlertmanagerResources(ctx, kubeClient, opNamespace); err != nil {
		return err
	}
	return nil
}

func createNamespaces(ctx context.Context, kubeClient client.Client, opNamespace, publicNamespace string) error {
	if err := kubeClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: opNamespace,
		},
	}); err != nil {
		return err
	}
	if err := kubeClient.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: publicNamespace,
		},
	}); err != nil {
		return err
	}
	return nil
}

func createGCPSecretResources(ctx context.Context, kubeClient client.Client, namespace string) error {
	if gcpServiceAccount != "" {
		b, err := os.ReadFile(gcpServiceAccount)
		if err != nil {
			return fmt.Errorf("read GCP service account file: %w", err)
		}
		if err = kubeClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "user-gcp-service-account",
				Namespace: namespace,
			},
			Data: map[string][]byte{
				"key.json": b,
			},
		}); err != nil {
			return fmt.Errorf("create GCP service account secret: %w", err)
		}
	}
	return nil
}

func parseResourceYAML(b []byte) (runtime.Object, error) {
	// Ignore returned schema. It's redundant since it's already encoded in obj.
	obj, _, err := scheme.Codecs.UniversalDeserializer().Decode(b, nil, nil)
	return obj, err
}

func createCollectorResources(ctx context.Context, kubeClient client.Client, namespace string) error {
	if err := kubeClient.Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      operator.NameCollector,
			Namespace: namespace,
		},
	}); err != nil {
		return err
	}

	// The cluster role expected to exist already.
	const clusterRoleName = operator.DefaultOperatorNamespace + ":" + operator.NameCollector

	if err := kubeClient.Create(ctx, &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: clusterRoleName + ":" + namespace,
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
				Namespace: namespace,
				Name:      operator.NameCollector,
			},
		},
	}); err != nil {
		return err
	}

	collectorBytes, err := os.ReadFile(collectorManifest)
	if err != nil {
		return fmt.Errorf("read collector YAML: %w", err)
	}
	obj, err := parseResourceYAML(collectorBytes)
	if err != nil {
		return fmt.Errorf("decode collector: %w", err)
	}
	collector := obj.(*appsv1.DaemonSet)
	collector.Namespace = namespace
	if skipGCM {
		for i := range collector.Spec.Template.Spec.Containers {
			container := &collector.Spec.Template.Spec.Containers[i]
			if container.Name == "prometheus" {
				container.Args = append(container.Args, "--export.debug.disable-auth")
				break
			}
		}
	}

	if err = kubeClient.Create(ctx, collector); err != nil {
		return fmt.Errorf("create collector DaemonSet: %w", err)
	}
	return nil
}

func createAlertmanagerResources(ctx context.Context, kubeClient client.Client, namespace string) error {
	evaluatorBytes, err := os.ReadFile(ruleEvalManifest)
	if err != nil {
		return fmt.Errorf("read rule-evaluator YAML: %w", err)
	}

	obj, err := parseResourceYAML(evaluatorBytes)
	if err != nil {
		return fmt.Errorf("decode evaluator: %w", err)
	}
	evaluator := obj.(*appsv1.Deployment)
	evaluator.Namespace = namespace

	if err := kubeClient.Create(ctx, evaluator); err != nil {
		return fmt.Errorf("create rule-evaluator Deployment: %w", err)
	}

	alertmanagerBytes, err := os.ReadFile(alertmanagerManifest)
	if err != nil {
		return fmt.Errorf("read alertmanager YAML: %w", err)
	}
	for i, doc := range strings.Split(string(alertmanagerBytes), "---") {
		obj, err = parseResourceYAML([]byte(doc))
		if err != nil {
			return fmt.Errorf("deserializing alertmanager manifest: %w", err)
		}
		obj, ok := obj.(client.Object)
		if !ok {
			return fmt.Errorf("unknown object at index %d", i)
		}

		obj.SetNamespace(namespace)

		if err := kubeClient.Create(ctx, obj); err != nil {
			return fmt.Errorf("create object at index %d: %w", i, err)
		}
	}

	return nil
}

func (tctx *OperatorContext) Client() client.Client {
	return tctx.kubeClient
}

// subtest derives a new test function from a function accepting a test context.
func (tctx *OperatorContext) subtest(f func(context.Context, *OperatorContext)) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.TODO()
		childCtx := *tctx
		childCtx.T = t
		childCtx.kubeClient = NewTrackingClient(tctx.kubeClient.Base())
		t.Cleanup(func() {
			if leakResources {
				return
			}
			t.Log("cleaning up resources...")
			if err := childCtx.kubeClient.Cleanup(ctx); err != nil {
				t.Fatalf("unable to cleanup resources: %s", err)
			}
		})
		f(ctx, &childCtx)
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

func labelListOptions(labelName string) (client.ListOptions, error) {
	req, err := labels.NewRequirement(labelName, selection.Exists, []string{})
	if err != nil {
		return client.ListOptions{}, err
	}
	listOpts := client.ListOptions{
		LabelSelector: labels.NewSelector().Add(*req),
	}
	return listOpts, nil
}

func cleanupResource(ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, labelName string) error {
	listOpts, err := labelListOptions(labelName)
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
func cleanupResources(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, labelName string) error {
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return err
	}
	gvks, err := getGroupVersionKinds(discoveryClient)
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
			continue
		}
		if err := cleanupResource(ctx, kubeClient, gvk, labelName); err != nil {
			errs = append(errs, err)
		}
	}

	// DeleteAllOf does not work for namespaces, so we must delete individually.
	listOpts, err := labelListOptions(labelName)
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
