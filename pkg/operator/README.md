# Operator

See the [binary documentation](../../cmd/operator/README.md) for deployment instructions.

## Testing

The operator has an end-to-end test suite to run functional tests against a real
Kubernetes cluster.

To run the tests a kubeconfig pointing to a GKE cluster is required. This is generally
already taken care of while setting up a GKE cluster
([instructions](https://cloud.google.com/kubernetes-engine/docs/how-to/creating-a-zonal-cluster)).
Use `kubectl config {current,set}-context` to verify or change which cluster the tests will
execute against.

The tests require that the CRD definition and ClusterRole `gmp-system:collector` already
exist in the cluster. (They are part of deploying the operator itself, we make this manual
for tests to not unknowingly deploy resources with cluster-wide effects.)
All other resources are created and cleaned up by the test suite. To setup the resources:

```bash
kubectl apply -f ../../cmd/operator/deploy/operator/crds.yaml
kubectl apply -f ../../cmd/operator/deploy/operator/clusterrole.yaml
kubectl apply -f ../../cmd/operator/deploy/operator/priority_class.yaml
```

The tests verify the metric data written into GCM, for which information about the
GKE cluster must be provided. Execute the tests with:

```bash
go test ./e2e/ \
    --project-id=$PROJECT_ID --cluster=$CLUSTER_NAME --location=$LOCATION
```

### Credentials

Tests by default assume to run in a GKE cluster and that they can infer sufficient
credentials from the environment.

If that's not the case the `--skip-gcm` flag disables tests paths that require
connectivity to the GMP and GCM backends.

Alternatively, the `--gcp-service-account=<filepath>` flag allows providing a
GCP service account credentials file which is used for deployed components instead
of relying on the environment. The service account needs permission to read and write
metric data against the `--project-id`.
Running the test on GKE with and without this option provides more comprehensive
coverage.

## Code Generation

To update generated code when changes to Custom Resource Definitions are made run:

```bash
make codegen
make crds
```

The generated CRD YAMLs may require manual editing. Especially consider whether fields
are correctly marked as required or not.