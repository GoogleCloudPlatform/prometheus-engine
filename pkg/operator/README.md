# Operator

See the [binary documentation](../../cmd/operator/README.md) for deployment instructions.

## Testing

The operator has an end-to-end test suite to run functional tests against a real Kubernetes cluster.

To run the tests a kubeconfig pointing to a GKE cluster is required. This is generally already taken
care of while setting up a GKE cluster
([instructions](https://cloud.google.com/kubernetes-engine/docs/how-to/creating-a-zonal-cluster)).
Use `kubectl config {current,set}-context` to verify or change which cluster the tests will
execute against.

The easiest way to run end-to-end tests is to deploy all the operator yourself and connect to that
cluster by passing `--local-operator` to your test:

```bash
go test ./e2e/ --local-operator \
    --project-id=$PROJECT_ID --cluster=$CLUSTER_NAME --location=$LOCATION
```

To run the test with a test-deployed operator, the test expects various resources, which are part of
deploying the operator, to be installed in the cluster:

```bash
kubectl apply -f ../../cmd/operator/deploy/crds/
kubectl apply -f ../../cmd/operator/deploy/operator/00-namespace.yaml
kubectl apply -f ../../cmd/operator/deploy/operator/01-priority-class.yaml
```

The operator itself is run locally within the test suite. Thus, make sure the blocking
webhooks are not currently enabled:

```bash
kubectl delete -f ../../cmd/operator/deploy/operator/08-validatingwebhookconfiguration.yaml
kubectl delete -f ../../cmd/operator/deploy/operator/09-mutatingwebhookconfiguration.yaml
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
make regen
```

The generated CRD YAMLs may require manual editing. Especially consider whether fields
are correctly marked as required or not.
