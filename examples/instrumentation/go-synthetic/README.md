# go-synthetic

`go-synthetic` is a toy application that emits example Prometheus metrics and exemplars.

Metrics and behaviour are synthetic. `go-synthetic` program allocates some memory,
burns some CPU and optionally (see `--*-count` flags) provide all possible metric
types (including exotic OpenMetrics types and Native Histograms).

It's used for testing and validation purposes, but also can be used to demo and debug
Prometheus monitoring infrastructure.

## Running on Kubernetes

If running managed-collection on a Kubernetes cluster, the `go-synthetic` can be
deployed and monitored by:

```bash
kubectl apply -f ./go-synthetic.yaml
```
