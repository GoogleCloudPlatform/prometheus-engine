# GPE Collector

This repository contains the gpe-operator for managed collection of Prometheus metrics
on GKE as well as the export library to convert and write Prometheus data to
Google Cloud Monitoring (GCM)

## Operator

To update generated code when changes to Custom Resource Definitions are made run:

```bash
hack/update-codegen.sh
```

Register the ServiceMonitoring CRD with the cluster:

```bash
kubectl apply -f deploy/operator_crds.yaml
```

Run the operator locally (requires active kubectl context to have all permissions
the operator needs):

```bash
go run cmd/operator/main.go
```

Create a ServiceMonitoring resource and observe the operator reconciling it:

```bash
kubectl apply -f example/svcmon.yaml
```