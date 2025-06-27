// Copyright 2024 Google LLC
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

// Package operator contains the Prometheus
package operator

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/GoogleCloudPlatform/prometheus-engine/collector/export"
	monitoringv1 "github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1"
	"github.com/go-logr/logr"
	arv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/util/cert"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// setupAdmissionWebhooks configures validating webhooks for the operator-managed
// custom resources and registers handlers with the webhook server.
func setupAdmissionWebhooks(ctx context.Context, logger logr.Logger, kubeClient client.Client, webhookServer *webhook.DefaultServer, opts *Options, vpaAvailable bool) error {
	// Write provided cert files.
	caBundle, err := ensureCerts(opts.OperatorNamespace, webhookServer.Options.CertDir, opts.TLSCert, opts.TLSKey, opts.CACert)
	if err != nil {
		return err
	}

	name := webhookName(opts.OperatorNamespace)

	if len(caBundle) > 0 {
		// Keep setting the caBundle, if "ensureCerts" gives us those, in the expected webhook configurations.
		// In case of not enough permissions we will keep trying with error message.
		go continuouslySetCABundle(ctx, logger, kubeClient, name, caBundle)
	}
	scheme := kubeClient.Scheme()

	// Validating webhooks.
	webhookServer.Register(
		validatePath(monitoringv1.OperatorConfigResource()),
		admission.WithCustomValidator(scheme, &monitoringv1.OperatorConfig{}, &monitoringv1.OperatorConfigValidator{
			Namespace:    opts.PublicNamespace,
			Name:         NameOperatorConfig,
			VPAAvailable: vpaAvailable,
		}),
	)
	webhookServer.Register(
		validatePath(monitoringv1.RulesResource()),
		admission.ValidatingWebhookFor(scheme, &monitoringv1.Rules{}),
	)
	webhookServer.Register(
		validatePath(monitoringv1.ClusterRulesResource()),
		admission.ValidatingWebhookFor(scheme, &monitoringv1.ClusterRules{}),
	)
	webhookServer.Register(
		validatePath(monitoringv1.GlobalRulesResource()),
		admission.ValidatingWebhookFor(scheme, &monitoringv1.GlobalRules{}),
	)
	// Defaulting webhooks.
	webhookServer.Register(
		defaultPath(monitoringv1.OperatorConfigResource()),
		admission.WithCustomDefaulter(scheme, &monitoringv1.OperatorConfig{}, &operatorConfigDefaulter{
			projectID: opts.ProjectID,
			location:  opts.Location,
			cluster:   opts.Cluster,
		}),
	)
	return nil
}

func webhookName(namespace string) string {
	return fmt.Sprintf("%s.%s.monitoring.googleapis.com", NameOperator, namespace)
}

// ensureCerts writes the cert/key files to the specified directory.
// If cert/key are not available, generate them.
func ensureCerts(operatorNamespace, dir, certEncoded, keyEncoded, caCertEncoded string) ([]byte, error) {
	var (
		crt, key, caData []byte
		err              error
	)
	if keyEncoded != "" && certEncoded != "" {
		crt, err = base64.StdEncoding.DecodeString(certEncoded)
		if err != nil {
			return nil, fmt.Errorf("decoding TLS certificate: %w", err)
		}
		key, err = base64.StdEncoding.DecodeString(keyEncoded)
		if err != nil {
			return nil, fmt.Errorf("decoding TLS key: %w", err)
		}
		if caCertEncoded != "" {
			caData, err = base64.StdEncoding.DecodeString(caCertEncoded)
			if err != nil {
				return nil, fmt.Errorf("decoding certificate authority: %w", err)
			}
		}
	} else if keyEncoded == "" && certEncoded == "" && caCertEncoded == "" {
		// Generate a self-signed pair if none was explicitly provided. It will be valid
		// for 1 year.
		// TODO(freinartz): re-generate at runtime and update the ValidatingWebhookConfiguration
		// at runtime whenever the files change.
		fqdn := fmt.Sprintf("%s.%s.svc", NameOperator, operatorNamespace)

		crt, key, err = cert.GenerateSelfSignedCertKey(fqdn, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("generate self-signed TLS key pair: %w", err)
		}
		// Use crt as the ca in the self-sign case.
		caData = crt
	} else {
		return nil, errors.New("flags key-base64 and cert-base64 must both be set")
	}
	// Create cert/key files.
	if err := os.WriteFile(filepath.Join(dir, "tls.crt"), crt, 0666); err != nil {
		return nil, fmt.Errorf("create cert file: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "tls.key"), key, 0666); err != nil {
		return nil, fmt.Errorf("create key file: %w", err)
	}
	return caData, nil
}

func validatePath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/validate/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func defaultPath(gvr metav1.GroupVersionResource) string {
	return fmt.Sprintf("/default/%s/%s/%s", gvr.Group, gvr.Version, gvr.Resource)
}

func setValidatingWebhookCABundle(ctx context.Context, kubeClient client.Client, name string, caBundle []byte) error {
	var vwc arv1.ValidatingWebhookConfiguration
	err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, &vwc)
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	for i := range vwc.Webhooks {
		vwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	return kubeClient.Update(ctx, &vwc)
}

func setMutatingWebhookCABundle(ctx context.Context, kubeClient client.Client, name string, caBundle []byte) error {
	var mwc arv1.MutatingWebhookConfiguration
	err := kubeClient.Get(ctx, client.ObjectKey{Name: name}, &mwc)
	if apierrors.IsNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}

	for i := range mwc.Webhooks {
		mwc.Webhooks[i].ClientConfig.CABundle = caBundle
	}
	return kubeClient.Update(ctx, &mwc)
}

func continuouslySetCABundle(ctx context.Context, logger logr.Logger, kubeClient client.Client, name string, caBundle []byte) {
	// Initial sleep for the client to initialize before our first calls.
	// Ideally we could explicitly wait for it.
	time.Sleep(5 * time.Second)

	for {
		if err := setValidatingWebhookCABundle(ctx, kubeClient, name, caBundle); err != nil {
			logger.Error(err, "Setting CA bundle for ValidatingWebhookConfiguration failed; retrying in 1m...")
		}
		if err := setMutatingWebhookCABundle(ctx, kubeClient, name, caBundle); err != nil {
			logger.Error(err, "Setting CA bundle for MutatingWebhookConfiguration failed; retrying in 1m...")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
	}
}

type operatorConfigDefaulter struct {
	projectID string
	location  string
	cluster   string
}

func (d *operatorConfigDefaulter) Default(_ context.Context, o runtime.Object) error {
	oc := o.(*monitoringv1.OperatorConfig)
	_ = d.update(oc)
	return nil
}

// update defaults the OperatorConfig, returning true if the OperatorConfig was updated.
func (d *operatorConfigDefaulter) update(oc *monitoringv1.OperatorConfig) bool {
	updated := false

	// Upsert projectID, location, and cluster to external labels.
	// If not present in external labels, use the values passed to the operator.
	// If present in external labels, this is effectively a no-op.
	// Do this for both collection and rule-evaluator configuration.
	var projectID, location, cluster = resolveLabels(d.projectID, d.location, d.cluster, oc.Collection.ExternalLabels)
	collectionExpected := map[string]string{
		export.KeyProjectID: projectID,
		export.KeyLocation:  location,
		export.KeyCluster:   cluster,
	}
	if oc.Collection.ExternalLabels == nil {
		oc.Collection.ExternalLabels = collectionExpected
		updated = true
	} else {
		for key, val := range collectionExpected {
			if oc.Collection.ExternalLabels[key] != val {
				oc.Collection.ExternalLabels[key] = val
				updated = true
			}
		}
	}

	projectID, location, cluster = resolveLabels(d.projectID, d.location, d.cluster, oc.Rules.ExternalLabels)
	rulesExpected := map[string]string{
		export.KeyProjectID: projectID,
		export.KeyLocation:  location,
		export.KeyCluster:   cluster,
	}
	if oc.Rules.ExternalLabels == nil {
		oc.Rules.ExternalLabels = rulesExpected
		updated = true
	} else {
		for key, val := range rulesExpected {
			if oc.Rules.ExternalLabels[key] != val {
				oc.Rules.ExternalLabels[key] = val
				updated = true
			}
		}
	}
	return updated
}
