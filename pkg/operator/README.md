# Operator Package (`pkg/operator`)

This package implements the reconciliation loops, admission webhooks, and Custom Resource Definition (CRD) types (`monitoring.googleapis.com/v1`) used by [`cmd/operator`](../../cmd/operator/README.md).

## Package Architecture

`Operator.Run()` in [`operator.go`](operator.go) registers four controllers with `controller-runtime`:

| Controller Name    | Source Files                                                                                       | Watched Primary Resources                                                           | Managed Outputs                                                                                                                                                                                   |
|:-------------------|:---------------------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------|:--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `operator-config`  | [`operator_config.go`](operator_config.go)                                                         | `OperatorConfig` (`gmp-public/config`)                                              | Reconciles the `collector` DaemonSet and `rule-evaluator` Deployment in `gmp-system`, and mirrors referenced Secrets (`collection`, `rules`, `alertmanager`) from `gmp-public` into `gmp-system`. |
| `collector-config` | [`collection.go`](collection.go), [`splitting.go`](splitting.go)                                   | `PodMonitoring`, `ClusterPodMonitoring`, `ClusterNodeMonitoring`, `OperatorConfig`  | Validates scrape endpoints, generates gzipped Prometheus configuration in `Secret/gmp-system/collection`, and updates CRD status conditions.                                                      |
| `rules`            | [`rules.go`](rules.go), [`apis/monitoring/v1/rules_config.go`](apis/monitoring/v1/rules_config.go) | `Rules`, `ClusterRules`, `GlobalRules`                                              | Injects scope labels (`project_id`, `location`, `cluster`, `namespace`) into PromQL expressions and writes generated rule files to `ConfigMap/gmp-system/rules-generated`.                        |
| `target-status`    | [`endpoint_status.go`](endpoint_status.go)                                                         | `PodMonitoring`, `ClusterPodMonitoring`, `ClusterNodeMonitoring` + `collector` Pods | Polls `/api/v1/targets` on ready collector pods and populates `status.endpointStatuses` on the corresponding monitoring CRDs.                                                                     |

## Code Generation

> [!IMPORTANT]
> Never manually edit generated files under `charts/operator/crds/`, `manifests/`, `doc/api.md`, `pkg/operator/generated/`, or `pkg/operator/apis/monitoring/v1/zz_generated.deepcopy.go`. CI enforces a clean working tree via `CHECK=1 make regen`.

When modifying Go API types under [`pkg/operator/apis/monitoring/v1/`](apis/monitoring/v1/), run the following two steps from the repository root:

1. **Regenerate CRD YAMLs, Helm manifests, API documentation, and formatting**:

```bash
make regen
```

This executes [`./hack/presubmit.sh`](../../hack/presubmit.sh) (`crdgen`, `manifests`, `docgen`, and `format`), updating `charts/operator/crds/*.yaml`, `manifests/setup.yaml`, `manifests/operator.yaml`, `manifests/rule-evaluator.yaml`, and `doc/api.md`.

2. **Regenerate Go `DeepCopy` methods and typed client-go code (when structs or fields change)**:

```bash
./hack/presubmit.sh codegen
```

This runs `kube::codegen::gen_helpers` to update `pkg/operator/apis/monitoring/v1/zz_generated.deepcopy.go` and `pkg/operator/generated/`.

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

To run the `e2e/` suite directly against an existing Kubernetes cluster (such as GKE) where `manifests/setup.yaml` has been applied and in-cluster validating webhooks are not blocking local operator execution:

```bash
kubectl apply -f manifests/setup.yaml
go test ./e2e/ \
  -args -project-id="$PROJECT_ID" -cluster="$CLUSTER_NAME" -location="$LOCATION" -skip-gcm
```

Omit `-skip-gcm` (or pass `-gcp-service-account=<filepath>`) when verifying metric ingestion and querying against Google Cloud Monitoring.
