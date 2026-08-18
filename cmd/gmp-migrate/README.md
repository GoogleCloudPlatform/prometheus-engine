# gmp-migrate

`gmp-migrate` is a migration CLI tool designed to translate [Prometheus Operator](https://prometheus-operator.dev/) monitoring resources to [Google Cloud Managed Service for Prometheus (GMP)](https://cloud.google.com/stackdriver/docs/managed-prometheus) Custom Resources.

---

## Supported Resources & Dependencies

`gmp-migrate` strictly distinguishes between **target resources to convert** and **backing dependencies** needed for cross-resource resolution:

* **Target Resources**:
  * `monitoring.coreos.com/v1.PodMonitor` → Converted to `PodMonitoring` (or `ClusterPodMonitoring` if cluster-scoped).
  * `monitoring.coreos.com/v1.ServiceMonitor` → Converted to `PodMonitoring` (or `ClusterPodMonitoring`), resolved to backing Pods via `Service`.
* **Backing Dependencies** (Ingested for resolution, not emitted directly):
  * `v1.Service`: Resolves endpoint port names to container ports and service selectors to Pod selectors.
  * `v1.Secret`: Validates referenced authentication credentials (`basicAuth`, `authorization`, `oauth2`).
  * `v1.ConfigMap`: Ingested when referenced in TLS CAs to automatically synthesize companion `v1.Secret` manifests.

---

## Feature Translation & Parity Matrix

### 1. Resource & Scoping Translation

| Prometheus Operator Field                  | Status        | GMP Translation Behavior                                                                          |
|:-------------------------------------------|:-------------:|:--------------------------------------------------------------------------------------------------|
| `kind: PodMonitor`                         | `1:1 Parity`  | Converted to `monitoring.googleapis.com/v1.PodMonitoring`.                                        |
| `kind: ServiceMonitor`                     | `Transformed` | Converted to `PodMonitoring` (or `ClusterPodMonitoring`), resolved to backing Pods via `Service`. |
| `spec.namespaceSelector.any: true`         | `Transformed` | Converted to cluster-scoped `ClusterPodMonitoring`.                                               |
| `spec.namespaceSelector.matchNames: [...]` | `Transformed` | Generates distinct `PodMonitoring` manifests for each targeted namespace.                         |
| Omitted `namespaceSelector`                | `1:1 Parity`  | Preserves source namespace in `PodMonitoring`.                                                    |
| `v1.Service`, `v1.Secret`, `v1.ConfigMap`  | `Dependency`  | Ingested to resolve ports, selectors, credentials, and TLS CAs.                                   |

### 2. Workload & Service Resolution

| Prometheus Operator Field           | Status        | GMP Translation Behavior                                                                                    |
|:------------------------------------|:-------------:|:------------------------------------------------------------------------------------------------------------|
| `spec.selector` (`PodMonitor`)      | `1:1 Parity`  | Mapped directly to `spec.selector.matchLabels` / `matchExpressions`.                                        |
| `spec.selector` (`ServiceMonitor`)  | `Transformed` | Matches backing `Service`; extracts Service's pod selector into `PodMonitoring.spec.selector`.              |
| Multiple Services matching selector | `Transformed` | Compatible Services are merged; conflicting selectors/ports split into suffixed resources (`<name>-<svc>`). |
| Empty selector (`matchLabels: {}`)  | `Transformed` | Retained (scrapes all pods in namespace/cluster) with a warning TODO for operator confirmation.             |

### 3. Endpoint & Port Configuration

| Prometheus Operator Field                 | Status        | GMP Translation Behavior                                                                                                                   |
|:------------------------------------------|:-------------:|:-------------------------------------------------------------------------------------------------------------------------------------------|
| `port` (named string on `ServiceMonitor`) | `Transformed` | Resolved via `Service.spec.ports` to the container's numeric or named `targetPort`.                                                        |
| `port` (named string on `PodMonitor`)     | `1:1 Parity`  | Mapped directly to `spec.endpoints[].port`.                                                                                                |
| `targetPort` (integer or name)            | `1:1 Parity`  | Mapped directly to `spec.endpoints[].port`.                                                                                                |
| `path`, `scheme`, `params`                | `1:1 Parity`  | Mapped directly to `spec.endpoints[]`.                                                                                                     |
| `interval` / `scrapeTimeout`              | `Transformed` | Normalized to Go duration strings; automatically enforces safety cap (`timeout <= interval`).                                              |
| `proxyUrl`                                | `Transformed` | Unauthenticated URLs mapped; embedded basic auth (`user:pass@`) is stripped with a warning.                                                |
| `followRedirects`, `enableHttp2`          | `Unsupported` | Dropped with warning (GMP collectors always follow redirects and negotiate HTTP/2 for TLS).                                                |
| `honorLabels`                             | `Unsupported` | Dropped with warning (GMP managed target labels always take precedence; conflicting metric labels are renamed with an `exported_` prefix). |
| `honorTimestamps`                         | `Unsupported` | Dropped with warning (GMP always uses the scrape ingestion timestamp; target metric timestamps are ignored).                               |
| `trackTimestampsStaleness`                | `Unsupported` | Dropped with warning (currently unsupported in GMP CRDs).                                                                                  |

### 4. Authentication & TLS

| Prometheus Operator Field                    | Status        | GMP Translation Behavior                                                                        |
|:---------------------------------------------|:-------------:|:------------------------------------------------------------------------------------------------|
| `basicAuth.password`                         | `1:1 Parity`  | Mapped to Secret reference: `spec.endpoints[].basicAuth.password.secret`.                       |
| `basicAuth.username`                         | `Transformed` | Mapped as direct string literal in `spec.endpoints[].basicAuth.username`.                       |
| `bearerTokenSecret` / `authorization`        | `1:1 Parity`  | Mapped to GMP `spec.endpoints[].authorization` (`credentials.secret`).                          |
| `oauth2`                                     | `Transformed` | `clientID` and `tokenURL` mapped as string literals; `clientSecret` mapped to Secret reference. |
| `tlsConfig.ca.configMap`                     | `Transformed` | Automatically synthesizes companion `v1.Secret` (`secret-<configmap-name>`) for GMP compliance. |
| `tlsConfig.ca.secret`, `cert`, `keySecret`   | `1:1 Parity`  | Mapped directly to `spec.endpoints[].tls`.                                                      |
| `tlsConfig.insecureSkipVerify`, `serverName` | `1:1 Parity`  | Mapped directly to `spec.endpoints[].tls`.                                                      |
| `caFile`, `certFile`, `keyFile`              | `Unsupported` | Dropped with warning (GMP hermetic containers require Secret references, not host paths).       |

### 5. Relabeling, Target Labels & Limits

| Prometheus Operator Field                                                    | Status        | GMP Translation Behavior                                                                               |
|:-----------------------------------------------------------------------------|:-------------:|:-------------------------------------------------------------------------------------------------------|
| `metricRelabelConfigs`                                                       | `1:1 Parity`  | Mapped directly to `spec.endpoints[].metricRelabeling`.                                                |
| `targetLabels` (`ServiceMonitor`)                                            | `Transformed` | Statically mapped to post-scrape `metricRelabeling` rules per service group.                           |
| `podTargetLabels` (`PodMonitor` / `ServiceMonitor`)                          | `Transformed` | Mapped to `spec.targetLabels.fromPod`.                                                                 |
| Pod label extraction (`__meta_kubernetes_pod_label_*`)                       | `Transformed` | Promoted from pre-scrape relabeling to `spec.targetLabels.fromPod`.                                    |
| Metadata extraction (`__meta_kubernetes_pod_name`, etc.)                     | `Transformed` | Promoted to `spec.targetLabels.metadata`.                                                              |
| Pod annotation filtering (`keep`/`drop` on annotations)                      | `Unsupported` | Dropped with warning & TODO (GMP `spec.selector` only filters on Pod labels).                          |
| `sampleLimit`, `labelLimit`, `labelNameLengthLimit`, `labelValueLengthLimit` | `1:1 Parity`  | Mapped directly to `spec.limits`.                                                                      |
| `targetLimit`, `bodySizeLimit`, `keepDroppedTargets`                         | `Unsupported` | Dropped with warning (limits are managed at the collector infrastructure and GCM project quota level). |

---

## Installation & Building

### Prerequisites
* Go 1.24+

### Build Binary

```bash
# Build binary to ./build/bin/gmp-migrate
NO_DOCKER=1 make gmp-migrate
```

### Run Directly via `go run`

```bash
go run ./cmd/gmp-migrate -f <input-path>
```

---

## Invocation Pipeline Routes

### CLI Flags

```bash mdox-exec="bash hack/format_help.sh gmp-migrate"
Usage of gmp-migrate:
Migrate Prometheus Operator configurations to Google Managed Prometheus (GMP).

  -a	Emit all manifests, including best-effort draft configurations with TODO annotations
  -all
    	Emit all manifests, including best-effort draft configurations with TODO annotations
  -f value
    	Input source (YAML file, directory, or '-' for stdin) (Required)
  -file value
    	Input source (YAML file, directory, or '-' for stdin) (Required)
```

### Route 1: Local Files & GitOps Repositories (File-to-File)

```bash
# Ready-only mode (Default: emits only 100% complete manifests)
gmp-migrate -f path/to/monitors/ -f path/to/services/ > ready_gmp_manifests.yaml 2> migration.log

# Full Review mode (--all: emits all manifests, including drafts with TODO annotations)
gmp-migrate --all -f path/to/monitors/ -f path/to/services/ > all_gmp_manifests.yaml 2> migration.log
```

### Route 2: Live Cluster Extraction (Cluster-to-File)

```bash
kubectl get podmonitors,servicemonitors,services,configmaps,secrets -A -o yaml | \
  gmp-migrate --all -f - > gmp_manifests.yaml 2> migration.log
```

### Route 3: Live Cluster Migration with Dry-Run Validation (Cluster-to-Cluster)

```bash
# Step 1: Validate with Server-Side Dry-Run (Safe, non-mutating)
kubectl get podmonitors,servicemonitors,services,configmaps,secrets -A -o yaml | \
  gmp-migrate -f - | kubectl apply --server-side --dry-run=server -f -

# Step 2: Apply 100% production-ready manifests directly
kubectl get podmonitors,servicemonitors,services,configmaps,secrets -A -o yaml | \
  gmp-migrate -f - | kubectl apply --server-side -f -
```

---

## Operational Considerations & Safety Guardrails

> [!IMPORTANT]
> Review these critical considerations before migrating resources in production environments:

1. **Do NOT Pipe `--all` Directly to `kubectl apply`**: `--all` emits best-effort draft manifests containing `TODO_*` placeholder values. Applying them directly will cause CRD validation or collector ingestion errors.
2. **Always Provide Backing Services for `ServiceMonitor`**: If a `ServiceMonitor` is passed without its corresponding `Service`, `gmp-migrate` will emit placeholders (`TODO_RESOLVE_PORT`, `TODO_SET_POD_LABELS`).
3. **Multi-Namespace Secret Isolation**: Kubernetes forbids cross-namespace Secret references. If a monitor selects multiple namespaces (`matchNames: [...]`), referenced Secrets must exist in **each** target namespace.
4. **Scope Expansion from Dropped Relabeling Rules**: When pod annotation filtering rules (`action: keep/drop`) are dropped, ensure equivalent Pod labels are applied to target workloads to prevent unintended scraping.
5. **Wildcard Selector Verification**: In GMP, an empty selector (`matchLabels: {}`) matches all pods in the namespace/cluster. Confirm whether wildcard collection is intentional.

---

## Diagnostic Logs & Exit Codes

```text
=========================================
Migration Complete Summary:
  Successfully Migrated:      X
  Migrated with Warnings:     Y
  Migrated with Action Items: Z
  Skipped (Unsupported):      W
  Failed:                     V
=========================================
```

| Exit Code | Outcome                  | Description                                                                                                | Behavior                                                                                                                                        |
|:---------:|:-------------------------|:-----------------------------------------------------------------------------------------------------------|:------------------------------------------------------------------------------------------------------------------------------------------------|
|  **`0`**  | **Clean Parity**         | All input resources converted cleanly with no unresolved items or fatal errors.                            | All converted manifests written to `Stdout`.                                                                                                    |
|  **`1`**  | **Action Items Present** | Converted manifests contain items requiring operator review (e.g. unresolved port names, missing Secrets). | Default mode: emits only clean manifests to `Stdout`.<br>`--all` mode: emits all manifests with inline `gmp.googleapis.com/todo-*` annotations. |
|  **`1`**  | **Fatal Error**          | One or more resources encountered fatal parsing or conversion errors (e.g. malformed YAML).                | Zero manifests written to `Stdout`. Diagnostic errors logged to `Stderr`.                                                                       |

---

## Example Walkthrough

### 1. Input Manifests

```yaml
# input.yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend-svc
  namespace: web
  labels:
    app: frontend
spec:
  selector:
    app: frontend-pod
  ports:
  - name: web-metrics
    port: 80
    targetPort: 8080
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: frontend-monitor
  namespace: web
  labels:
    app: frontend
spec:
  selector:
    matchLabels:
      app: frontend
  endpoints:
  - port: web-metrics
    interval: 30s
    path: /metrics
```

### 2. Run Migration

```bash
gmp-migrate -f input.yaml > output.yaml
```

### 3. Diagnostic Output (`Stderr`)

```text
[INFO] [Service:web/frontend-svc] Ingested backing service
[INFO] [ServiceMonitor:web/frontend-monitor] Successfully decoded ServiceMonitor
[INFO] [ServiceMonitor:web/frontend-monitor] Converted successfully

=========================================
Migration Complete Summary:
  Successfully Migrated:      1
  Migrated with Warnings:     0
  Migrated with Action Items: 0
  Skipped (Unsupported):      0
  Failed:                     0
=========================================
```

### 4. Converted Output (`output.yaml`)

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: frontend-monitor
  namespace: web
spec:
  endpoints:
  - interval: 30s
    path: /metrics
    port: 8080
  selector:
    matchLabels:
      app: frontend-pod
```

---

## Handling Action Items & TODO Annotations

When running with `--all`, draft configurations that require review will include inline annotations and placeholders:

```yaml
metadata:
  annotations:
    gmp.googleapis.com/todo-1: "[WARNING] Named port 'metrics' was not found on Service 'my-svc'... ACTION: Replace TODO_RESOLVE_PORT_METRICS with target container port name or number."
spec:
  endpoints:
  - port: TODO_RESOLVE_PORT_METRICS
```

To resolve action items:
1. Replace the `TODO_*` placeholder with the intended port, label, or value.
2. Remove the `gmp.googleapis.com/todo-*` annotation from `metadata.annotations`.
3. Apply the validated manifest to your cluster.

---

## AI Agent Integration & Skill

For users pair-programming with AI coding assistants (such as Google Jetski or Gemini Code Assist), this repository includes a companion reconciliation skill located at [SKILL.md](./SKILL.md).

The skill guides AI assistants to execute `gmp-migrate`, parse diagnostic logs, prompt operators for missing credentials without exposing secrets into chat logs, resolve port placeholders, and generate GitOps-ready diffs.

---

## Development & Testing

```bash
# Run unit tests
go test -v ./pkg/migrate/...

# Run linter
make lint
```
