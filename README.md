# Prometheus Engine

[![Go Report Card](https://goreportcard.com/badge/github.com/GoogleCloudPlatform/prometheus-engine)](https://goreportcard.com/report/github.com/GoogleCloudPlatform/prometheus-engine)
[![GoDoc](https://pkg.go.dev/badge/github.com/GoogleCloudPlatform/prometheus-engine?status.svg)](https://pkg.go.dev/github.com/GoogleCloudPlatform/prometheus-engine?tab=doc)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/GoogleCloudPlatform/prometheus-engine)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)
![Build status](https://github.com/GoogleCloudPlatform/prometheus-engine/actions/workflows/presubmit.yml/badge.svg)


This repository contains various binaries and packages for client-side usage
of Google Cloud Managed Service for Prometheus (GMP), a managed Prometheus offering on top
of Google Cloud Monitoring (GCM).

For more documentation and to get started, go to [g.co/cloud/managedprometheus](https://g.co/cloud/managedprometheus).

## Binaries

* **[config-reloader](cmd/config-reloader)**: An auxiliary binary to initiate reload on configuration file changes.
* **[frontend](cmd/frontend)**: An authorizing proxy for the Prometheus-compatible query API of GMP. It additionally hosts a query UI.
* **[operator](cmd/operator)**: A Kubernetes operator for managed metric collection for GMP.
* **[rule-evaluator](cmd/rule-evaluator)**: A Prometheus rule evaluation engine that evaluates against GMP.

For the fully Prometheus-compatible binary that writes ingested data into GMP/GCM,
see [GoogleCloudPlatform/prometheus](https://github.com/GoogleCloudPlatform/prometheus).

## Build
Run `make help` shows a list of candidate targets with documentation.

Any go application in `./cmd/` with an associated `main.go`, e.g. `./cmd/operator/main.go`
is a candidate for build by running:
```
make operator
```

- Running `make bin` will generate all the go binaries.
  - Setting `NO_DOCKER=1` here will build all the binaries natively on the host machine.
- Running `make test` will run unit and e2e tests.
  - If `NO_DOCKER=1` is set, end-to-end tests will be run against the current
  kubectl context. It is assumed the cluster has access to the GCM API.
  Ensure `GMP_CLUSTER` and `GMP_LOCATION` are set, e.g.
  ```
  NO_DOCKER=1 GMP_CLUSTER=<my-cluster> GMP_LOCATION=<cluster-location> make test
  ```
- Running `make presubmit` will run various checks on the repo to ensure it is
ready to submit a pull request. This includes testing, formatting,
and regenerating files in-place.
  - Setting `DRY_RUN=1` won't regenerate any files but will return a
  non-zero exit code if the current changes differ from what would be. This
  can be useful in running in CI workflows.

### Dependencies
In order to best develop and contribute to this repository, the following dependencies are
recommended:
1. [`go`](https://golang.org/doc/install)
2. [`gcloud`](https://cloud.google.com/sdk/docs/install)
3. [`kubectl`](https://kubernetes.io/docs/tasks/tools/)
  - Can also be installed via
  ```
  gcloud components install kubectl
  ```
4. [`Docker`](https://docs.docker.com/get-docker/)
  - Can also be run via
  ```
  gcloud alpha cloud-shell ssh -- -nNT -L `pwd`/docker.sock:/var/run/docker.sock
  # Then in separate terminal.
  export DOCKER_HOST=unix://docker.sock
  ```
