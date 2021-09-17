# Operator

This binary is a Kubernetes operator that provides Managed Collection for Google
Cloud Prometheus Engine on Kubernetes.

## Run

Create or update cluster resources required by the operator.

```bash
kubectl apply -f deploy/operator/
```

Run the operator locally (requires active kubectl context to have all permissions
the operator needs):

```bash
go run main.go
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
kubectl -n gmp-system port-forward --address 0.0.0.0 collector 19090
```

Go to `http://localhost:19090/targets`.

## Teardown

Simply stop running the operator locally and remove all manifests in the cluster with:

```bash
kubectl delete -f deploy/ --recursive
```