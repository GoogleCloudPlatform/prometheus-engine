# GPE Collector

This repository contains the gpe-operator for managed collection of Prometheus metrics
on GKE as well as the export library to convert and write Prometheus data to
Google Cloud Monitoring (GCM)

## Operator

To update generated code when changes to Custom Resource Definitions are made run:

```bash
hack/update-codegen.sh
```

Create or update cluster resources required by the operator.

```bash
kubectl apply -f deploy/manifests.yaml
```

Run the operator locally (requires active kubectl context to have all permissions
the operator needs):

```bash
go run cmd/operator/main.go
```

Create a Service and ServiceMonitoring for the GPE collector agents themselves:

```bash
kubectl apply -f example/svcmon.yaml
```

The operator updates the configuration of all collectors after which they start
scraping themselves.

Verify by port-forwarding the collector service and inspecting the UI of a
random collector:

```bash
kubectl -n gpe-system port-forward --address 0.0.0.0 svc/collector 9090
```

Go to `http://localhost:9090/targets`.


### Testing

The operator has an end-to-end test suite to run functional tests against a real
Kubernetes cluster.

To run the tests a kubeconfig is required. This is generally already taken care of while
setting up `kubectl`. Use `kubectl config {get,set}-context` to verify or change which cluster
the tests will execute against.

The tests require that the ClusterRole `gpe-system:collector` cluster role exists in
the cluster. All other resources are created and cleaned up by the test suite. To
add the ClusterRole run:

```bash
kubectl apply -f deploy/clusterrole.yaml
```

Execute the tests with:

```bash
go test ./pkg/operator/e2e/
```