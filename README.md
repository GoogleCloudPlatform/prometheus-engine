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
* **[datasource-syncer](cmd/datasource-syncer)**: A cron job for periodic Oauth2 token injection to Grafana Prometheus datasource.

For the fully Prometheus-compatible binary that writes ingested data into GMP/GCM,
see [GoogleCloudPlatform/prometheus](https://github.com/GoogleCloudPlatform/prometheus).

## Docker Images

Images for this repo are regularly released [in the GKE release GCR](https://console.cloud.google.com/gcr/images/gke-release/global[main.go](cmd%2Fdatasource-syncer%2Fmain.go)/prometheus-engine).

## Development

### Dependencies

In order to best develop and contribute to this repository, the following dependencies are
recommended:
1. [`go`](https://golang.org/doc/install)
2. [`gcloud`](https://cloud.google.com/sdk/docs/install)
3. [`kubectl`](https://kubernetes.io/docs/tasks/tools/)

Can be also installed via:

```bash
gcloud components install kubectl
```
4. [`Docker`](https://docs.docker.com/get-docker/) with
   [`buildx`](https://docs.docker.com/build/architecture/#install-buildx) plugin.

If you want to execute docker containers on remote machine you can run:

```bash
gcloud alpha cloud-shell ssh --authorize-session -- -nNT -L `pwd`/docker.sock:/var/run/docker.sock

# Then in separate terminal.
export DOCKER_HOST=unix://docker.sock
```

5. For UI development or update (e.g. to resolve UI security issue), `npm` is
   required. See [pkg/ui documentation](pkg/ui/README.md) for details.

### Presubmit

`make presubmit` regenerates all resources, builds all images and runs all tests.

Steps from presubmit are validated on the CI, but feel free to run it if you see
CI failures related to regenerating resources or when you want to do local check
before submitting.

Run `CHECK=1 make presubmit` to fail the command if repo state is not clean after
presubmit (might require committing the changes).

### Building

Run `make help` shows a list of candidate targets with documentation.

Any go application in `./cmd/` with an associated `main.go`, e.g. `./cmd/operator/main.go`
is a candidate for build by running, for example:

```bash
make operator
make frontend
make rule-evaluator
make config-reloader
```

This also includes example Go apps from `/examples/instrumentation/`:

```bash
make go-synthetic
```

Running `make bin` will build all of the above go binaries.
* Setting `NO_DOCKER=1` here will build all the binaries natively on the host machine.

### Testing

#### Unit

To run unit tests locally, use `go test`, your IDE or `NO_DOCKER=1 make test`.

To run unit tests from docker container run `make test`

#### Kubernetes End-to-end tests

Running `make e2e` will generally run e2e tests against Kubernetes cluster:
* By default, it runs in hermetic docker container, downloads kind, recreates
  a single node kind cluster and runs [e2e](./e2e) tests against it.
* Each Go test from [e2e](./e2e) is running in separate `kind` cluster. Name of
  the test is determining the cluster name, so `TestXYZ` is executed in `XYX-<short unique sha>` cluster.
* To run a single test, use the `TEST_RUN` environment variable. For example:

```bash
TEST_RUN=TestCollectorPodMonitoring make e2e
```

* For easier debugging you can choose to keep `kind` cluster running after test failures using `KIND_PERSIST` variable:

```bash
KIND_PERSIST=1 TEST_RUN=TestCollectorPodMonitoring make e2e
```

##### GCM integration

Some tests verify GCM state. For this you need a GCP service account to with read and write permissions against
GCM in certain project. Here is how to configure it:

* If you have GCP service account in a local file, use `GOOGLE_APPLICATION_CREDENTIALS` environment variable to specify the path to that file.

```bash
GOOGLE_APPLICATION_CREDENTIALS=<path to SA> TEST_RUN=TestCollectorPodMonitoring make e2e
```

* If you have the content of GCP service account in environment variable (e.g. CI), use `GCM_SECRET` variable:

```bash
GCM_SECRET=<...> TEST_RUN=TestCollectorPodMonitoring make e2e
```

> NOTE: This is what CI is using at the moment.
> NOTE2: This is utilizing an explicit credentials path via OperatorConfig, while GOOGLE_APPLICATION_CREDENTIALS is using default credentials mode.

* If you want to skip GCM tests ensure those two variables are empty.

```bash
GCM_SECRET="" GOOGLE_APPLICATION_CREDENTIALS="" TEST_RUN=TestCollectorPodMonitoring make e2e
```

##### Debugging

In docker mode, after failures with `KIND_PERSIST` and during tests, you can use
inspect local `kind` cluster for further investigations. The easiest way is to use
the same docker image and setup that is used in `make e2e`, by starting a new
container using `gmp/kindtest` image on the same docker socket and network.

You can use `make e2e-exec` to start an interactive shell:

```bash
make e2e-exec 
```

In the interactive shell, you can inspect existing tests/cluster or spin up a new tests.

For example, assuming you are running (or ran with `KIND_PERSIST=1`) `make e2e` you can
access the Kubernetes cluster (e.g. to list pods):

```bash
kind get clusters 
kind export kubeconfig -n <TestName-hash from the output above>
kubectl -n gmp-system get po
```

You could also execute a single test e.g. `TestCollectorPodMonitoring`, but you need to
either reuse already started `kind` cluster or create your own e.g.

```bash
kind export kubeconfig -n "existing-cluster"
go test -v ./e2e -run "TestCollectorPodMonitoring" -args -project-id=test-proj -cluster="existing-cluster" -location=test-loc -skip-gcm
kubectl -n gmp-system get po
```

However, it might be easier to run `KIND_PERSIST=1 TEST_RUN=TestCollector make e2e` in separate terminal and
play with custom `time.Sleep` statements for more detailed investigations.

##### Benchmarking

See [BENCHMARK.md](./BENCHMARK.md).
