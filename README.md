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

* **[config-reloader](cmd/config-reloader)**: An auxiliary sidecar binary to initiate reloads on configuration file changes.
* **[frontend](cmd/frontend)**: An authorizing proxy for the Prometheus-compatible query API of GMP. It additionally hosts a query UI.
* **[operator](cmd/operator)**: A Kubernetes operator for managed metric collection and rule evaluation for GMP.
* **[rule-evaluator](cmd/rule-evaluator)**: A Prometheus rule evaluation engine that evaluates alerting and recording rules against GMP.
* **[datasource-syncer](cmd/datasource-syncer)**: A periodic job that injects short-lived OAuth2 access tokens into Grafana Prometheus data sources.
* **[gmp-migrate](cmd/gmp-migrate)**: A standalone CLI tool to convert Prometheus Operator (`PodMonitor`, `ServiceMonitor`) resources to GMP Custom Resources.

For the fully Prometheus-compatible binary that writes ingested data into GMP/GCM,
see [GoogleCloudPlatform/prometheus](https://github.com/GoogleCloudPlatform/prometheus).

## Docker Images

Container images for this repository (except `gmp-migrate`, which is a standalone CLI binary) are published to [Artifact Registry](https://console.cloud.google.com/artifacts/docker/gke-release/us/gke-release/prometheus-engine) under `us-docker.pkg.dev/gke-release/gke-release/prometheus-engine/<binary>`.

## Deploying Self-Managed Manifests & Custom Resources

On GKE, Managed Service for Prometheus can be enabled directly via GKE cluster flags. For self-managed deployments or non-GKE Kubernetes clusters, apply the pre-rendered manifests in [`manifests/`](manifests/) in the following order:

```bash
# 1. Create namespaces (gmp-system, gmp-public), CRDs, and cluster RBAC:
kubectl apply -f manifests/setup.yaml

# 2. Deploy gmp-operator, admission webhooks, and default OperatorConfig:
kubectl apply -f manifests/operator.yaml

# 3. (Optional) Deploy standalone rule-evaluator:
kubectl apply -f manifests/rule-evaluator.yaml
```

### Custom Resource Definitions (CRDs)

All CRDs live under API group `monitoring.googleapis.com/v1`. For the full field-level schema, see the generated [API Reference (`doc/api.md`)](doc/api.md) and the sample manifests in [`examples/`](examples/).

| Kind                    | Scope      | Allowed Namespace             | Purpose                                                                                                                                  |
|:------------------------|:-----------|:------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------|
| `PodMonitoring`         | Namespaced | Any user namespace            | Scrapes Pod endpoints in the same namespace; injects `project_id`, `location`, `cluster`, `namespace`, `job`, and `instance`.            |
| `ClusterPodMonitoring`  | Cluster    | Cluster-scoped                | Scrapes Pod endpoints across all namespaces; injects `namespace` from the scraped Pod.                                                   |
| `ClusterNodeMonitoring` | Cluster    | Cluster-scoped                | Scrapes node-level endpoints (such as kubelet, cAdvisor, or node-exporter).                                                              |
| `Rules`                 | Namespaced | Any user namespace            | Evaluates Prometheus alerting and recording rules scoped to the resource's `namespace` (plus `project_id`, `location`, `cluster`).       |
| `ClusterRules`          | Cluster    | Cluster-scoped                | Evaluates alerting and recording rules across all namespaces in the cluster (scoped to `project_id`, `location`, `cluster`).             |
| `GlobalRules`           | Cluster    | Cluster-scoped                | Evaluates alerting and recording rules across the entire GCM metrics scope without injecting cluster or namespace selectors.             |
| `OperatorConfig`        | Namespaced | `gmp-public` (named `config`) | Configures cluster-wide collector credentials, kubelet scraping, rule-evaluator credentials, Alertmanager endpoints, and export filters. |

## Development

See [`AGENTS.md`](AGENTS.md) for detailed contribution guidelines, commit message formatting (`make conform`), linter rules (`make lint`), and code-generation workflows (`make regen`).

### Repository Layout & Go Modules

| Path                                                       | Go Module                                                                 | Description                                                                                                                            |
|:-----------------------------------------------------------|:--------------------------------------------------------------------------|:---------------------------------------------------------------------------------------------------------------------------------------|
| [`cmd/`](cmd/)                                             | [`go.mod`](go.mod)                                                        | Main entrypoints and Dockerfiles for the 6 binaries.                                                                                   |
| [`pkg/`](pkg/), [`internal/`](internal/)                   | [`go.mod`](go.mod)                                                        | Reconcilers, CRD API types (`pkg/operator/apis/monitoring/v1`), GCM export client, migration library, and UI asset loader (`pkg/ui`).  |
| [`charts/`](charts/) → [`manifests/`](manifests/)          | —                                                                         | Internal Helm templates (`charts/`) that render static deployment manifests (`manifests/`) via `make regen`.                           |
| [`doc/api.md`](doc/api.md)                                 | —                                                                         | Autogenerated CRD API reference documentation (do not edit by hand; updated via `make regen`).                                         |
| [`e2e/`](e2e/)                                             | [`go.mod`](go.mod)                                                        | End-to-end Kubernetes test suite executed against `kind` or GKE clusters.                                                              |
| [`examples/`](examples/)                                   | [`go.mod`](go.mod) & [`examples/scripts/go.mod`](examples/scripts/go.mod) | Reference CRD manifests, feature guides, synthetic Go workload (`examples/instrumentation/go-synthetic`), and exporter configurations. |
| [`ops/`](ops/)                                             | [`ops/gmpctl/go.mod`](ops/gmpctl/go.mod)                                  | Multi-branch release and vulnerability automation CLI (`ops/gmpctl`).                                                                  |
| [`third_party/prometheus_ui/`](third_party/prometheus_ui/) | —                                                                         | Vendored upstream Prometheus React UI (`base/`) and GMP overrides (`override/`).                                                       |
| [`tools/`](tools/)                                         | [`tools/go.mod`](tools/go.mod)                                            | Pinned build, codegen, and linting tool dependencies (`controller-gen`, `helm`, `yq`, `mdox`, `shfmt`, `addlicense`).                  |

### Dependencies

In order to best develop and contribute to this repository, the following dependencies are
recommended:
1. [`go`](https://golang.org/doc/install) (matching the version in [`go.mod`](go.mod))
2. [`gcloud`](https://cloud.google.com/sdk/docs/install)
3. [`kubectl`](https://kubernetes.io/docs/tasks/tools/)

Can also be installed via:

```bash
gcloud components install kubectl
```

4. [`Docker`](https://docs.docker.com/get-docker/) with
   [`buildx`](https://docs.docker.com/build/architecture/#install-buildx) plugin.

If you want to execute docker containers on a remote machine you can run:

```bash
gcloud alpha cloud-shell ssh --authorize-session -- -nNT -L `pwd`/docker.sock:/var/run/docker.sock

# Then in separate terminal.
export DOCKER_HOST=unix://docker.sock
```

5. For UI development or updates (e.g. to resolve UI security issues), `npm` is
   required. See [pkg/ui documentation](pkg/ui/README.md) for details.

### Presubmit, Code Generation & Linting

* `make regen` (or `./hack/presubmit.sh`): Regenerates CRD YAMLs (`charts/operator/crds/`), Helm manifests (`manifests/`), API docs (`doc/api.md`), and runs formatting (`./hack/presubmit.sh format`).
* `CHECK=1 make regen`: Verifies that no uncommitted diffs or missing license headers remain after regeneration (enforced in CI).
* `./hack/presubmit.sh codegen`: Regenerates Kubernetes `DeepCopy` methods (`zz_generated.deepcopy.go`) and typed client/informer/lister code (`pkg/operator/generated/`) when Go API types under `pkg/operator/apis/monitoring/v1/` change.
* `make lint`: Runs `golangci-lint` across the repository.
* `make conform`: Validates git commit messages against [`.conform.yaml`](.conform.yaml).
* `make presubmit` (or `CHECK=1 make presubmit`): Regenerates all resources, builds all images, and runs unit tests in Docker.

### Building

Running `make help` shows a list of candidate targets with documentation.

Any Go application in `./cmd/` with an associated `main.go` (e.g. `./cmd/operator/main.go`)
can be built via `make`:

```bash
make operator
make frontend
make rule-evaluator
make config-reloader
make datasource-syncer
NO_DOCKER=1 make gmp-migrate
```

This also includes example Go apps from `/examples/instrumentation/`:

```bash
make go-synthetic
```

Running `make bin` will build the component Go binaries into `./build/bin/`.
* Setting `NO_DOCKER=1` (`NO_DOCKER=1 make bin`) builds the binaries natively on the host machine without Docker.

### Testing

#### Unit

To run unit tests natively on the host machine, use `go test ./...`, your IDE, or:

```bash
NO_DOCKER=1 make test
```

To run unit tests inside the hermetic Docker container, run `make test`.

#### Kubernetes End-to-end tests

Running `make e2e` builds the container images and runs end-to-end tests against a Kubernetes cluster:
* By default, it runs in a hermetic Docker container, downloads `kind`, creates
  a single-node `kind` cluster, and runs [e2e](./e2e) tests against it.
* Each Go test from [e2e](./e2e) runs in a separate `kind` cluster. The name of
  the test determines the cluster name, so `TestXYZ` is executed in a `XYZ-<short unique sha>` cluster.
* To run a single test (or re-run without rebuilding Docker images via `make e2e-only`), use the `TEST_RUN` environment variable:

```bash
TEST_RUN=TestCollectorPodMonitoring make e2e
# Or re-run against already-built images:
TEST_RUN=TestCollectorPodMonitoring make e2e-only
```

* For easier debugging you can keep the `kind` cluster running after test failures using the `KIND_PERSIST` variable:

```bash
KIND_PERSIST=1 TEST_RUN=TestCollectorPodMonitoring make e2e
```

##### GCM integration

Some tests verify GCM state. For this you need a GCP service account with read and write permissions against
GCM in a project. Here is how to configure it:

* If you have a GCP service account key in a local file, use the `GOOGLE_APPLICATION_CREDENTIALS` environment variable to specify the path to that file:

```bash
GOOGLE_APPLICATION_CREDENTIALS=<path to SA> TEST_RUN=TestCollectorPodMonitoring make e2e
```

* If you have the content of a GCP service account key in an environment variable (e.g. CI), use the `GCM_SECRET` variable:

```bash
GCM_SECRET=<...> TEST_RUN=TestCollectorPodMonitoring make e2e
```

> NOTE: This is what CI uses.
> NOTE2: This utilizes an explicit credentials path via `OperatorConfig`, while `GOOGLE_APPLICATION_CREDENTIALS` uses default credentials mode.

* If you want to skip GCM tests, ensure those two variables are empty:

```bash
GCM_SECRET="" GOOGLE_APPLICATION_CREDENTIALS="" TEST_RUN=TestCollectorPodMonitoring make e2e
```

##### Debugging

In Docker mode, after failures with `KIND_PERSIST` and during tests, you can
inspect the local `kind` cluster for further investigation. The easiest way is to use
the same Docker image and setup that is used in `make e2e`, by starting a new
container using the `gmp/kindtest` image on the same Docker socket and network.

You can use `make e2e-exec` to start an interactive shell:

```bash
make e2e-exec
```

In the interactive shell, you can inspect existing tests/clusters or spin up a new test.

For example, assuming you are running (or ran with `KIND_PERSIST=1`) `make e2e`, you can
access the Kubernetes cluster (e.g. to list pods):

```bash
kind get clusters
kind export kubeconfig -n <TestName-hash from the output above>
kubectl -n gmp-system get po
```

You can also execute a single test (e.g. `TestCollectorPodMonitoring`) by reusing an already started `kind` cluster or creating your own:

```bash
kind export kubeconfig -n "existing-cluster"
go test -v ./e2e -run "TestCollectorPodMonitoring" -args -project-id=test-proj -cluster="existing-cluster" -location=test-loc -skip-gcm
kubectl -n gmp-system get po
```

However, it might be easier to run `KIND_PERSIST=1 TEST_RUN=TestCollector make e2e` in a separate terminal and
play with custom `time.Sleep` statements for more detailed investigations.

##### Benchmarking

See [BENCHMARK.md](./BENCHMARK.md).
