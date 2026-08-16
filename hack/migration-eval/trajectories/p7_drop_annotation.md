# GMP Migration Report: `hack/migration-eval/monitors/p7_drop_annotation.yaml`

## 1. Executive Summary
- **Source Manifest**: [`hack/migration-eval/monitors/p7_drop_annotation.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/p7_drop_annotation.yaml)
- **Source Resource**: `PodMonitor: eval-p7/drop-annotation-monitor`
- **Target Resource**: `PodMonitoring: eval-p7/drop-annotation-monitor`
- **Migration Status**: **Reconciled & Ready** (Clean Parity achieved via Recipe 5.1 Case B)

---

## 2. CLI Execution & Diagnostics Summary
Running `gmp-migrate --all -f hack/migration-eval/monitors/p7_drop_annotation.yaml` generated the draft configuration with the following diagnostics:

```
[INFO] [PodMonitor:eval-p7/drop-annotation-monitor] Successfully decoded PodMonitor
[WARNING] [PodMonitor:eval-p7/drop-annotation-monitor] Relabeling rule referencing pod annotation "__meta_kubernetes_pod_annotation_prometheus_io_scrape" is unsupported in GMP. The rule has been dropped.
[WARNING] [PodMonitor:eval-p7/drop-annotation-monitor] Scraping scope expanded: targets previously excluded by this rule will now be scraped. Adjust pod selectors to compensate.
[WARNING] [PodMonitor:eval-p7/drop-annotation-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.
[WARNING] [PodMonitor:eval-p7/drop-annotation-monitor] Dropped target filtering rule ('drop' on '__meta_kubernetes_pod_annotation_prometheus_io_scrape'). action=Add equivalent pod label selector in 'spec.selector.matchLabels'.
[SUCCESS] [PodMonitor:eval-p7/drop-annotation-monitor] Converted successfully
```

---

## 3. Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring: eval-p7/drop-annotation-monitor` | `gmp.googleapis.com/todo-1` (Scope Expansion / Dropped Drop Rule) | Inspected [`hack/migration-eval/workloads.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/workloads.yaml#L273-L299) in namespace `eval-p7`. Found Deployment `payment-processor` with template annotation `prometheus.io/scrape: "false"`. | Added `spec.selector.matchExpressions` with `operator: NotIn`, `key: prometheus.io/scrape`, `values: ["false"]` to `PodMonitoring`, and added companion label `prometheus.io/scrape: "false"` to `payment-processor` Deployment pod template. Removed TODO annotation. |

---

## 4. Final Reconciled Manifests

### 4.1 Reconciled `PodMonitoring` Manifest
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: drop-annotation-monitor
  namespace: eval-p7
spec:
  endpoints:
  - interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
    matchExpressions:
    - key: prometheus.io/scrape
      operator: NotIn
      values:
      - "false"
```

### 4.2 Companion Workload Patch (`Deployment: eval-p7/payment-processor`)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payment-processor
  namespace: eval-p7
spec:
  template:
    metadata:
      labels:
        app.kubernetes.io/name: payment-processor
        prometheus.io/scrape: "false"
```

---

## 5. Per-File Code Diffs

### Diff: `PodMonitoring` Conversion & TODO Cleanup
```diff
--- a/eval-p7/drop-annotation-monitor (draft)
+++ b/eval-p7/drop-annotation-monitor (reconciled)
@@ -1,13 +1,14 @@
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[WARNING] Dropped target filtering rule (''drop''
-      on ''__meta_kubernetes_pod_annotation_prometheus_io_scrape''). ACTION: Add equivalent
-      pod label selector in ''spec.selector.matchLabels''.'
   name: drop-annotation-monitor
   namespace: eval-p7
 spec:
   endpoints:
   - interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
+    matchExpressions:
+    - key: prometheus.io/scrape
+      operator: NotIn
+      values:
+      - "false"
```

### Diff: Target Workload Companion Patch (`hack/migration-eval/workloads.yaml`)
```diff
--- a/hack/migration-eval/workloads.yaml (eval-p7/payment-processor)
+++ b/hack/migration-eval/workloads.yaml (eval-p7/payment-processor)
@@ -288,6 +288,7 @@
     spec:
       template:
         metadata:
           labels:
             app.kubernetes.io/name: payment-processor
+            prometheus.io/scrape: "false"
           annotations:
             prometheus.io/scrape: "false"
```
