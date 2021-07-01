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

The tests require that the CRD definition and ClusterRole `gpe-system:collector` already
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
    --project-id=$PROJECT_ID --cluster=$CLUSTER_NAME
```

## Code Generation

To update generated code when changes to Custom Resource Definitions are made run:

```bash
hack/update-codegen.sh
```