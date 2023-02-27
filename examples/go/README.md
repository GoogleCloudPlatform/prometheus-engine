# Example Go application: instrumentation and manifests.

This directory contains `go-observable-app` that showcases Go instrumentation for common observability signals:

* Logging
* Prometheus Metrics with exemplars.
* OpenTelemetry tracing.

The application is in a form of HTTP client-server architecture, that mimics ping (server) and pinger (client). 

The binary is also available in a form of docker image [bplotka/go-observable-app:v0.1.0](https://hub.docker.com/r/bplotka/go-observable-app).

## Observability

See [pod-monitoring.yaml](./pod-monitoring.yaml) for example PodMonitoring resource describing how to monitor both applications.

Instrumentation means use:

* Logging: Standard error (so kubectl logs will show it).
* Metrics: Prometheus scrape endpoint is exposed.
* Traces: OTLP backend or collector is required (e.g. OTLP collector or/and [Cloud Trace](https://cloud.google.com/trace))

## Using Server

Ping server, defined in [cmd/ping/main.go](./cmd/ping/main.go) is a server that responds to HTTP requests on `/ping` path
with the `pong` response or error. 

To make observability for this server more interesting additional characteristics are configurable: 
* The 200 code response is returned with given `-success-probability`, otherwise 500 code is returned.
* Additional extra latency is added as specified in `-latency` e.g. `-latency=20%200ms` means 20% of requests
will have extra artificial 200ms of latency.

When deployment you can use `http://<address specified by -listen-address>/ping` to manually call ping server or you can follow [Client](#using-client) to setup programmatic client.

See [ping.yaml](./ping.yaml) for example Kubernetes Deployment and Service.

```bash
docker run -it bplotka/go-observable-app:v0.1.0 /bin/ping --help
Usage of /bin/ping:
  -latency string
        Encoded latency and probability of the response in format as: <probability>%<duration>,<probability>%<duration>.... (default "90%500ms,10%200ms")
  -listen-address string
        The address to listen on for HTTP requests. (default ":8080")
  -log-format string
        Log format to use. Possible options: logfmt or json (default "logfmt")
  -log-level string
        Log filtering level. Possible values: "error", "warn", "info", "debug" (default "info")
  -set-version string
        Injected version to be presented via metrics. (default "v0.1.0")
  -success-probability float
        The probability (in %) of getting a successful response (default 100)
  -trace-endpoint string
        Optional GRPC OTLP endpoint for tracing backend. Set it to 'stdout' to print traces to the output instead.
  -trace-sampling-ratio float
        Sampling ratio. Currently 1.0 is the best value to use with exemplars. (default 1)
```

## Using Client

Client allows to continuously call [Server](#using-client), so we can observe both applications over time.

See [pinger.yaml](./pinger.yaml) for example Kubernetes Deployment.

```bash
docker run -it bplotka/go-observable-app:v0.1.0 /bin/pinger --help
Usage of /bin/pinger:
  -endpoint string
        The address of pong app we can connect to and send requests. (default "http://app.demo.svc.cluster.local:8080/ping")
  -listen-address string
        The address to listen on for HTTP requests. (default ":8080")
  -log-format string
        Log format to use. Possible options: logfmt or json (default "logfmt")
  -log-level string
        Log filtering level. Possible values: "error", "warn", "info", "debug" (default "info")
  -pings-per-second int
        How many pings per second we should request (default 10)
  -set-version string
        Injected version to be presented via metrics. (default "v0.1.0")
  -trace-endpoint string
        Optional GRPC OTLP endpoint for tracing backend. Set it to 'stdout' to print traces to the output instead.
  -trace-sampling-ratio float
        Sampling ratio. Currently 1.0 is the best value to use with exemplars. (default 1)
```
## Demo

NOTE: This assumes default namespace.

Deploy 3 replicas of server, service for load balancing requests and one replica of client:

```bash
kubectl apply -f ping.yaml
kubectl apply -f pong.yaml
kubectl apply -f pod-monitoring.yaml
```

## FAQ

### What's the difference with [branch/prometheus-example-app](https://github.com/branch/prometheus-example-app)?

`go-observable-app` shows newer, production ready instrumentation patterns, for example:

* Idiomatic client_golang code (global registry is an anti-pattern).
* Manually instrumented OpenTelemetry tracing.
* Correlation of logs, metrics and traces in form of common labels, request ID and exemplars.
