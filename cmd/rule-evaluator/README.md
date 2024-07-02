# Rule Evaluator

The rule evaluator closely replicates the rule evaluation behavior of the Prometheus server.

Instead of reading and writing from local storage, it evaluates alerting and recording rules
against a Prometheus compatible query API endpoint (typically Google Cloud Prometheus Engine's API)
and writes data back to Google Cloud Monitoring via the CreateTimeSeries API.

## Setup

For local setup, make sure that the `gcloud` CLI is [setup](https://cloud.google.com/sdk/docs/quickstart).

We use example configuration files and rule files, which work like in a regular Prometheus server.
For the config file, the rule evaluator considers the `alerting` and `rule_files` section as well as applicable fields of the `global` section.

Consult the Prometheus documentation for details on the [configuration format](https://prometheus.io/docs/prometheus/latest/configuration/configuration) as well as the [alerting](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/) and [recording](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/) rule file format.

### Run

```bash
PROJECT_ID=$(gcloud config get-value core/project)
ZONE=us-central1-b
CONFIG_FILE=example/config.yaml
# Default gcloud credentials. Substitute for service account key in production.
CREDENTIALS=~/.config/gcloud/application_default_credentials.json
GCM_TARGET=staging-monitoring.sandbox.googleapis.com:443
```

```bash
go run main.go \
  --export.label.project-id=$PROJECT_ID \
  --export.label.location=$ZONE \
  --export.endpoint=$GCM_TARGET \
  --export.credentials-file=$CREDENTIALS \
  --query.credentials-file=$CREDENTIALS \
  --query.project-id=$PROJECT_ID \
  --config.file=$CONFIG_FILE
```

After a while recording rule results become visible through Prometheus Engine's query
API (see [frontend]("../frontend/README.md") for setting up a UI) and firing alerts appear
in the AlertManager and are routed from there.

## Flags

```bash mdox-exec="bash hack/format_help.sh rule-evaluator"
usage: rule [<flags>]

The Prometheus Rule Evaluator


Flags:
  -h, --[no-]help                Show context-sensitive help (also try
                                 --help-long and --help-man).
      --[no-]export.disable      Disable exporting to GCM.
      --export.endpoint="monitoring.googleapis.com:443"  
                                 GCM API endpoint to send metric data to.
      --export.compression=none  The compression format to use for gRPC requests
                                 ('none' or 'gzip').
      --export.credentials-file=""  
                                 Credentials file for authentication with the
                                 GCM API.
      --export.label.project-id=""  
                                 Default project ID set for all exported data.
                                 Prefer setting the external label "project_id"
                                 in the Prometheus configuration if not using
                                 the auto-discovered default.
      --export.user-agent-mode=unspecified  
                                 Mode for user agent used for requests against
                                 the GCM API. Valid values are "gke", "kubectl",
                                 "on-prem", "baremetal" or "unspecified".
      --export.label.location=""  
                                 The default location set for all exported data.
                                 Prefer setting the external label "location" in
                                 the Prometheus configuration if not using the
                                 auto-discovered default.
      --export.label.cluster=""  The default cluster set for all scraped
                                 targets. Prefer setting the external label
                                 "cluster" in the Prometheus configuration if
                                 not using the auto-discovered default.
      --export.match= ...        A Prometheus time series matcher. Can be
                                 repeated. Every time series must match at
                                 least one of the matchers to be exported.
                                 This flag can be used equivalently to the
                                 match[] parameter of the Prometheus federation
                                 endpoint to selectively export data.
                                 (Example: --export.match='{job="prometheus"}'
                                 --export.match='{__name__=~"job:.*"})
      --export.debug.metric-prefix="prometheus.googleapis.com"  
                                 Google Cloud Monitoring metric prefix to use.
      --[no-]export.debug.disable-auth  
                                 Disable authentication (for debugging
                                 purposes).
      --export.debug.batch-size=200  
                                 Maximum number of points to send in one batch
                                 to the GCM API.
      --export.debug.shard-count=1024  
                                 Number of shards that track series to send.
      --export.debug.shard-buffer-size=2048  
                                 The buffer size for each individual shard.
                                 Each element in buffer (queue) consists of
                                 sample and hash.
      --export.token-url=""      The request URL to generate token that's needed
                                 to ingest metrics to the project
      --export.token-body=""     The request Body to generate token that's
                                 needed to ingest metrics to the project.
      --export.quota-project=""  The projectID of an alternative project for
                                 quota attribution.
      --export.debug.fetch-metadata-timeout=10s  
                                 The total timeout for the initial gathering
                                 of the best-effort GCP data from the metadata
                                 server. This data is used for special
                                 labels required by Prometheus metrics (e.g.
                                 project id, location, cluster name), as well as
                                 information for the user agent. This is done on
                                 startup, so make sure this work to be faster
                                 than your readiness and liveliness probes.
      --export.ha.backend=none   Which backend to use to coordinate HA pairs
                                 that both send metric data to the GCM API.
                                 Valid values are "none" or "kube"
      --export.ha.kube.config=""  
                                 Path to kube config file.
      --export.ha.kube.namespace=""  
                                 Namespace for the HA locking resource. Must be
                                 identical across replicas. May be set through
                                 the KUBE_NAMESPACE environment variable.
                                 ($KUBE_NAMESPACE)
      --export.ha.kube.name=""   Name for the HA locking resource. Must
                                 be identical across replicas. May be set
                                 through the KUBE_NAME environment variable.
                                 ($KUBE_NAME)
      --query.project-id=""      Project ID of the Google Cloud Monitoring
                                 scoping project to evaluate rules against.
      --query.target-url=https://monitoring.googleapis.com/v1/projects/PROJECT_ID/location/global/prometheus  
                                 The address of the Prometheus server query
                                 endpoint. (PROJECT_ID is replaced with the
                                 --query.project-id flag.)
      --query.generator-url=https://console.cloud.google.com/monitoring/metrics-explorer  
                                 The base URL used for the generator URL in the
                                 alert notification payload. Should point to an
                                 instance of a query frontend that accesses the
                                 same data as --query.target-url.
      --query.credentials-file=<FILE>  
                                 Credentials file for OAuth2 authentication with
                                 --query.target-url.
      --[no-]query.debug.disable-auth  
                                 Disable authentication (for debugging
                                 purposes).
      --web.listen-address=":9091"  
                                 The address to listen on for HTTP requests.
      --config.file="prometheus.yml"  
                                 Prometheus configuration file path.
      --alertmanager.notification-queue-capacity=10000  
                                 The capacity of the queue for pending
                                 Alertmanager notifications.

```

## Development

For development, the rule evaluator can evaluate rule queries against arbitrary other
endpoints that expose the Prometheus query API. For example, against a locally running
Prometheus server:

```bash
TARGET=http://localhost:19090
```

```bash
go run main.go \
    --export.label.project-id=$PROJECT_ID \
    --export.label.location=$ZONE \
    --export.endpoint=$GCM_TARGET \
    --export.credentials-file=$CREDENTIALS \
    --query.project-id=$PROJECT_ID \
    --query.target-url=$TARGET \
    --config.file=$CONFIG_FILE
```
