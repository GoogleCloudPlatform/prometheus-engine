# GMP Migration Report: `hack/migration-eval/monitors/p8_empty_selector.yaml`

## 1. Migration Overview & Initial Diagnostics

- **Source Manifest**: [`hack/migration-eval/monitors/p8_empty_selector.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/p8_empty_selector.yaml)
- **Source Resource**: `PodMonitor: eval-p8/wildcard-empty-monitor`
- **Target Resource**: `PodMonitoring: eval-p8/wildcard-empty-monitor`
- **CLI Execution**: `gmp-migrate --all -f hack/migration-eval/monitors/p8_empty_selector.yaml`

### Initial Migration Summary
```
=========================================
Migration Complete Summary:
  Successfully Migrated:      0
  Migrated with Warnings:     0
  Migrated with Action Items: 1
  Skipped (Unsupported):      0
  Failed:                     0
=========================================
```

### Initial CLI Output & Diagnostics:
- `[WARNING] [PodMonitor:eval-p8/wildcard-empty-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.`
- `[WARNING] [PodMonitor:eval-p8/wildcard-empty-monitor] Resulting PodMonitoring selector is empty. It will select and scrape all pods in this namespace. Verify if this is intended.`
- Emitted draft manifest with annotation `gmp.googleapis.com/todo-1`: `[WARNING] Resulting PodMonitoring selector is empty and matches all pods in this namespace. ACTION: Define explicit 'matchLabels' in 'spec.selector'.`

---

## 2. Investigation & Context Discovery (Recipe 6.2)

Following Recipe 6.2 of the `gmp-migrate` skill:

1. **Workload Discovery**:
   - Query: `kubectl get deployment,daemonset,statefulset -n eval-p8 -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.template.metadata.labels}{"\n"}{end}'`
   - Discovered: Workload `Deployment/payment-processor` in namespace `eval-p8` with pod labels `{"app.kubernetes.io/name":"payment-processor"}`.
2. **Pod Container Ports**:
   - Query: `kubectl get pods -n eval-p8 -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[*].ports}{"\n"}{end}'`
   - Discovered: Container exposing port `name: metrics` (`containerPort: 8080`).

---

## 3. Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Diagnostic | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring: eval-p8/wildcard-empty-monitor` | `gmp.googleapis.com/todo-1` (Empty Selector Warning) | Inspected workloads in namespace `eval-p8` via `kubectl get deployment` | Discovered `payment-processor` (`app.kubernetes.io/name: payment-processor`). Populated explicit `spec.selector.matchLabels` to prevent unintended namespace-wide scraping, and removed TODO annotation. |
| `PodMonitoring: eval-p8/wildcard-empty-monitor` | `[WARNING]` (Missing Scrape Interval) | `gmp-migrate` default duration handling | Populated `spec.endpoints[0].interval: 30s` (mandatory in GMP CRDs). |

---

## 4. Reconciled Manifests & Diffs

### Option A (Recommended): Explicit Workload Targeting
Targets the discovered `payment-processor` deployment:

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: wildcard-empty-monitor
  namespace: eval-p8
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  endpoints:
  - port: metrics
    interval: 30s
```

#### Diff against Initial Migration Draft:
```diff
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[WARNING] Resulting PodMonitoring selector is empty
-      and matches all pods in this namespace. ACTION: Define explicit ''matchLabels''
-      in ''spec.selector''.'
-  creationTimestamp: null
   name: wildcard-empty-monitor
   namespace: eval-p8
 spec:
   endpoints:
   - interval: 30s
     port: metrics
-  selector: {}
-  targetLabels:
-    metadata: null
-status:
-  observedGeneration: 0
+  selector:
+    matchLabels:
+      app.kubernetes.io/name: payment-processor
```

### Option B: Wildcard Namespace-Wide Scraping (If Intended)
If scraping all pods exposing port `metrics` in namespace `eval-p8` was intentional:

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: wildcard-empty-monitor
  namespace: eval-p8
spec:
  selector: {}
  endpoints:
  - port: metrics
    interval: 30s
```

---

## 5. Verification & Dry-Run Validation

1. **Server-Side Dry Run (Option A - Explicit Selector)**:
   ```bash
   kubectl apply --dry-run=server -f -
   # Output: podmonitoring.monitoring.googleapis.com/wildcard-empty-monitor created (server dry run)
   ```
   **Status**: Passed.

2. **Server-Side Dry Run (Option B - Wildcard Selector)**:
   ```bash
   kubectl apply --dry-run=server -f -
   # Output: podmonitoring.monitoring.googleapis.com/wildcard-empty-monitor created (server dry run)
   ```
   **Status**: Passed.

Migration and reconciliation for `hack/migration-eval/monitors/p8_empty_selector.yaml` is complete and verified.
