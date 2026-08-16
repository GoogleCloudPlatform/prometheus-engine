# GMP Migration Report: `n3_corrupt_secret.yaml` & `offline_corrupt_secret.yaml`

## 1. Migration Summary

| Metric | Count |
| :--- | :--- |
| **Input Manifests** | [`hack/migration-eval/monitors/n3_corrupt_secret.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/n3_corrupt_secret.yaml)<br>[`hack/migration-eval/monitors/offline_corrupt_secret.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/offline_corrupt_secret.yaml) |
| **Successfully Migrated (Clean)** | 0 |
| **Migrated with Warnings** | 0 |
| **Migrated with Action Items** | 1 (`PodMonitoring: eval-n3/corrupt-secret-monitor`) |
| **Skipped (Unsupported)** | 0 |
| **Failed (Fatal)** | 0 |
| **Exit Code** | 1 (Action items requiring review / reconciliation) |

---

## 2. CLI Execution & Diagnostics

### Command Invoked
```bash
gmp-migrate --all \
  -f hack/migration-eval/monitors/n3_corrupt_secret.yaml \
  -f hack/migration-eval/monitors/offline_corrupt_secret.yaml
```

### Stderr Diagnostic Logs
```text
[INFO] [PodMonitor:eval-n3/corrupt-secret-monitor] Successfully decoded PodMonitor
[WARNING] [PodMonitor:eval-n3/corrupt-secret-monitor] Scrape interval is empty. Defaulting to '30s' as GMP requires this field.
[WARNING] [PodMonitor:eval-n3/corrupt-secret-monitor] Failed to base64-decode key "username" in Secret "corrupt-auth". action=Ensure Secret "corrupt-auth" contains valid base64 data (or use stringData).
[SUCCESS] [PodMonitor:eval-n3/corrupt-secret-monitor] Converted successfully
```

---

## 3. Converted Manifest (Draft with Action Items)

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[ERROR] Failed to base64-decode key "username" in
      Secret "corrupt-auth". ACTION: Ensure Secret "corrupt-auth" contains valid base64
      data (or use stringData).'
  creationTimestamp: null
  name: corrupt-secret-monitor
  namespace: eval-n3
spec:
  endpoints:
  - basicAuth:
      username: TODO_CORRUPT_SECRET_DATA_USERNAME
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

---

## 4. Reconciliation Audit & Action Item Analysis

| Resource | TODO Key / Placeholder | Source / Cause | Recipe & Action Required |
| :--- | :--- | :--- | :--- |
| `PodMonitoring: eval-n3/corrupt-secret-monitor` | `gmp.googleapis.com/todo-1`<br>(`TODO_CORRUPT_SECRET_DATA_USERNAME`) | `Secret/eval-n3/corrupt-auth` key `username` contains invalid base64 data: `"%%%invalid-base64%%%"` | **Recipe 2.3 (Corrupt Base64 Data)**:<br>Key `username` in Secret `corrupt-auth` is corrupted. Do not fabricate credentials. The Secret's `data.username` must be repaired with valid base64-encoded credentials (or migrated to `stringData`), or the placeholder `TODO_CORRUPT_SECRET_DATA_USERNAME` replaced with the valid username string. |

---

## 5. Remediation Diff (Example)

Once the valid secret value is provided (e.g. `username: "metrics-user"`), the manifest reconciles as follows:

```diff
 metadata:
   name: corrupt-secret-monitor
   namespace: eval-n3
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Failed to base64-decode key "username" in Secret "corrupt-auth"...'
 spec:
   endpoints:
   - basicAuth:
-      username: TODO_CORRUPT_SECRET_DATA_USERNAME
+      username: metrics-user
     interval: 30s
     port: metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
```
