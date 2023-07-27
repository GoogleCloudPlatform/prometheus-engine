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

## Docker Images

Images for this repo are regularly released [in the GKE release GCR](https://console.cloud.google.com/gcr/images/gke-release/global/prometheus-engine).

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
make example-app
make config-reloader
```

Running `make bin` will build all of the above go binaries.
  * Setting `NO_DOCKER=1` here will build all the binaries natively on the host machine.

### Testing

#### Unit

To run unit tests locally, use `go test`, your IDE or `NO_DOCKER=1 make test`.

To run unit tests from docker container run `make test`

#### End-to-end tests

Running `make e2e` will run e2e tests against Kubernetes cluster:
  * By default, it run in hermetic docker container, downloads kind, recreates
a single node kind cluster and runs [e2e](./e2e) tests against it.
  * If `NO_DOCKER=1` is set, end-to-end tests will be run against the current
    kubectl context. It is assumed the cluster has access to the GCM API.
    Ensure `GMP_CLUSTER` and `GMP_LOCATION` are set, e.g.
  ```bash
  NO_DOCKER=1 GMP_CLUSTER=<my-cluster> GMP_LOCATION=<cluster-location> make e2e
  ```

In docker mode, to run a single test or debug a cluster during or after failed
test, you can try entering shell of the `kindtest` container. Before doing so, 
run `make e2e` to setup `kind` and start a cluster.

To enter shell with kind Kubernetes context, (ensure your docker socket is on
`/var/run/docker.sock`):

```bash
docker run --network host --rm -it \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v `pwd`/e2e:/build/e2e gmp/kindtest bash
```

To access kind Kubernetes (e.g. to list pods) run:

```bash
kind export kubeconfig
kubectl get po
```

To execute a single test e.g. `TestAlertmanagerDefault` you can do (in `kindtest` shell):

```bash
kind export kubeconfig
go test -v ./e2e -run "TestAlertmanagerDefault" -args -project-id=test-proj -cluster=test-cluster -location=test-loc -skip-gcm
```

Each test case is creating a separate set of namespaces e.g.
`gmp-test-testalertmanagerdefault-20230714-120756` and
`gmp-test-testalertmanagerdefault-20230714-120756-pub`, so to debug tests you
have to ensure those namespaces are not cleaned. You can also provide time.Sleep in
the place you want debug in.

##### Benchmarking 📢

See [BENCHMARK.md](./BENCHMARK.md).
