# Rule Evaluator

The `rule-evaluator` binary evaluates Prometheus alerting and recording rules against a Prometheus-compatible query API endpoint (typically Google Cloud Managed Service for Prometheus) and writes recording rule results and alert state series (`ALERTS`, `ALERTS_FOR_STATE`) back to Google Cloud Monitoring via the `CreateTimeSeries` API.

## Kubernetes Deployment Modes

In Kubernetes, `rule-evaluator` can be deployed in two ways:
1. **Operator-managed (`gmp-operator`)**: Configured automatically in the `gmp-system` namespace by `gmp-operator` based on `OperatorConfig/config` in `gmp-public` and `Rules`, `ClusterRules`, and `GlobalRules` Custom Resources (which are compiled into `ConfigMap/gmp-system/rules-generated`). Additional CLI flags can be passed via the `EXTRA_ARGS` environment variable on the container.
2. **Standalone deployment**: Deployed directly from [`manifests/rule-evaluator.yaml`](../../manifests/rule-evaluator.yaml) when running rule evaluation independently of `gmp-operator`.

On startup, `rule-evaluator` queries historical `ALERTS_FOR_STATE` series from the target query endpoint so pending alerts with `for` durations retain their state across pod restarts.

## Local Setup & Execution

For local execution against Google Cloud Monitoring, ensure the `gcloud` CLI is [installed and configured](https://cloud.google.com/sdk/docs/quickstart).

The `rule-evaluator` reads standard Prometheus configuration files (`global`, `alerting`, and `rule_files` sections) and rule files. Consult the upstream Prometheus documentation for the [configuration format](https://prometheus.io/docs/prometheus/latest/configuration/configuration/), [alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/), and [recording rules](https://prometheus.io/docs/prometheus/latest/configuration/recording_rules/).

### 1. Create a Minimal `config.yaml` and `rules.yaml`

```yaml
# config.yaml
global:
  evaluation_interval: 30s
rule_files:
  - rules.yaml
```

```yaml
# rules.yaml
groups:
  - name: example
    rules:
      - record: job:up:sum
        expr: sum by (job) (up)
```

### 2. Run Against Google Cloud Monitoring

From the **repository root**:

```bash
PROJECT_ID=$(gcloud config get-value core/project)
ZONE=us-central1-b
# Default gcloud credentials. Substitute a service account key in production.
CREDENTIALS="$HOME/.config/gcloud/application_default_credentials.json"

go run ./cmd/rule-evaluator \
  --export.label.project-id="$PROJECT_ID" \
  --export.label.location="$ZONE" \
  --export.label.cluster=local-dev \
  --export.credentials-file="$CREDENTIALS" \
  --query.credentials-file="$CREDENTIALS" \
  --query.project-id="$PROJECT_ID" \
  --config.file=config.yaml
```

Recording rule results become visible through the Prometheus query API (see [frontend](../frontend/README.md) for setting up a local query UI), and firing alerts are forwarded to configured Alertmanager endpoints.

## Flags

```bash mdox-exec="bash hack/format_help.sh rule-evaluator"
usage: rule [<flags>]

The Prometheus Rule Evaluator


Flags:
  -h, --[no-]help                Show context-sensitive help (also try
                                 --help-long and --help-man).
      --log.level=info           The level of logging. Can be one of 'debug',
                                 'info', 'warn', 'error'
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
      --export.debug.log-match= ...  
                                 A Prometheus time series matcher for debugging
                                 gRPC requests. Logs exact gRPC requests
                                 containing any time series matching these
                                 matchers. Can be repeated.
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
      --export.ha.lease-duration=EXPORT.HA.LEASE-DURATION  
                                 Duration that non-leader replicas will wait
                                 before trying to acquire the lease. Optional.
                                 Defaults to 15s. ($LEASE_DURATION)
      --export.ha.renew-deadline=EXPORT.HA.RENEW-DEADLINE  
                                 Duration that the leader will retry refreshing
                                 the lease before giving up. Optional. Defaults
                                 to 10s. ($RENEW_DEADLINE)
      --export.ha.retry-period=EXPORT.HA.RETRY-PERIOD  
                                 Duration between lease renewal retries.
                                 Optional. Defaults to 2s. ($RETRY_PERIOD)
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

## Local Dry-Run Development (Without GCM Credentials)

For local development and testing, `rule-evaluator` can evaluate rule queries against any local Prometheus-compatible HTTP server (such as a port-forwarded collector on `http://localhost:19090`) with authentication and GCM export disabled:

```bash
go run ./cmd/rule-evaluator \
  --export.label.project-id=test-project \
  --export.label.location=us-central1-b \
  --export.label.cluster=test-cluster \
  --export.disable \
  --query.target-url=http://localhost:19090 \
  --query.debug.disable-auth \
  --config.file=config.yaml
```
