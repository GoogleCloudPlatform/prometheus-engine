# GPE Collector

This repository contains the gpe-operator for managed collection of Prometheus metrics
on GKE as well as the export library to convert and write Prometheus data to
Google Cloud Monitoring (GCM)

## Operator

To update generated code when changes to Custom Resource Definitions are made run:

```bash
hack/update-codegen.sh
```

### Run

Create or update cluster resources required by the operator.

```bash
kubectl apply -f deploy/operator/
```

Run the operator locally (requires active kubectl context to have all permissions
the operator needs):

```bash
go run cmd/operator/main.go
```

Setup the builtin monitoring stack to collect cluster-level metrics::

```bash
kubectl apply -f deploy/ --recursive
```

The operator updates the configuration of all collectors after which they start
scraping various metric endpoints.

Verify by port-forwarding an arbitrary collector and inspect its UI. You should
see various targets being scraped successfully.

```bash
kubectl -n gpe-system port-forward --address 0.0.0.0 collector 9090
```

Go to `http://localhost:9090/targets`.

### Teardown

Simply stop running the operator locally and remove all manifests in the cluster with:

```bash
kubectl delete -f deploy/ --recursive
```

### Testing

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
kubectl apply -f deploy/clusterrole.yaml
kubectl apply -f deploy/crds.yaml
```

The tests verify the metric data written into GCM, for which information about the
GKE cluster must be provided. Execute the tests with:

```bash
go test ./pkg/operator/e2e/ \
    --project=$PROJECT_ID --location=$CLUSTER_LOCATION --cluster=$CLUSTER_NAME
```