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

	"go.uber.org/zap/zapcore"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
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

	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/deployutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/e2e/kubeutil"
	"github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
)

const (
	collectorManifest = "../cmd/operator/deploy/operator/10-collector.yaml"

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
	deployOperator    bool
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
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	return client.New(kubeconfig, client.Options{
		Scheme: scheme,
	})
}

func setRESTConfigDefaults(restConfig *rest.Config) error {
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
	return rest.SetKubernetesDefaults(restConfig)
}

// TestMain injects custom flags and adds extra signal handling to ensure testing
// namespaces are cleaned after tests were executed.
func TestMain(m *testing.M) {
	cleanupOnly := false
	flag.StringVar(&projectID, "project-id", "", "The GCP project to write metrics to.")
	flag.StringVar(&cluster, "cluster", "", "The name of the Kubernetes cluster that's tested against.")
	flag.StringVar(&location, "location", "", "The location of the Kubernetes cluster that's tested against.")
	flag.BoolVar(&skipGCM, "skip-gcm", false, "Skip validating GCM ingested points.")
	flag.StringVar(&gcpServiceAccount, "gcp-service-account", "", "Path to GCP service account file for usage by deployed containers.")
	flag.BoolVar(&portForward, "port-forward", true, "Whether to port-forward Kubernetes HTTP requests.")
	flag.BoolVar(&leakResources, "leak-resources", true, "If set, prevents deleting resources. Useful for debugging.")
	flag.BoolVar(&cleanup, "cleanup-resources", true, "If set, cleans resources before running tests.")
	flag.BoolVar(&cleanupOnly, "cleanup-resources-only", cleanupOnly, "If set, cleans resources and then exits.")
	flag.BoolVar(&deployOperator, "deploy-operator", false, "If set, deploys the operator image in the manifest.")

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
	if err != nil {
		fmt.Fprintln(os.Stderr, "Loading kubeconfig failed:", err)
		os.Exit(1)
	}
	if err := setRESTConfigDefaults(kubeconfig); err != nil {
		fmt.Fprintln(os.Stderr, "Setting REST config defaults failed:", err)
		os.Exit(1)
	}

	c, err := newClient()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build Kubernetes client:", err)
		os.Exit(1)
	}

	if cleanup || cleanupOnly {
		fmt.Fprintln(os.Stdout, "cleaning resources before tests...")
		if err := cleanupResources(context.Background(), kubeconfig, c, ""); err != nil {
			fmt.Fprintln(os.Stderr, "cleaning up failed:", err)
			os.Exit(1)
		}
		if cleanupOnly {
			fmt.Fprintln(os.Stderr, "cleaning up finished")
			os.Exit(0)
		}
	}

	ctx := context.Background()
	if err := deployutil.DeployGlobalResources(ctx, c); err != nil {
		fmt.Fprintln(os.Stderr, "create global resources:", err)
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
	if leakResources {
		return
	}
	fmt.Fprintln(os.Stdout, "cleaning up abandoned resources...")
	if err := cleanupResources(context.Background(), kubeconfig, c, ""); err != nil {
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

	namespace, pubNamespace, userNamespace string

	kClient kubeutil.DelegatingClient
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
	userNamespace := fmt.Sprintf("%s-user", namespace)

	tctx := &OperatorContext{
		T:             t,
		namespace:     namespace,
		pubNamespace:  pubNamespace,
		userNamespace: userNamespace,
	}
	tctx.kClient = kubeutil.NewLabelWriterClient(c, tctx.getSubTestLabels())
	t.Cleanup(func() {
		if !leakResources {
			if err := cleanupResourcesInNamespaces(ctx, kubeconfig, tctx.Client(), []string{namespace, pubNamespace, userNamespace}, tctx.getSubTestLabelValue()); err != nil {
				t.Fatalf("unable to cleanup resources: %s", err)
			}
		}
		cancel()
	})

	if err := createBaseResources(tctx, ctx, tctx.Client(), namespace, pubNamespace, userNamespace, tctx.GetOperatorTestLabelValue()); err != nil {
		t.Fatalf("create resources: %s", err)
	}

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

func (tctx *OperatorContext) GetOperatorTestLabelValue() string {
	return strings.SplitN(tctx.T.Name(), "/", 2)[0]
}

type commonDeployOption interface {
	deployutil.DeployOption
	deployutil.DeployLocalOption
}

// createBaseResources creates resources the operator requires to exist already.
// These are resources which don't depend on runtime state and can thus be deployed
// statically, allowing to run the operator without critical write permissions.
func createBaseResources(t testing.TB, ctx context.Context, kubeClient client.Client, opNamespace, publicNamespace, userNamespace, labelValue string) error {
	if err := createGCPSecretResources(ctx, kubeClient, opNamespace); err != nil {
		return err
	}
	commonDeployOpts := []commonDeployOption{
		deployutil.WithOperatorNamespace(opNamespace),
		deployutil.WithPublicNamespace(publicNamespace),
		deployutil.WithUserNamespace(userNamespace),
		deployutil.WithLabels(testLabel, labelValue),
		deployutil.WithMeta(projectID, cluster, location),
	}
	if !deployOperator {
		var deployLocalOpts []deployutil.DeployLocalOption = make([]deployutil.DeployLocalOption, len(commonDeployOpts))
		for i, d := range commonDeployOpts {
			deployLocalOpts[i] = d
		}
		return deployutil.DeployLocalOperator(t, ctx, kubeconfig, kubeClient, append(deployLocalOpts, deployutil.WithPortForward(portForward))...)
	}
	var deployOpts []deployutil.DeployOption = make([]deployutil.DeployOption, len(commonDeployOpts))
	for i, d := range commonDeployOpts {
		deployOpts[i] = d
	}
	return deployutil.DeployOperator(t, ctx, kubeconfig, kubeClient, deployOpts...)
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

func (tctx *OperatorContext) RestConfig() *rest.Config {
	return kubeconfig
}

func (tctx *OperatorContext) Client() client.Client {
	return tctx.kClient
}

// subtest derives a new test function from a function accepting a test context.
func (tctx *OperatorContext) subtest(f func(context.Context, *OperatorContext)) func(*testing.T) {
	return func(t *testing.T) {
		ctx := context.TODO()
		childCtx := *tctx
		childCtx.T = t
		childCtx.kClient = kubeutil.NewLabelWriterClient(tctx.kClient.Base(), childCtx.getSubTestLabels())
		t.Cleanup(func() {
			if leakResources {
				return
			}
			t.Log("cleaning up resources...")
			if err := cleanupResourcesInNamespaces(ctx, kubeconfig, childCtx.Client(), []string{tctx.namespace, tctx.pubNamespace}, childCtx.getSubTestLabelValue()); err != nil {
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

func isNamespaced(kubeClient client.Client, gvk schema.GroupVersionKind) (bool, error) {
	apiVersion, kind := gvk.ToAPIVersionAndKind()
	obj := metav1.PartialObjectMetadata{
		TypeMeta: metav1.TypeMeta{
			APIVersion: apiVersion,
			Kind:       kind,
		},
	}
	return kubeClient.IsObjectNamespaced(&obj)
}

func labelSelector(labelName, labelValue string) (labels.Selector, error) {
	if labelValue == "" {
		req, err := labels.NewRequirement(labelName, selection.Exists, []string{})
		if err != nil {
			return nil, err
		}
		return labels.NewSelector().Add(*req), nil
	}
	req, err := labels.NewRequirement(labelName, selection.Equals, []string{labelValue})
	if err != nil {
		return nil, err
	}
	return labels.NewSelector().Add(*req), nil
}

func cleanupResource(ctx context.Context, kubeClient client.Client, gvk schema.GroupVersionKind, labelValue, namespace string) error {
	labelSelector, err := labelSelector(testLabel, labelValue)
	if err != nil {
		return err
	}
	listOpts := client.ListOptions{
		LabelSelector: labelSelector,
	}
	if namespace != "" {
		listOpts.Namespace = namespace
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
		// Ignore meta-resource types.
		if errors.Is(err, &meta.NoKindMatchError{GroupKind: gvk.GroupKind()}) {
			return nil
		}
		return fmt.Errorf("unable to delete %s: %w", gvk.String(), err)
	}
	return nil
}

// cleanupResources cleans all resources created by tests. If no label value is provided,
// then all resources with the label are removed.
func cleanupResources(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, labelValue string) error {
	namespaces, err := getNamespaces(ctx, kubeClient)
	if err != nil {
		return err
	}
	return cleanupResourcesInNamespaces(ctx, restConfig, kubeClient, namespaces, labelValue)
}

func cleanupResourcesInNamespaces(ctx context.Context, restConfig *rest.Config, kubeClient client.Client, namespaces []string, labelValue string) error {
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
		// We don't care about resources that we can't even manage.
		if !kubeClient.Scheme().IsGroupRegistered(gvk.Group) {
			continue
		}

		namespaced, err := isNamespaced(kubeClient, gvk)
		if err != nil {
			// Ignore meta-resource types.
			if errors.Is(err, &meta.NoKindMatchError{GroupKind: gvk.GroupKind()}) {
				continue
			}
			errs = append(errs, err)
		}
		if namespaced {
			if labelValue == "" {
				// Skip because deleting the namespace will delete the resource.
				continue
			}
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
	labelSelector, err := labelSelector(testLabel, labelValue)
	if err != nil {
		return err
	}
	namespaceList := corev1.NamespaceList{}
	if err := kubeClient.List(ctx, &namespaceList, &client.ListOptions{
		LabelSelector: labelSelector,
	}); err != nil {
		return err
	}
	for _, namespace := range namespaceList.Items {
		if err := kubeClient.Delete(ctx, &namespace, &client.DeleteOptions{}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
