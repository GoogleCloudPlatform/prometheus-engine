# Operator Package (`pkg/operator`)

This package implements the reconciliation loops, admission webhooks, and Custom Resource Definition (CRD) types (`monitoring.googleapis.com/v1`) used by [`cmd/operator`](../../cmd/operator/README.md).

## Package Architecture

`Operator.Run()` in [`operator.go`](operator.go) registers up to five controllers with `controller-runtime`:

| Controller Name    | Source Files                                                                                             | Watched Resources / Trigger                                                         | Managed Outputs                                                                                                                                                                                                  |
|:-------------------|:---------------------------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------|:-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `collector-config` | [`collection.go`](collection.go), [`apis/monitoring/v1/pod_config.go`](apis/monitoring/v1/pod_config.go) | `OperatorConfig`, `PodMonitoring`, `ClusterPodMonitoring`, `ClusterNodeMonitoring`  | Mirrors `collection.credentials` to `Secret/gmp-system/collection`, generates Prometheus config in `ConfigMap/gmp-system/collector` (optionally gzipped), and updates monitoring CRD status conditions.          |
| `rules`            | [`rules.go`](rules.go), [`apis/monitoring/v1/rules_config.go`](apis/monitoring/v1/rules_config.go)       | `OperatorConfig`, `Rules`, `ClusterRules`, `GlobalRules`                            | Injects scope labels into `Rules`/`ClusterRules` PromQL expressions, writes rule files to `ConfigMap/gmp-system/rules-generated`, updates status conditions, and scales `rule-evaluator` and `alertmanager` (0/1). |
| `operator-config`  | [`operator_config.go`](operator_config.go)                                                               | `OperatorConfig` (`gmp-public/config`), `gmp-public` Secrets                        | Defaults `OperatorConfig`, generates `ConfigMap/gmp-system/rule-evaluator`, and mirrors `rules` and `alertmanager` Secrets from `gmp-public` into `gmp-system`.                                                  |
| `scaling`          | [`scaling.go`](scaling.go) *(only registered when VPA CRD is installed)*                                 | `OperatorConfig` (`gmp-public/config`), owned `VerticalPodAutoscaler`s              | Creates or deletes `VerticalPodAutoscaler` resources for `alertmanager`, `collector`, `gmp-operator`, and `rule-evaluator` in `gmp-system` based on `scaling.vpa.enabled`.                                       |
| `target-status`    | [`target_status.go`](target_status.go), [`endpoint_status_builder.go`](endpoint_status_builder.go)       | Periodic 10s channel timer (when `features.targetStatus.enabled` is `true`)         | Polls `/api/v1/targets` on `collector` pods and populates `status.endpointStatuses` on `PodMonitoring` and `ClusterPodMonitoring` CRDs.                                                                          |

## Code Generation

> [!IMPORTANT]
> Never manually edit generated files under `charts/operator/crds/`, `manifests/`, `doc/api.md`, or `pkg/operator/apis/monitoring/v1/zz_generated.deepcopy.go`. CI enforces a clean working tree via `CHECK=1 make regen`.

When modifying Go API types under [`pkg/operator/apis/monitoring/v1/`](apis/monitoring/v1/), run the following two steps from the repository root:

1. **Regenerate Go `DeepCopy` methods (when structs or fields change)**:

```bash
CODEGEN_PKG="$(go list -modfile=tools/go.mod -m -f '{{.Dir}}' k8s.io/code-generator)" ./hack/presubmit.sh codegen
```

This runs `kube::codegen::gen_helpers` to update `pkg/operator/apis/monitoring/v1/zz_generated.deepcopy.go`.

2. **Regenerate CRD YAMLs, Helm manifests, API documentation, and formatting**:

```bash
make regen
```

This executes [`./hack/presubmit.sh`](../../hack/presubmit.sh) (`format`, `crdgen`, `manifests`, and `docgen`) and `addlicense`, updating `charts/operator/crds/*.yaml`, `manifests/setup.yaml`, `manifests/operator.yaml`, `manifests/rule-evaluator.yaml`, `cmd/datasource-syncer/datasource-syncer.yaml`, and `doc/api.md`.

## Testing

### Unit Tests

Run unit tests for the operator package natively from the repository root:

```bash
go test ./pkg/operator/...
```

### End-to-End Tests (`e2e/`)

To run the full end-to-end suite in isolated `kind` clusters (from the repository root):

```bash
make e2e
# Or run a single test without rebuilding images:
TEST_RUN=TestCollectorPodMonitoring make e2e-only
```

To run a single `setupCluster`-based `e2e/` test directly against a clean Kubernetes cluster (with GKE Managed Service for Prometheus disabled and no `gmp-*` CRDs or namespaces pre-installed):

```bash
go test ./e2e/ -run '^TestCollectorPodMonitoring$' \
  -args -project-id="$PROJECT_ID" -cluster="$CLUSTER_NAME" -location="$LOCATION" -skip-gcm
```

Omit `-skip-gcm` (or pass `-gcp-service-account=<filepath>`) when verifying metric ingestion and querying against Google Cloud Monitoring.
