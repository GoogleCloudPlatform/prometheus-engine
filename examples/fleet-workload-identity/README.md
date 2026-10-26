# GMP on Fleet clusters with Workload Identity Federation

This [Kustomize](https://kustomize.io/) overlay deploys Managed Service for Prometheus managed collection on a non-GKE cluster that's registered to a [Fleet](https://cloud.google.com/kubernetes-engine/fleet-management/docs) with Workload Identity enabled (for example, an attached or on-prem cluster). It patches the released `manifests/setup.yaml` and `manifests/operator.yaml` so that:

* the operator gets the cluster's project, name, and location (`--project-id`, `--cluster`, `--location`), which it can't discover without a metadata server, and
* the collector and rule-evaluator authenticate with a projected Kubernetes service account token and an `external_account` credential, through the Fleet membership's Workload Identity provider, instead of a service account key.

## Prerequisites

1. Register the cluster with Workload Identity enabled:

   ```bash
   gcloud container fleet memberships register MEMBERSHIP_NAME \
     --project=PROJECT_ID \
     --location=global \
     --context=KUBECONTEXT \
     --enable-workload-identity \
     --has-private-issuer
   ```

2. Grant the collector's Kubernetes service account (also used by the rule-evaluator) permission to write and query metrics:

   ```bash
   for role in roles/monitoring.metricWriter roles/monitoring.viewer; do
     gcloud projects add-iam-policy-binding PROJECT_ID \
       --member="principal://iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/PROJECT_ID.svc.id.goog/subject/ns/gmp-system/sa/collector" \
       --role="$role"
   done
   ```

## Deploy

Install the CRDs first, so the `OperatorConfig` in the overlay can be applied:

```bash
kubectl apply --server-side -f https://raw.githubusercontent.com/GoogleCloudPlatform/prometheus-engine/v0.19.4-rc.0/manifests/setup.yaml
kubectl wait --for=condition=established --timeout=60s crd/operatorconfigs.monitoring.googleapis.com
```

The overlay uses four placeholders. Substitute them when rendering, limiting `envsubst` to these variables so the rest of the manifests are untouched:

```bash
export PROJECT_ID=my-project
export MEMBERSHIP_NAME=my-cluster        # also used as the "cluster" label
export MEMBERSHIP_LOCATION=global        # the Fleet membership's location
export LOCATION=us-central1              # where metrics are stored; a GCP region or zone

kubectl kustomize examples/fleet-workload-identity \
  | envsubst '${PROJECT_ID} ${MEMBERSHIP_NAME} ${MEMBERSHIP_LOCATION} ${LOCATION}' \
  | kubectl apply --server-side -f -
```

Then configure scraping with `PodMonitoring` resources as usual, for example [`examples/pod-monitoring.yaml`](../pod-monitoring.yaml). If creating monitoring resources right after installing fails with a webhook certificate error, wait a few seconds for the operator to finish starting and retry.

## Customization

* **Service account impersonation** (for example, with VPC Service Controls): add `"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/GSA_EMAIL:generateAccessToken"` to `key.json` in `wif-credentials.yaml`. Then grant `GSA_EMAIL` the monitoring roles on `PROJECT_ID`, and let the principal above impersonate it (the principal itself then no longer needs the monitoring roles):

  ```bash
  for role in roles/monitoring.metricWriter roles/monitoring.viewer; do
    gcloud projects add-iam-policy-binding PROJECT_ID \
      --member="serviceAccount:GSA_EMAIL" --role="$role"
  done
  gcloud iam service-accounts add-iam-policy-binding GSA_EMAIL \
    --member="principal://iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/PROJECT_ID.svc.id.goog/subject/ns/gmp-system/sa/collector" \
    --role="roles/iam.workloadIdentityUser"
  ```
* **Pod scheduling**: add Kustomize patches for tolerations or node selectors on the `gmp-operator`, `rule-evaluator`, and `alertmanager` workloads. The collector `DaemonSet` already tolerates all taints.

> Don't apply this overlay to GKE clusters with managed collection enabled; GKE already runs these components.
