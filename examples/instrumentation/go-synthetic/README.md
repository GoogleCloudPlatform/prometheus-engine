# go-synthetic

`go-synthetic` is a toy application that emits example Prometheus metrics and exemplars.

Metrics and behaviour are synthetic. `go-synthetic` program allocates some memory,
burns some CPU and optionally (see `--*-count` flags) provide all possible metric
types (including exotic OpenMetrics types and Native Histograms).

It's used for testing and validation purposes, but also can be used to demo and debug
Prometheus monitoring infrastructure.

## Running Locally

You can run this application locally via:

```bash
go run ./examples/instrumentation/go-synthetic/
```

Then, you can access the [`/metrics`](http://localhost:8080/metrics) endpoint. For example, via `curl`:

```bash
curl localhost:8080/metrics
```

### Authorization

The example application can be protected with various authorization methods:

#### BasicAuth

```bash
go run ./examples/instrumentation/go-synthetic/ --basic-auth-username=admin --basic-auth-password=pw
curl localhost:8080/metrics -u "admin:pw"
```

#### Authorization

```bash
go run ./examples/instrumentation/go-synthetic/ --auth-scheme=Bearer --auth-parameters=xyz
curl -H "Authorization: Bearer xyz" localhost:8080/metrics
```

## Running on Kubernetes

If running managed-collection on a Kubernetes cluster, the `go-synthetic` can be
deployed and monitored by:

```bash
kubectl apply -f ./go-synthetic.yaml
```
