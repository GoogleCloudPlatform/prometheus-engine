# Migration Report: `hack/migration-eval/monitors/p4_oauth2.yaml`

The Prometheus Operator manifest [p4_oauth2.yaml](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/p4_oauth2.yaml) has been migrated to Google Managed Prometheus (GMP) using the `gmp-migrate` CLI.

## Summary of Results

* **Input Resource**: `PodMonitor:eval-p4/oauth2-monitor`
* **Output Resource**: `PodMonitoring:eval-p4/oauth2-monitor`
* **Migration Status**: **Migrated with Action Items (1)** (Exit code: 1)
* **Detailed Artifact**: [p4_oauth2_migration_report.md](file:///usr/local/google/home/kunnikrishnan/.gemini/jetski/brain/76568533-956c-478f-85ca-3da748277e12/p4_oauth2_migration_report.md)

### Migration Statistics
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

## Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Source / Reason | Resolution Action Required |
| :--- | :--- | :--- | :--- |
| `PodMonitoring:eval-p4/oauth2-monitor` | `gmp.googleapis.com/todo-1`<br>`TODO_SET_CLIENT-ID_FROM_SECRET_OAUTH-CREDS` | Referenced Secret `oauth-creds` (key: `client-id`) was not found in migration inputs. | Provide `oauth-creds` Secret in inputs, or set `spec.endpoints[0].oauth2.clientID` directly. |
| `PodMonitoring:eval-p4/oauth2-monitor` | `gmp.googleapis.com/todo-2`<br>`TODO_SET_OAUTH2_TOKEN_URL` | Source `spec.podMetricsEndpoints[0].oauth2.tokenUrl` was empty (`""`). | Specify the OAuth2 token endpoint URL in `spec.endpoints[0].oauth2.tokenURL`. |
| `PodMonitoring:eval-p4/oauth2-monitor` | Scrape Interval Defaulting | Omitted in source manifest. | `gmp-migrate` automatically defaulted `interval` to `30s` (required by GMP schema). |

---

## Converted Draft Manifest (`gmp-migrate --all`)

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[ERROR] Referenced SECRET "oauth-creds" for key "client-id"
      was not found in migration inputs. ACTION: Verify SECRET "oauth-creds" exists
      in namespace "eval-p4" or provide it in migration inputs.'
    gmp.googleapis.com/todo-2: '[ERROR] OAuth2 tokenURL is empty. ACTION: Specify
      a valid token endpoint URL in ''spec.endpoints[].oauth2.tokenUrl''.'
  name: oauth2-monitor
  namespace: eval-p4
spec:
  endpoints:
  - interval: 30s
    oauth2:
      clientID: TODO_SET_CLIENT-ID_FROM_SECRET_OAUTH-CREDS
      tokenURL: TODO_SET_OAUTH2_TOKEN_URL
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
```

---

## Reconciled Manifest Diff (Sample)

```diff
 metadata:
   name: oauth2-monitor
   namespace: eval-p4
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Referenced SECRET "oauth-creds" for key "client-id"...'
-    gmp.googleapis.com/todo-2: '[ERROR] OAuth2 tokenURL is empty...'
 spec:
   selector:
     matchLabels:
       app.kubernetes.io/name: payment-processor
   endpoints:
   - port: metrics
     interval: 30s
     oauth2:
-      clientID: TODO_SET_CLIENT-ID_FROM_SECRET_OAUTH-CREDS
-      tokenURL: TODO_SET_OAUTH2_TOKEN_URL
+      clientID: "payment-service-client-id"
+      tokenURL: "https://auth.corp.internal/oauth/v2/token"
```
