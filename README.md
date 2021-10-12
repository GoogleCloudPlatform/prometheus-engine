# Prometheus Engine

This repository contains various binaries and packages for client-side usage
of Google Cloud Managed Service for Prometheus (GMP), a managed Prometheus offering on top
of Google Cloud Monitoring (GCM).

See g.co/cloud/managedprometheus for more documentation.

## Binaries

* **[config-reloader](cmd/config-reloader)**: An auxiliary binary to initiate reload on configuration file changes.
* **[example-app](cmd/example-app)**: An application exposing synthetic metrics.
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
This generates in a binary in `./build/bin/`.

The make target can be configured to build a Docker image for any application in `./cmd/`
with an associated `Dockerfile`, e.g. `./cmd/operator/Dockerfile` by setting the
environment variable `DOCKER_BUILD=1`. This results in a minimal docker image
containing just that application's binary. The binary is subsequently copied to the
local directory `./build/bin/`.

Running `make all` will generate all the go binaries.

Running `make docker` will generate all the go binary Docker images.

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
