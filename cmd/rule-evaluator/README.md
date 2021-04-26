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

## Development

For development, the rule evaluator can evaluate rule queries against arbitrary other
endpoints that expose the Prometheus query API. For example, against a locally running
Prometheus server:

```bash
TARGET=http://localhost:9090
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