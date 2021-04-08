# Prometheus Rule Evaluator

This binary will digest Prometheus rule configurations and evaluate them against a global query endpoint. Results are written back to GCM via the CreateTimeSeries API and alert notifications are sent to a configurable Alertmanager endpoint.

## Spinup

Make sure that your `gcloud` CLI is [setup properly](https://cloud.google.com/sdk/docs/quickstart).

Authenticate with Google Cloud:
```
gcloud auth login
gcloud auth application-default login
```
## Install

1. [Local Prometheus Instance](https://prometheus.io/docs/prometheus/latest/getting_started/) or GPE Instance //TODO(maxamin): add GPE link
2. [Alert Manager](https://prometheus.io/download/)

## Startup Guide

Startup Prometheus and Alert Manager.

Define the project id, location, and config file:

```bash
PROJECT_ID=$(gcloud config get-value core/project)
ZONE=us-central1-b
CONFIG_FILE=prometheus.yml
CREDENTIALS=~/.config/gcloud/application_default_credentials.json #default location
```

Define one of the following query target:

```bash
# Address to local prometheus instance
TARGET=http://localhost:9090/ # default Prometheus query address


# Address to GPE instance
TARGET=https://staging-monitoring.sandbox.googleapis.com/v1alpha/projects/$PROJECT_ID/location/global/prometheus/
```

Run the binary:

```bash
go run main.go \
    --gcm.project_id=$PROJECT_ID \
    --gcm.label.location=$ZONE \
    --query.target-url=$TARGET \
    --config.file=$CONFIG_FILE \
    --gcm.credentials-file=$CREDENTIALS
```

## Evaluation

1. Recordings data is available in the Cloud Monitoring metric explorer for your project (https://pantheon.corp.google.com/monitoring/metrics-explorer).
2. Alerts are viewiable at configured alert manager in prometheus.yml (default: http://localhost:9093).
3. Metrics are viewable at the listening address (default: http://localhost:9091/metrics).