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
# In a second terminal: 
curl localhost:8080/metrics -u "admin:pw"
```

#### Authorization

```bash
go run ./examples/instrumentation/go-synthetic/ --auth-scheme=Bearer --auth-parameters=xyz
# In a second terminal: 
curl -H "Authorization: Bearer xyz" localhost:8080/metrics
```

#### OAuth 2.0

```bash
go run ./examples/instrumentation/go-synthetic/ --oauth2-client-id=abc --oauth2-client-secret=xyz
# In a second terminal: 
curl "localhost:8080/token?grant_type=client_credentials&client_id=abc&client_secret=xyz"
# Fetch access token from above and use as bearer token example below:
curl -H "Authorization: Bearer DZ~9UYwD" localhost:8080/metrics
```

#### mTLS

```bash
go run ./examples/instrumentation/go-synthetic/ --tls-create-self-signed=true
# In a second terminal: 
curl -k https://localhost:8080/metrics
```

### Extended metric naming conventions (UTF-8)

`go-synthetic` application can be configured (`-metric-naming-style` and `-status-label-naming-style` flags) to expose metrics that utilize [UTF-8](https://prometheus.io/docs/guides/utf8/) Prometheus feature:

```
-metric-naming-style string
        Change the default metric names to test UTF-8 extended charset features. This option will affect all "example_*" metric names produced by this application. For example:
        - 'prometheus' style will keep the old name 'example_incoming_requests_pending'
        - 'gcm-extended' style will add /, . and - , so 'example/incoming.requests-pending'
        - 'exotic-utf-8' style will add (forbidden in GCM) exotic chars like '😂', so 'example/🗻😂/incoming.requests-pending' (default "prometheus") 
-status-label-naming-style string
        Change the default label name for example metrics with 'status' label to test UTF-8 extended charset features. For example:
        - 'prometheus' style will keep the old label name 'status'
        - 'gcm-label-extended' style will add / and . , so 'example/http.request.status'
        - 'exotic-utf-8' style will add (forbidden in GCM) exotic chars like '😂', so 'example/🗻😂/http.request-status' (default "prometheus")
```

* For example, by default strict Prometheus naming style metrics are exposed.

> NOTE: Notice the `'Accept: application/openmetrics-text;escaping=allow-utf-8'` header that needs to be part of the scrape to expose OpenMetrics without any escaping mechanism (e.g. replacing unsupported characters to _).

```
go run ./examples/instrumentation/go-synthetic/
# In a second terminal: 
curl -sH "Accept: application/openmetrics-text;escaping=allow-utf-8" http://localhost:8080/metrics | head
# HELP example_histogram_incoming_request_duration Duration ranges of incoming requests.
# TYPE example_histogram_incoming_request_duration histogram
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="0.0"} 6
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="100.0"} 14
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="200.0"} 28 # {trace_id="ab85daed2f6501f21c040c4684325aae",span_id="6c542499c49c0c94",project_id="example-project"} 169.82267286094256 1.757332042969475e+09
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="300.0"} 47 # {project_id="example-project",trace_id="0e2ce5f35094959ca373a5a14e39fcb7",span_id="ac79404377cc8c90"} 200.2281014489768 1.757332054198945e+09
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="400.0"} 72
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="500.0"} 92 # {trace_id="d132e5d31008f581f94bfa6de7829e0b",span_id="69737420ea7c68bd",project_id="example-project"} 409.0665258170509 1.757332053685366e+09
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="600.0"} 116 # {trace_id="2f2d1a8503001e34af1e5d5da8fa7332",span_id="78d1964df8ab21b7",project_id="example-project"} 528.5128440901829 1.757332055844474e+09
example_histogram_incoming_request_duration_bucket{method="POST",path="/",string="200",le="700.0"} 145 # {trace_id="52449b3c13bd15b5ed7772c2cfe0ae06",span_id="62f346aa9d0cdae3",project_id="example-project"} 608.0780714825812 1.757332050591418e+09
```

* Flags sets to `gcm-extended` changes some metric names and `status` label to a charset that is accepted on Google Cloud Monitoring after a recent extension:

```
go run ./examples/instrumentation/go-synthetic/ --metric-naming-style=gcm-extended --status-label-naming-style=gcm-extended
# In a second terminal:  
curl -sH "Accept: application/openmetrics-text;escaping=allow-utf-8" http://localhost:8080/metrics | head                  
# HELP "example/histogram.incoming-request-duration" Duration ranges of incoming requests.
# TYPE "example/histogram.incoming-request-duration" histogram
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="0.0"} 9
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="100.0"} 19
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="200.0"} 34 # {trace_id="920ca78cd74556cfb8c6c7993604d817",span_id="38f56ddd9c3a1e61",project_id="example-project"} 198.88090238570788 1.757535833132103e+09
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="300.0"} 55
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="400.0"} 73 # {trace_id="37ec8c2962edfe839464181c3cbfb605",span_id="2e0b86045589d275",project_id="example-project"} 320.4433061084259 1.7575358368461812e+09
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="500.0"} 95 # {trace_id="d7896d0cbd99dd3325d098e04b0578cd",span_id="ef25c722d7bfcca1",project_id="example-project"} 455.2962879992864 1.757535841561217e+09
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="600.0"} 104
{"example/histogram.incoming-request-duration_bucket","example/http.request.status"="200",method="POST",path="/",le="700.0"} 122
```

* Flags sets to `exotic-utf-8` changes some metric names and `status` label to a charset that is NOT accepted on Google Cloud Monitoring and generally a bad idea (readability), but in theory, accepted by Prometheus:

```
go run ./examples/instrumentation/go-synthetic/ --metric-naming-style=exotic-utf-8 --status-label-naming-style=exotic-utf-8
# In a second terminal: 
curl -sH "Accept: application/openmetrics-text;escaping=allow-utf-8" http://localhost:8080/metrics | head 
# HELP "example/🗻😂/histogram.incoming-request-duration" Duration ranges of incoming requests.
# TYPE "example/🗻😂/histogram.incoming-request-duration" histogram
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="0.0"} 35 # {trace_id="f89000a12bb59c26e29981937d45e0ed",span_id="8fca7896ebb9b960",project_id="example-project"} -37.4811575860198 1.7573317119410172e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="100.0"} 67 # {span_id="ed43a20f53fbf5f0",project_id="example-project",trace_id="af18e0193947f899d2673e5b678f58e1"} 45.265566446857974 1.7573317040902898e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="200.0"} 127 # {trace_id="2f5b1b7a697d0acc6b16d90be98fe5ce",span_id="30b477ddd87c7aa1",project_id="example-project"} 190.87337467908512 1.7573317337490401e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="300.0"} 208 # {trace_id="85400d275155d1e3c740f7d4531ab003",span_id="f00dc80e6d3943e9",project_id="example-project"} 278.681925106673 1.7573317339548218e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="400.0"} 319 # {trace_id="088412fa97dd0dd761774e4e4d5ee444",span_id="1d6a01da5c49f458",project_id="example-project"} 384.7555867524189 1.7573316990380569e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="500.0"} 439 # {span_id="2a8531e8812b4b38",project_id="example-project",trace_id="21372592453c087b3c6e7d073a19da84"} 454.4641010490816 1.757331741793956e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="600.0"} 545 # {trace_id="60739c7384eb92991f902e2d14d75d70",span_id="59ec50c5e9b78bfc",project_id="example-project"} 591.8032678033114 1.757331736644248e+09
{"example/🗻😂/histogram.incoming-request-duration_bucket","example/🗻😂/http.request-status"="200",method="POST",path="/",le="700.0"} 644 # {trace_id="ee607f7b8dcb202c934f28b3901b4616",span_id="a858fed3ada1e0ca",project_id="example-project"} 653.5098474788573 1.757331690028308e+09
```

## Running on Kubernetes

If running managed-collection on a Kubernetes cluster, the `go-synthetic` can be
deployed and monitored by:

```bash
kubectl apply -f ./go-synthetic.yaml
```
