# Prometheus Engine

This repository contains various binaries and packages for client-side usage
of Google Cloud Prometheus Engine (GPE), a managed Prometheus offering on top
of Google Cloud Monitoring (GCM).

## Binaries

* **[config-reloader](cmd/config-reloader)**: An auxiliary binary to initiate reload on configuration file changes.
* **[example-app](cmd/example-app)**: An application exposing synthetic metrics.
* **[frontend](cmd/frontend)**: An authorizing proxy for the Prometheus-compatible query API of GPE. It additionally hosts a query UI.
* **[operator](cmd/operator)**: A Kubernetes operator for managed metric collection for GPE.
* **[rule-evaluator](cmd/rule-evaluator)**: A Prometheus rule evaluation engine that evaluates against GPE.

For the fully Prometheus-compatible binary that writes ingested data into GPE/GCM,
see [GoogleCloudPlatform/prometheus](https://github.com/GoogleCloudPlatform/prometheus).
