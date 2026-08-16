# GMP Migration Report: `n4_conflicting_keep.yaml`

## 1. Executive Summary

| Attribute | Value |
| :--- | :--- |
| **Source File** | [`hack/migration-eval/monitors/n4_conflicting_keep.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/n4_conflicting_keep.yaml) |
| **Source Resource** | `PodMonitor:eval-n4/conflict-keep-monitor` |
| **Target Resource** | `PodMonitoring:eval-n4/conflict-keep-monitor` |
| **CLI Exit Code** | `1` (`ActionItemsCount: 1`, `FailedCount: 0`) |
| **Migration Status** | **Migrated with Action Items** (Draft generated requiring label conflict resolution) |

### Migration Summary Counter
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

---

## 2. CLI Execution & Diagnostics

### Command Invocation
```bash
go run ./cmd/gmp-migrate --all -f hack/migration-eval/monitors/n4_conflicting_keep.yaml
```

### Tool Diagnostic Log (`Stderr`)
```
[INFO] [PodMonitor:eval-n4/conflict-keep-monitor] Successfully decoded PodMonitor
[INFO] [PodMonitor:eval-n4/conflict-keep-monitor] Converted target filtering relabeling rule ("__meta_kubernetes_pod_label_env" -> "production") to Pod Selector (matchLabels).
[WARNING] [PodMonitor:eval-n4/conflict-keep-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.
[WARNING] [PodMonitor:eval-n4/conflict-keep-monitor] Conflicting relabeling keep rules for label "env": cannot require both "production" and "staging" simultaneously. action=Define the intended label value for "env" in 'spec.selector.matchLabels'.
[SUCCESS] [PodMonitor:eval-n4/conflict-keep-monitor] Converted successfully
```

---

## 3. Input Manifest Analysis

In the source `PodMonitor`, two sequential `action: keep` relabeling rules are defined on the same target label (`__meta_kubernetes_pod_label_env`):
1. `regex: "production"` with `action: keep`
2. `regex: "staging"` with `action: keep`

```yaml
# Source: hack/migration-eval/monitors/n4_conflicting_keep.yaml
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata:
  name: conflict-keep-monitor
  namespace: eval-n4
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  podMetricsEndpoints:
  - port: metrics
    relabelings:
    - sourceLabels: ["__meta_kubernetes_pod_label_env"]
      regex: "production"
      action: keep
    - sourceLabels: ["__meta_kubernetes_pod_label_env"]
      regex: "staging"
      action: keep
```

### Relabeling Conflict Mechanics
In Prometheus Operator / Prometheus scrape relabeling:
* Each `action: keep` filter is executed sequentially.
* An `action: keep` drops any target whose `sourceLabels` do NOT match the `regex`.
* If a pod has `env=production`, the first rule keeps it, but the second rule (`regex: "staging"`) immediately drops it.
* If a pod has `env=staging`, the first rule (`regex: "production"`) immediately drops it.
* Consequently, having two separate sequential `keep` rules for different scalar values on the same label creates a logical contradiction that drops 100% of discovered targets unless resolved.

---

## 4. Generated Raw Draft Manifest (`--all` Mode)

`gmp-migrate` produced a draft manifest with `gmp.googleapis.com/todo-1` annotation and defaulted `spec.selector.matchLabels.env` to the first encountered rule value (`production`):

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[ERROR] Conflicting relabeling keep rules for label
      "env": cannot require both "production" and "staging" simultaneously. ACTION:
      Define the intended label value for "env" in ''spec.selector.matchLabels''.'
  creationTimestamp: null
  name: conflict-keep-monitor
  namespace: eval-n4
spec:
  endpoints:
  - interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
      env: production
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

---

## 5. Reconciliation Audit & Options (Recipe 5.2)

### Reconciliation Audit Table
| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring:eval-n4/conflict-keep-monitor` | `gmp.googleapis.com/todo-1` (Conflicting keep rules for label `env`) | Analyzed sequential keep relabelings in source manifest and workload in [`hack/migration-eval/workloads.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/workloads.yaml#L389-L415) | Clarified intended workload target (`production` vs `staging` vs multi-environment match expression). Cleared `todo-1` annotation and configured unambiguous pod selector. |

---

## 6. Resolution Plans & Code Diffs

Depending on the intended target environment, the following resolution options are available:

### Option A: Target Production Environment (Recommended Default)
Retain `env: production` in `matchLabels` and remove the TODO annotation.

#### Diff for `PodMonitoring:eval-n4/conflict-keep-monitor`:
```diff
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Conflicting relabeling keep rules for label
-      "env": cannot require both "production" and "staging" simultaneously. ACTION:
-      Define the intended label value for "env" in ''spec.selector.matchLabels''.'
   name: conflict-keep-monitor
   namespace: eval-n4
 spec:
   endpoints:
   - interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
       env: production
```

#### Companion Workload Patch (Deployment `payment-processor` in `eval-n4`):
```diff
 apiVersion: apps/v1
 kind: Deployment
 metadata:
   name: payment-processor
   namespace: eval-n4
 spec:
   template:
     metadata:
       labels:
         app.kubernetes.io/name: payment-processor
+        env: production
```

---

### Option B: Target Staging Environment
Update `env` to `staging` in `matchLabels` and remove the TODO annotation.

#### Diff for `PodMonitoring:eval-n4/conflict-keep-monitor`:
```diff
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Conflicting relabeling keep rules for label
-      "env": cannot require both "production" and "staging" simultaneously. ACTION:
-      Define the intended label value for "env" in ''spec.selector.matchLabels''.'
   name: conflict-keep-monitor
   namespace: eval-n4
 spec:
   endpoints:
   - interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
-      env: production
+      env: staging
```

---

### Option C: Target Both Environments (Multi-Valued Match Expression)
If the original author intended to scrape pods belonging to either `production` OR `staging` (i.e. regex `production|staging`), use `matchExpressions` with operator `In`:

#### Diff for `PodMonitoring:eval-n4/conflict-keep-monitor`:
```diff
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Conflicting relabeling keep rules for label
-      "env": cannot require both "production" and "staging" simultaneously. ACTION:
-      Define the intended label value for "env" in ''spec.selector.matchLabels''.'
   name: conflict-keep-monitor
   namespace: eval-n4
 spec:
   endpoints:
   - interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
-      env: production
+    matchExpressions:
+    - key: env
+      operator: In
+      values:
+      - production
+      - staging
```

---

## 7. Reconciled Ready-to-Apply Manifest (Option A)

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: conflict-keep-monitor
  namespace: eval-n4
spec:
  endpoints:
  - interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
      env: production
```

---

## 8. Verification & Next Steps

1. **Dry-Run Validation**:
   ```bash
   kubectl apply --dry-run=client -f <reconciled_manifest.yaml>
   ```
2. **Workload Label Alignment**: Verify target deployment in `eval-n4` has matching `env: production` (or `staging`) label on its pod template.
3. **Status Check**: Once applied to a live GKE/GMP cluster, verify `status.conditions` on the `PodMonitoring` resource:
   ```bash
   kubectl describe podmonitoring conflict-keep-monitor -n eval-n4
   ```
