# GMP Migration Report: `hack/migration-eval/monitors/n1_ambiguous_ports.yaml`

## 1. Executive Summary

The Prometheus Operator `PodMonitor` defined in [`hack/migration-eval/monitors/n1_ambiguous_ports.yaml`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/hack/migration-eval/monitors/n1_ambiguous_ports.yaml) was processed and migrated to a Google Managed Prometheus (GMP) `PodMonitoring` custom resource using `gmp-migrate` and the [`gmp-migrate`](file:///usr/local/google/home/kunnikrishnan/Desktop/prometheus-engine/.agents/skills/gmp-migrate/SKILL.md) skill.

- **Source Manifest:** `PodMonitor:eval-n1/ambiguous-ports-monitor`
- **Initial Migration Status:** 1 Action Item (`TODO_SET_PORT` emitted due to missing endpoint port)
- **Target Workload:** `Deployment/multi-port-service` in namespace `eval-n1`
- **Resolution Status:** Successfully reconciled and verified via server-side dry run (`kubectl apply --dry-run=server`).

---

## 2. CLI Migration Execution

Running `gmp-migrate --all` against the input manifest produced the following initial draft manifest and diagnostics:

```
[INFO] [PodMonitor:eval-n1/ambiguous-ports-monitor] Successfully decoded PodMonitor
[WARNING] [PodMonitor:eval-n1/ambiguous-ports-monitor] Endpoint [0] does not specify a 'port' or 'targetPort'. action=Specify a valid port name or number in 'spec.endpoints[].port'.
[SUCCESS] [PodMonitor:eval-n1/ambiguous-ports-monitor] Converted successfully
```

### Initial Converted Draft Manifest (`gmp-migrate` Output)
```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  annotations:
    gmp.googleapis.com/todo-1: '[ERROR] Endpoint [0] does not specify a ''port'' or
      ''targetPort''. ACTION: Specify a valid port name or number in ''spec.endpoints[].port''.'
  creationTimestamp: null
  name: ambiguous-ports-monitor
  namespace: eval-n1
spec:
  endpoints:
  - interval: 30s
    port: TODO_SET_PORT
  selector:
    matchLabels:
      app.kubernetes.io/name: multi-port-service
  targetLabels:
    metadata: null
status:
  observedGeneration: 0
```

---

## 3. Investigation & Ambiguity Resolution (Recipe 1.3)

### Investigation Steps:
1. **Workload Discovery:** Queried pods matching selector `app.kubernetes.io/name: multi-port-service` in namespace `eval-n1`.
2. **Container Port Inspection:** 
   ```bash
   kubectl get pod -n eval-n1 -l app.kubernetes.io/name=multi-port-service \
     -o jsonpath='{.items[0].spec.containers[*].ports}'
   ```
   **Discovered Ports on container `api`:**
   - `int-metrics` (containerPort: `9090`) - Designated metrics port
   - `http-web` (containerPort: `8080`) - Web application traffic
   - `admin` (containerPort: `15090`) - Administrative interface

3. **Ambiguity Resolution:**
   Following Recipe 1.3, the ambiguous placeholder `TODO_SET_PORT` was resolved to the intended internal metrics endpoint `int-metrics` (or port `9090`).

---

## 4. Reconciliation Audit Table

| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring:eval-n1/ambiguous-ports-monitor` | `gmp.googleapis.com/todo-1` (`TODO_SET_PORT`) | Inspected pod container ports in namespace `eval-n1` matching `app.kubernetes.io/name: multi-port-service` | Replaced placeholder `TODO_SET_PORT` with named port `int-metrics` (port `9090`) and stripped the TODO annotation. |

---

## 5. Manifest Diffs

### Reconciliation Diff (Draft $\rightarrow$ Final Reconciled Manifest)
```diff
 apiVersion: monitoring.googleapis.com/v1
 kind: PodMonitoring
 metadata:
-  annotations:
-    gmp.googleapis.com/todo-1: '[ERROR] Endpoint [0] does not specify a ''port'' or
-      ''targetPort''. ACTION: Specify a valid port name or number in ''spec.endpoints[].port''.'
   creationTimestamp: null
   name: ambiguous-ports-monitor
   namespace: eval-n1
 spec:
   endpoints:
   - interval: 30s
-    port: TODO_SET_PORT
+    port: int-metrics
   selector:
     matchLabels:
       app.kubernetes.io/name: multi-port-service
   targetLabels:
     metadata: null
 status:
   observedGeneration: 0
```

---

## 6. Final Reconciled GMP Manifest

```yaml
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: ambiguous-ports-monitor
  namespace: eval-n1
spec:
  endpoints:
  - interval: 30s
    port: int-metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: multi-port-service
```

---

## 7. Dry-Run Verification

Server-side dry-run validation was executed against the Kubernetes cluster:
```bash
kubectl apply --dry-run=server -f - <<EOF
apiVersion: monitoring.googleapis.com/v1
kind: PodMonitoring
metadata:
  name: ambiguous-ports-monitor
  namespace: eval-n1
spec:
  endpoints:
  - interval: 30s
    port: int-metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: multi-port-service
EOF
```

**Result:**
```
podmonitoring.monitoring.googleapis.com/ambiguous-ports-monitor created (server dry run)
```
Status: **PASSED** (100% valid schema and admission pass).
