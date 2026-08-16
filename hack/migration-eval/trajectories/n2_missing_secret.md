# Migration Evaluation Report: `hack/migration-eval/monitors/n2_missing_secret.yaml`

The migration for `hack/migration-eval/monitors/n2_missing_secret.yaml` has been executed and evaluated using the `gmp-migrate` skill.

---

## 1. Executive Summary

| Attribute | Value |
| :--- | :--- |
| **Input Source** | `hack/migration-eval/monitors/n2_missing_secret.yaml` |
| **Target Resource** | `PodMonitoring: eval-n2/missing-secret-monitor` |
| **CLI Exit Code** | `1` (Action Items Present) |
| **Status** | Draft Generated with Action Items (`ActionItemsCount: 1`) |

### Migration Statistics
- **Successfully Migrated**: 0
- **Migrated with Warnings**: 0
- **Migrated with Action Items**: 1
- **Skipped (Unsupported)**: 0
- **Failed**: 0

---

## 2. CLI Execution & Diagnostics

### Command Executed
```bash
gmp-migrate --all -f hack/migration-eval/monitors/n2_missing_secret.yaml
```

### Diagnostics Log (Stderr)
```text
[INFO] [PodMonitor:eval-n2/missing-secret-monitor] Successfully decoded PodMonitor
[WARNING] [PodMonitor:eval-n2/missing-secret-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.
[WARNING] [PodMonitor:eval-n2/missing-secret-monitor] Referenced SECRET "ghost-secret" for key "username" was not found in migration inputs. action=Verify SECRET "ghost-secret" exists in namespace "eval-n2" or provide it in migration inputs.
[SUCCESS] [PodMonitor:eval-n2/missing-secret-monitor] Converted successfully
```

---

## 3. Root Cause Analysis

1. **Missing Secret Dependency (`ghost-secret`)**:
   - The source `PodMonitor` specifies `basicAuth` referencing Secret `ghost-secret` for `username` (key: `username`) and `password` (key: `password`).
   - In Prometheus Operator, `basicAuth.username` is a `SecretKeySelector`. In GMP CRDs, `spec.endpoints[].basicAuth.username` is a plaintext `string`, while `password` is a `SecretSelector` (`secret: {name, key}`).
   - Because `ghost-secret` was not provided in the input bundle and does not exist in namespace `eval-n2`, `gmp-migrate` safely produced a draft manifest with the placeholder `TODO_SET_USERNAME_FROM_SECRET_GHOST-SECRET` and attached the action item annotation `gmp.googleapis.com/todo-1`.
2. **Scrape Interval Defaulting**:
   - `podMetricsEndpoints[0]` omitted `interval`. GMP strictly requires `spec.endpoints[].interval`, so `gmp-migrate` safely defaulted it to `30s`.

---

## 4. Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring: eval-n2/missing-secret-monitor` | `gmp.googleapis.com/todo-1`<br/>(`TODO_SET_USERNAME_FROM_SECRET_GHOST-SECRET`) | Checked `hack/migration-eval/workloads.yaml` and cluster namespace `eval-n2`; Secret `ghost-secret` is absent. | **Negative Scenario N2**: Flagged missing Secret dependency. Once `ghost-secret` is created or provided, replace `TODO_SET_USERNAME_FROM_SECRET_GHOST-SECRET` with the decoded username and remove the TODO annotation. |
| `PodMonitoring: eval-n2/missing-secret-monitor` | Interval Defaulting | `podMetricsEndpoints[0].interval` was empty in source YAML. | Automatically populated `interval: 30s` per GMP CRD requirements. |

---

## 5. Generated Manifest & Diff

### Draft Manifest Emitted (`--all`)
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[ERROR] Referenced SECRET "ghost-secret" for key "username"
      was not found in migration inputs. ACTION: Verify SECRET "ghost-secret" exists
      in namespace "eval-n2" or provide it in migration inputs.'
  creationTimestamp: null
  name: missing-secret-monitor
  namespace: eval-n2
spec:
  endpoints:
  - basicAuth:
      password:
        secret:
          key: password
          name: ghost-secret
      username: TODO_SET_USERNAME_FROM_SECRET_GHOST-SECRET
    interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

### Comparison Diff (Upstream PodMonitor vs Reconciled PodMonitoring)
```diff
-apiVersion: monitoring.coreos.com/v1
-kind: PodMonitor
+apiVersion: monitoring.googleapis.com/v1
+kind: PodMonitoring
 metadata:
   name: missing-secret-monitor
   namespace: eval-n2
 spec:
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
-  podMetricsEndpoints:
+  endpoints:
   - port: metrics
+    interval: 30s
     basicAuth:
-      username:
-        name: ghost-secret
-        key: username
+      username: <RECONCILED_USERNAME>
       password:
-        name: ghost-secret
-        key: password
+        secret:
+          name: ghost-secret
+          key: password
```

Full artifact report generated at: `/usr/local/google/home/kunnikrishnan/.gemini/jetski/brain/3b6ff93b-c240-45d2-ba69-256ec1ad1cfa/migration_report_n2_missing_secret.md`
