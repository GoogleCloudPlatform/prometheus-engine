# Migration Report: `hack/migration-eval/monitors/p3_configmap_tls.yaml`

## 1. Migration Execution Summary
- **Command**: `gmp-migrate --all -f hack/migration-eval/monitors/p3_configmap_tls.yaml -f hack/migration-eval/workloads.yaml`
- **Exit Code**: `0` (Success)
- **Status**: Clean Parity / Ready with Warnings (0 Action Items, 0 Failures)

```
=========================================
Migration Complete Summary:
  Successfully Migrated:      0
  Migrated with Warnings:     1
  Migrated with Action Items: 0
  Skipped (Unsupported):      22
  Failed:                     0
=========================================
```

---

## 2. Telemetry & Diagnostics Log
- `[INFO] [PodMonitor:eval-p3/tls-ca-monitor] Successfully decoded PodMonitor`
- `[WARNING] [PodMonitor:eval-p3/tls-ca-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.`
- `[INFO] [PodMonitor:eval-p3/tls-ca-monitor] Translated TLS ConfigMap reference to GMP Secret. Generated new Secret manifest. configmap=cluster-ca generated_secret=secret-cluster-ca`
- `[SUCCESS] [PodMonitor:eval-p3/tls-ca-monitor] Converted successfully`

---

## 3. Reconciliation & Audit Details

| Resource (`<Kind>:<ns>/<name>`) | Source / Issue | Investigation Step | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitor:eval-p3/tls-ca-monitor` | Prometheus Operator `tlsConfig.ca.configMap` reference (`cluster-ca:ca.crt`) | Discovered backing `ConfigMap` `cluster-ca` in `eval-p3` in migration inputs (`workloads.yaml`). | Converted `ConfigMap` to companion Kubernetes `Secret` (`secret-cluster-ca`) containing CA cert, and mapped `tls.ca.secret` to `secret-cluster-ca:ca.crt` in `PodMonitoring`. |
| `PodMonitor:eval-p3/tls-ca-monitor` | Missing `spec.podMetricsEndpoints[0].interval` | Evaluated GMP CRD schema requirement for mandatory scrape interval. | Defaulted scrape interval to standard `30s` (non-breaking warning). |

---

## 4. Converted Manifests

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: secret-cluster-ca
  namespace: eval-p3
stringData:
  ca.crt: |
    -----BEGIN CERTIFICATE-----
    MIIB/TCCAWagAwIBAgIUeN7+dummycertdata...
    -----END CERTIFICATE-----
---
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  creationTimestamp: null
  name: tls-ca-monitor
  namespace: eval-p3
spec:
  endpoints:
  - interval: 30s
    port: metrics
    scheme: https
    tls:
      ca:
        secret:
          key: ca.crt
          name: secret-cluster-ca
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

---

## 5. Input vs Output Diff

```diff
-apiVersion: monitoring.coreos.com/v1
-kind: PodMonitor
+apiVersion: monitoring.googleapis.com/v1
+kind: PodMonitoring
 metadata:
   name: tls-ca-monitor
   namespace: eval-p3
 spec:
-  podMetricsEndpoints:
-  - port: metrics
-    scheme: https
-    tlsConfig:
-      ca:
-        configMap:
-          name: cluster-ca
-          key: ca.crt
+  endpoints:
+  - interval: 30s
+    port: metrics
+    scheme: https
+    tls:
+      ca:
+        secret:
+          name: secret-cluster-ca
+          key: ca.crt
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
```
*(Plus generated companion Secret `secret-cluster-ca` in namespace `eval-p3`)*
