# Migration Report: `hack/migration-eval/monitors/p1_named_port.yaml`

## 1. Executive Summary
- **Source Monitor**: `hack/migration-eval/monitors/p1_named_port.yaml` (`kind: ServiceMonitor`, `eval-p1/payment-monitor`)
- **Backing Resources**: `hack/migration-eval/workloads.yaml` (`kind: Service`, `eval-p1/payment-service`, and `kind: Deployment`, `eval-p1/payment-processor`)
- **Resulting Resource**: `kind: PodMonitoring` (`eval-p1/payment-monitor`)
- **Status**: **Clean Parity (Exit Code: 0)**
- **Action Items / TODOs**: 0
- **Warnings**: 0
- **Failures**: 0

---

## 2. CLI Execution & Diagnostics

### Command Executed:
```bash
gmp-migrate -f hack/migration-eval/monitors/p1_named_port.yaml -f hack/migration-eval/workloads.yaml
```

### CLI Summary Output:
```
=========================================
Migration Complete Summary:
  Successfully Migrated:      1
  Migrated with Warnings:     0
  Migrated with Action Items: 0
  Skipped (Unsupported):      22
  Failed:                     0
=========================================
```

---

## 3. Resolution Breakdown

| Resolution Component | Source `ServiceMonitor` / `Service` | Converted `PodMonitoring` | Resolution Method |
| :--- | :--- | :--- | :--- |
| **Kind Mapping** | `ServiceMonitor` | `PodMonitoring` | Standard GMP conversion |
| **Endpoint Port** | `port: metrics` (named string) | `port: 8080` (integer) | Mapped via backing `payment-service` port `metrics` (`port: 80`, `targetPort: 8080`) |
| **Pod Selector** | `matchLabels: { app: payment-processor }` | `matchLabels: { app.kubernetes.io/name: payment-processor }` | Resolved via `payment-service.spec.selector` pointing to target Pod labels |
| **Scrape Interval** | `interval: 15s` | `interval: 15s` | Direct conversion |

---

## 4. Manifest Comparison

### Source Manifest (`hack/migration-eval/monitors/p1_named_port.yaml`)
```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: payment-monitor
  namespace: eval-p1
spec:
  selector:
    matchLabels:
      app: payment-processor
  endpoints:
  - port: metrics
    interval: 15s
```

### Backing Service (`hack/migration-eval/workloads.yaml`)
```yaml
apiVersion: v1
kind: Service
metadata:
  name: payment-service
  namespace: eval-p1
  labels:
    app: payment-processor
spec:
  type: ClusterIP
  selector:
    app.kubernetes.io/name: payment-processor
  ports:
  - name: metrics
    port: 80
    targetPort: 8080
```

### Generated GMP Manifest (`PodMonitoring`)
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  creationTimestamp: null
  name: payment-monitor
  namespace: eval-p1
spec:
  endpoints:
  - interval: 15s
    port: 8080
  selector:
    matchLabels:
      app.kubernetes.io/name: payment-processor
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

---

## 5. Audit & Validation Result
- **Validation**: 100% ready for deployment with zero manual reconciliation required.
- **Selector Integrity**: Directly targets pod label `app.kubernetes.io/name: payment-processor` matching the workload deployment.
- **Port Integrity**: Correctly resolved `targetPort: 8080` matching container port `8080` (`name: metrics`).
