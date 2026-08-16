# Migration Report: `hack/migration-eval/monitors/p6_keep_annotation.yaml`

## 1. Executive Summary

- **Source Manifest:** [p6_keep_annotation.yaml](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/p6_keep_annotation.yaml) (`kind: PodMonitor`, namespace: `eval-p6`, name: `keep-annotation-monitor`)
- **Backing Workload:** [workloads.yaml](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/workloads.yaml#L243-L269) (`kind: Deployment`, namespace: `eval-p6`, name: `payment-processor`)
- **Initial Migration Status:** Exit code `1` (1 Action Item requiring reconciliation, 0 fatal errors).
- **Final Status:** **100% Reconciled** with companion workload patch.

---

## 2. CLI Migration Execution & Initial Output

Executed `gmp-migrate` in review mode:
```bash
gmp-migrate --all -f hack/migration-eval/monitors/p6_keep_annotation.yaml
```

### CLI Summary & Diagnostics
```
[INFO] [PodMonitor:eval-p6/keep-annotation-monitor] Successfully decoded PodMonitor
[WARNING] [PodMonitor:eval-p6/keep-annotation-monitor] Relabeling rule referencing pod annotation "__meta_kubernetes_pod_annotation_prometheus_io_scrape" is unsupported in GMP. The rule has been dropped.
[WARNING] [PodMonitor:eval-p6/keep-annotation-monitor] Scraping scope expanded: targets previously excluded by this rule will now be scraped. Adjust pod selectors to compensate.
[WARNING] [PodMonitor:eval-p6/keep-annotation-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.
[WARNING] [PodMonitor:eval-p6/keep-annotation-monitor] Dropped target filtering rule ('keep' on '__meta_kubernetes_pod_annotation_prometheus_io_scrape'). action=Add equivalent pod label selector in 'spec.selector.matchLabels'.
[SUCCESS] [PodMonitor:eval-p6/keep-annotation-monitor] Converted successfully

=========================================
Migration Complete Summary:
  Successfully Migrated:      0
  Migrated with Warnings:     0
  Migrated with Action Items: 1
  Skipped (Unsupported):      0
  Failed:                     0
=========================================
```

### Initial Best-Effort Draft Manifest
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[WARNING] Dropped target filtering rule (''keep''
      on ''__meta_kubernetes_pod_annotation_prometheus_io_scrape''). ACTION: Add equivalent
      pod label selector in ''spec.selector.matchLabels''.'
  creationTimestamp: null
  name: keep-annotation-monitor
  namespace: eval-p6
spec:
  endpoints:
  - interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

---

## 3. Reconciliation Analysis (Recipe 5.1)

### Diagnostic Cause
Prometheus Operator supported relabeling rules matching metadata annotations (`__meta_kubernetes_pod_annotation_prometheus_io_scrape: "true"` with `action: keep`). Google Managed Prometheus (GMP) `spec.selector` strictly matches **Kubernetes Pod labels**, not annotations. Dropping this rule without adjusting selectors causes a scraping scope expansion.

### Investigation & Workload Corroboration
Inspected backing workload `Deployment/payment-processor` in [workloads.yaml](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/workloads.yaml#L243-L269):
- **Workload:** `payment-processor` in namespace `eval-p6`
- **Pod Template Labels:** `app.kubernetes.io/name: payment-processor`
- **Pod Template Annotations:** `prometheus.io/scrape: "true"`
- **Container Port:** `metrics` (port 8080)

### Reconciliation Strategy
1. **Promote Annotation to Label Selector:** Add `prometheus.io/scrape: "true"` to `PodMonitoring.spec.selector.matchLabels`.
2. **Companion Workload Patch:** Add label `prometheus.io/scrape: "true"` to `spec.template.metadata.labels` on `Deployment/payment-processor` in namespace `eval-p6`.
3. **Clean Up Annotations:** Remove `gmp.googleapis.com/todo-1` annotation and boilerplate null metadata/status fields.

---

## 4. Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring:eval-p6/keep-annotation-monitor` | `gmp.googleapis.com/todo-1` (Scope Expansion: Dropped Pre-Scrape Keep Rule) | Inspected pod annotations in `Deployment/payment-processor` in `workloads.yaml` | Promoted `prometheus.io/scrape: "true"` into `spec.selector.matchLabels` and created companion workload label patch. Cleaned up TODO annotations. |

---

## 5. Final Reconciled Manifests & Diffs

### Final Reconciled `PodMonitoring` Manifest
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: keep-annotation-monitor
  namespace: eval-p6
spec:
  endpoints:
  - interval: 30s
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
      prometheus.io/scrape: "true"
```

### Diff: Initial Draft vs Reconciled `PodMonitoring`
```diff
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[WARNING] Dropped target filtering rule (''keep''
-      on ''__meta_kubernetes_pod_annotation_prometheus_io_scrape''). ACTION: Add equivalent
-      pod label selector in ''spec.selector.matchLabels''.'
-  creationTimestamp: null
   name: keep-annotation-monitor
   namespace: eval-p6
 spec:
   endpoints:
   - interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
-  targetLabels:
-    metadata: null
-status:
-  observedGeneration: 0
+      prometheus.io/scrape: "true"
```

### Companion Workload Patch Diff
```diff
--- a/hack/migration-eval/workloads.yaml
+++ b/hack/migration-eval/workloads.yaml
@@ -257,6 +257,7 @@
     metadata:
       labels:
         app.kubernetes.io/name: payment-processor
+        prometheus.io/scrape: "true"
       annotations:
         prometheus.io/scrape: "true"
     spec:
```

---

## 6. Operational Notes & Safety Warning

> [!WARNING]
> Applying the companion workload patch to update `spec.template.metadata.labels` on `Deployment/payment-processor` will trigger a standard Kubernetes rolling restart of the pods. Ensure this change is rolled out during a standard deployment or maintenance window.
