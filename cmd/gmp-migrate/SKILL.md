---
name: gmp-migrate
description: >-
  Run the gmp-migrate CLI tool to convert Prometheus Operator manifests (PodMonitor,
  ServiceMonitor) to Google Managed Prometheus (GMP) CRDs (PodMonitoring,
  ClusterPodMonitoring), and reconcile generated TODOs, port placeholders, and errors.
---

# GMP Migration & Reconciliation Skill

This skill guides agents through executing the migration CLI tool, interpreting migration reports, diagnosing `Stderr` logs, resolving safety guardrails and `gmp.googleapis.com/todo-*` annotations, and safely validating converted GMP manifests.

---

## 1. Tool Execution Interface

> [!NOTE]
> All commands in this skill invoke the `gmp-migrate` binary. If the underlying CLI binary name or command syntax changes in the future, only this invocation section needs to be updated.

### Core CLI Invocations

1. **Default Mode (Ready-only)**: Emits strictly 100% ready manifests to `Stdout`. Omits any best-effort drafts requiring human/agent review.
   ```bash
   gmp-migrate -f <input-path> > ready_manifests.yaml 2> migration.log
   ```

2. **Full Review Mode (`--all` / `-a`)**: Emits all converted manifests to `Stdout`, including best-effort draft configurations with inline `gmp.googleapis.com/todo-*` annotations for human / agent reconciliation.
   ```bash
   gmp-migrate --all -f <input-path> > all_manifests.yaml 2> migration.log
   ```

3. **Multiple Input Sources (Monitors + Backing Services/Secrets)**:
   ```bash
   gmp-migrate --all -f path/to/monitors/ -f path/to/services/ > all_manifests.yaml 2> migration.log
   ```

4. **Live Cluster Ingestion via Stdin**:
   ```bash
   kubectl get podmonitors,servicemonitors,services,configmaps,secrets -A -o yaml | gmp-migrate --all -f - > all_manifests.yaml 2> migration.log
   ```

---

## 2. Input Discovery Strategy

1. **Local Files**:
   - Check the workspace for Prometheus Operator YAML manifests (`kind: PodMonitor` or `kind: ServiceMonitor`).
   - When migrating local files, also include directories containing backing `kind: Service`, `kind: Secret`, and `kind: ConfigMap` resources so the tool can perform cross-resource port and auth resolution.
2. **Live Cluster Context**:
   - If no local manifests exist, or if the user asks to migrate an active cluster, stream cluster resources directly via `kubectl ... | gmp-migrate --all -f -`.
3. **Ambiguous Source**:
   - If both local files and an active `kubectl` context exist, ask the user whether they want to migrate local manifests or pull live resources from the cluster.

---

## 3. Exit Code, Stderr Logs & Diagnostic Handling

The CLI strictly separates streams: converted YAML manifests are written to `Stdout`, while diagnostics and summary tables are written to `Stderr`. Workload selectors (`spec.selector.matchLabels` and `spec.selector.matchExpressions`) remain **100% pure and untouched**.

```
=========================================
Migration Complete Summary:
  Successfully Migrated:      X
  Migrated with Warnings:     Y
  Migrated with Action Items: Z
  Skipped (Unsupported):      W
  Failed:                     V
=========================================
```

| Exit Code / Report | Status | Stream Behavior & Recommended Agent Action |
| :--- | :--- | :--- |
| **Exit Code 0** (`ActionItemsCount == 0`, `FailedCount == 0`) | Clean Parity | All manifests are 100% ready. Written to `Stdout`. Proceed directly to dry-run verification and diff presentation. |
| **Exit Code 1** (`ActionItemsCount > 0`, `FailedCount == 0`) | Action Items Present | • Default mode: emits only ready manifests to `Stdout` and notes omitted drafts on `Stderr`.<br>• `--all` mode: emits all manifests (including drafts with inline `gmp.googleapis.com/todo-*` annotations) to `Stdout`.<br>Follow Section 5 to reconcile drafts. |
| **Exit Code 1** (`FailedCount > 0`) | Fatal Conversion Failure | Zero manifests were written to `Stdout` (0 bytes). Inspect `Stderr` logs to identify missing dependencies or malformed YAML inputs, resolve them, and re-run. |

### Inspecting CLI Diagnostics in `Stderr` Logs:
When investigating why a TODO or warning was added, inspect the `Stderr` log (or `migration.log`) for pre-scoped resource logs:
```bash
# Filter logs for a specific resource
grep "PodMonitor:default/my-app" migration.log

# Filter all warnings and fatal errors
grep -E "\[WARNING\]|\[ERROR\]" migration.log
```

---

## 4. Secret Creation & Namespacing Rules

* **`PodMonitoring` (Namespaced CRD)**: Kubernetes forbids cross-namespace Secret references. If a Secret or converted ConfigMap is referenced, **it must exist in the same namespace as the `PodMonitoring`**. `gmp-migrate` automatically generates a companion `Secret` clone in every target namespace where the `PodMonitoring` is created.
* **`ClusterPodMonitoring` (Cluster-Scoped CRD)**: Supports an explicit `namespace` field in `SecretKeySelector`. Companion Secrets live in a central namespace (e.g. `gmp-system`), and `secret.namespace` is explicitly populated.

---

## 5. Action Plan Catalog for TODO Reconciliation

> [!CAUTION]
> **Strict Read-Only Investigation Policy**:
> All `kubectl` commands executed during investigation (Recipes 1–6) MUST be strictly READ-ONLY (`get`, `describe`, `logs`, `--dry-run=client`).
> Under NO circumstances should an agent run state-changing commands (`kubectl apply`, `kubectl create`, `kubectl patch`, `kubectl delete`) against a live cluster without reaching Section 7 and receiving explicit user approval.

All TODO annotations follow the format: `gmp.googleapis.com/todo-N: "[WARNING|ERROR] <reason> ACTION: <action>"`. Match the placeholder or annotation message against the recipes below:

---

### Recipe 1: Port & Pod Selector Resolution

#### 1.1 Unresolved Named Port (`TODO_RESOLVE_PORT_<NAME>`)
* **Annotation**: `[WARNING] Named port "<name>" was not found on Service "<svc>"... ACTION: Replace TODO_RESOLVE_PORT_<NAME> with the target container port name or number.`
* **Commands**:
  ```bash
  kubectl get svc <service-name> -n <namespace> -o jsonpath='{.spec.ports}' | jq .
  kubectl get pod -n <namespace> -l <selector> -o jsonpath='{.items[0].spec.containers[*].ports}' | jq .
  ```
* **Resolution**:
  1. If Service `targetPort` is an integer (e.g. `8080`), replace placeholder with integer `8080`.
  2. If Service `targetPort` is a named string (e.g. `http-metrics`), set `port: http-metrics`.
  3. If Service `targetPort` is omitted, replace with the Service's integer `port`.
* **Diff**:
  ```diff
   spec:
     endpoints:
  -  - port: TODO_RESOLVE_PORT_METRICS
  +  - port: 8080
       interval: 30s
  ```

#### 1.2 Missing Backing Service (`TODO_RESOLVE_PORT` & `TODO_SET_POD_LABELS`)
* **Context**: `gmp-migrate` emits `TODO_SET_POD_LABELS: TODO_SET_POD_LABELS` as a safe guardrail placeholder. Replace this placeholder with the complete set of pod template labels for the target workload.
* **Annotation**: `[ERROR] Corresponding Kubernetes Service was not found... ACTION: Define target pod selector in 'spec.selector.matchLabels' and verify endpoint ports.`
* **Commands**:
  ```bash
  kubectl get svc -n <namespace> -o wide
  kubectl get deployment,statefulset,daemonset -n <namespace> -o wide
  ```
* **Agent Interactive Step**:
  - If a matching Service or workload is identified, copy its pod label selector into `spec.selector.matchLabels` and resolve endpoint ports.
  - If multiple ambiguous workloads match, prompt the user:
    > *"Could not find a backing Service for `<name>`. Discovered candidate workloads: `[app=payment-processor, app=payment-worker]`. Please confirm the target pod labels."*
* **Diff**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Corresponding Kubernetes Service was not found..."
   spec:
     selector:
       matchLabels:
  -      TODO_SET_POD_LABELS: TODO_SET_POD_LABELS
  +      app.kubernetes.io/name: payment-processor
  +      app.kubernetes.io/component: backend
     endpoints:
  -  - port: TODO_RESOLVE_PORT
  +  - port: 9102
  ```

#### 1.3 Missing Endpoint Port (`TODO_SET_PORT`)
* **Annotation**: `[ERROR] Endpoint [N] does not specify a 'port' or 'targetPort'...`
* **Commands**:
  ```bash
  kubectl get pod -n <namespace> -l <selector> -o jsonpath='{.items[0].spec.containers[*].ports}' | jq .
  ```
* **Agent Interactive Step**:
  - If a single container metrics port is found, resolve to that port.
  - If the pod has multiple containers or multiple ports (e.g. `8080` vs `15090`), **do not guess**—prompt the user to choose the intended metrics port.
* **Diff**:
  ```diff
   spec:
     endpoints:
  -  - port: TODO_SET_PORT
  +  - port: metrics
  ```

---

### Recipe 2: Missing or Incomplete Secrets & ConfigMaps

#### 2.1 Missing Source Secret / ConfigMap (`TODO_SET_<KEY>_FROM_<KIND>_<NAME>`)
* **Annotation**: `[ERROR] Referenced SECRET "<name>" for key "<key>" was not found in migration inputs...`
* **Commands**:
  ```bash
  # Fetch Secret data from cluster
  kubectl get secret <secret-name> -n <namespace> -o jsonpath='{.data.<key>}' | base64 --decode
  
  # Or fetch ConfigMap data
  kubectl get configmap <cm-name> -n <namespace> -o jsonpath='{.data.<key>}'
  ```
* **Diff**:
  ```diff
   spec:
     endpoints:
     - port: 8080
       basicAuth:
  -      username: TODO_SET_USER_FROM_SECRET_APP-AUTH
  +      username: prometheus-scraper
  ```

#### 2.2 Missing Key in Secret/ConfigMap (`TODO_MISSING_KEY_<KEY>_IN_<KIND>_<NAME>`)
* **Annotation**: `[ERROR] Key "<key>" was not found in referenced SECRET "<name>"...`
* **Commands**:
  ```bash
  # Inspect available keys in Secret/ConfigMap
  kubectl get secret <secret-name> -n <namespace> -o jsonpath='{.data}' | jq 'keys'
  ```
* **Resolution**: If the key was misspelled in the source monitor (e.g. `user` instead of `username`), extract value from the correct key.
* **Diff**:
  ```diff
   spec:
     endpoints:
     - port: 8080
       basicAuth:
  -      username: TODO_MISSING_KEY_USERNAME_IN_SECRET_APP-AUTH
  +      username: admin
  ```

#### 2.3 Corrupt Base64 Data (`TODO_CORRUPT_SECRET_DATA_<KEY>`)
* **Annotation**: `[ERROR] Failed to base64-decode key "<key>" in Secret "<name>"...`
* **Agent Interactive Step**:
  Do NOT fabricate key values or placeholders. Prompt the user:
  > *"Key `<key>` in Secret `<name>` contains invalid base64 data. Please provide the correct value or fix the Secret in the cluster."*
* **Diff**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Failed to base64-decode key \"user\" in Secret \"app-auth\"..."
   spec:
     endpoints:
     - port: 8080
       basicAuth:
  -      username: TODO_CORRUPT_SECRET_DATA_USER
  +      username: valid-user
  ```

---

### Recipe 3: Converted ConfigMap TLS & Auth References

#### 3.1 Companion Secret Manifest Generation (`secret-<configmap-name>`)
* **Cause**: Prometheus Operator allowed TLS CAs in `ConfigMap`; GMP strictly requires Kubernetes `Secret` objects.
* **Manifest Generation Commands**:
  ```bash
  # Check if companion Secret was generated in output.yaml
  grep -A 10 "name: secret-<configmap-name>" output.yaml
  
  # If missing, generate companion Secret manifest locally (do NOT apply directly to cluster)
  kubectl create secret generic secret-<configmap-name> -n <namespace> \
    --from-file=ca.crt=/path/to/ca.crt --dry-run=client -o yaml >> output.yaml
  ```
* **Companion Secret Manifest**:
  ```yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: secret-cluster-ca
    namespace: prod
  stringData:
    ca.crt: "-----BEGIN CERTIFICATE-----\n..."
  ```

#### 3.2 Empty Secret Reference Selectors (`TODO_SET_SECRET_NAME`, `TODO_SET_SECRET_KEY`)
* **Annotation**: `[ERROR] Referenced Secret has an empty name for key...`
* **Commands**:
  ```bash
  kubectl get secrets -n <namespace>
  ```
* **Diff**:
  ```diff
   spec:
     endpoints:
     - port: https
       tls:
         ca:
           secret:
  -          name: TODO_SET_SECRET_NAME
  -          key: TODO_SET_SECRET_KEY
  +          name: my-tls-ca
  +          key: ca.crt
  ```

---

### Recipe 4: OAuth2 & Proxy URL Configurations (Interactive User Prompting)

#### 4.1 OAuth2 Placeholders (`TODO_SET_OAUTH2_CLIENT_ID`, `TODO_SET_OAUTH2_TOKEN_URL`)
* **Annotation**: `[ERROR] OAuth2 clientID must be defined as either Secret or ConfigMap...`
* **Agent Interactive Step**:
  Do **NOT** guess public endpoints via curl. Inspect local Secrets in the namespace for OAuth credentials, and prompt the user:
  > *"OAuth2 configuration requires a `clientID` and `tokenUrl` (e.g. Google Cloud OAuth: `https://oauth2.googleapis.com/token`, Keycloak, Okta). Please provide the token endpoint URL and client ID for this workload."*
* **Diff**:
  ```diff
   spec:
     endpoints:
     - port: 8080
       oauth2:
  -      clientID: TODO_SET_OAUTH2_CLIENT_ID
  -      tokenUrl: TODO_SET_OAUTH2_TOKEN_URL
  +      clientID: "123456789.apps.googleusercontent.com"
  +      tokenUrl: "https://oauth2.googleapis.com/token"
         clientSecret:
           secret:
             name: oauth-secret
             key: client-secret
  ```

#### 4.2 Proxy URL Configuration (Malformed URLs & Stripped Credentials)
* **Context**: GMP CRDs only support an unauthenticated `proxyUrl` string. GMP does not support proxy credentials (there are no Secret fields for proxies, and embedded credentials like `user:pass@` in `proxyUrl` are rejected by CRD admission rules).
* **Annotations**:
  * `[ERROR] Proxy URL "<url>" is invalid or malformed. ACTION: Specify a valid proxy URL (e.g. 'http://proxy.example.com:8080').`
  * `[ERROR] Proxy URL contains embedded plaintext credentials. Credentials were removed. ACTION: Configure proxy authentication via proxy server configuration or network allowlist.`
* **Agent Interactive Step**:
  1. **Malformed URL (`TODO_SET_VALID_PROXY_URL`)**: Prompt the user for the valid corporate proxy host and port.
  2. **Stripped Credentials**: `gmp-migrate` automatically strips `user:password@` so the manifest passes GMP validation while preserving the proxy host and port. Prompt the user:
     > *"Proxy URL credentials were removed because GMP CRDs do not support proxy authentication. Please ensure proxy authentication is handled at the network level (e.g. IP allowlisting the GKE cluster at the proxy server or routing through an in-cluster forward proxy). If the target is inside the cluster/VPC, `proxyUrl` can be removed entirely."*
  3. Once confirmed, remove the TODO annotation from `metadata.annotations`.
* **Diff (Case A: Resolving Malformed URL Placeholder)**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Proxy URL is invalid or malformed..."
   spec:
     endpoints:
     - port: 8080
  -    proxyUrl: TODO_SET_VALID_PROXY_URL
  +    proxyUrl: http://proxy.corp.internal:3128
  ```
* **Diff (Case B: Reconciling Stripped Credentials Annotation)**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Proxy URL contains embedded plaintext credentials. Credentials were removed..."
   spec:
     endpoints:
     - port: 8080
       proxyUrl: http://proxy.corp.internal:3128
  ```

---

### Recipe 5: Relabeling Rules & Scope Expansion (Interactive User Guidance)

#### 5.1 Dropped Pre-Scrape Keep/Drop Rules (Scope Expansion Warning)
* **Annotation**: `[WARNING] Dropped target filtering rule ('keep'|'drop' on '__meta_kubernetes_pod_annotation_...'). ACTION: Add equivalent pod label selector in 'spec.selector.matchLabels'.`
* **Agent Interactive Step**:
  GMP `spec.selector` **only matches Kubernetes Pod labels**, never Pod annotations. In Prometheus Operator, scraped pods are the intersection of the monitor's `spec.selector` and the annotation rule:
  $$\text{Scraped Targets} = (\text{Pods matching } \texttt{spec.selector}) \cap (\text{Pods matching Annotation Rule})$$

  ##### Case A: Reconciling `action: keep` (e.g. `prometheus.io/scrape: "true"`)
  1. **Intersection Query**: Query for workloads matching **both** the monitor's original `spec.selector` AND the annotation:
     ```bash
     kubectl get deployment,statefulset,daemonset -n <namespace> -l <original-monitor-selector> \
       -o jsonpath='{range .items[?(@.spec.template.metadata.annotations["prometheus.io/scrape"]=="true")]}{.kind}{"/"}{.metadata.name}{"\n"}{end}'
     ```
  2. **Update `PodMonitoring`**: Combine the original selector with the promoted label:
     ```yaml
     spec:
       selector:
         matchLabels:
           app.kubernetes.io/name: payment-gateway
           prometheus.io/scrape: "true"
     ```
  3. **Generate Companion Workload Patches**: Add `labels: { prometheus.io/scrape: "true" }` to each matching workload discovered in step 1.
  4. **Warn & Prompt**: Warn the user that patching the workload pod templates will trigger a rolling restart of their pods, and request approval before applying.

  ##### Case B: Reconciling `action: drop` (e.g. `prometheus.io/scrape: "false"`)
  1. **Identify Excluded Workloads**: Query for workloads matching `spec.selector` that have the exclusion annotation:
     ```bash
     kubectl get deployment,statefulset,daemonset -n <namespace> -l <original-monitor-selector> \
       -o jsonpath='{range .items[?(@.spec.template.metadata.annotations["prometheus.io/scrape"]=="false")]}{.kind}{"/"}{.metadata.name}{"\n"}{end}'
     ```
  2. **Inverted Selector via `matchExpressions: NotIn`**:
     - Add `labels: { prometheus.io/scrape: "false" }` to the excluded workload(s).
     - In `PodMonitoring.spec.selector`, use `matchExpressions` with `operator: NotIn`:
       ```yaml
       spec:
         selector:
           matchLabels:
             app.kubernetes.io/name: payment-gateway
           matchExpressions:
           - key: prometheus.io/scrape
             operator: NotIn
             values:
             - "false"
       ```
     > **Note**: Kubernetes `NotIn` matches all pods where the label is absent, or present with any value other than `"false"`.

* **Diffs**:
  ```diff
   # PodMonitoring CRD (Case A: Keep Rule)
   spec:
     selector:
       matchLabels:
         app.kubernetes.io/name: payment-gateway
  +      prometheus.io/scrape: "true"
  ```
  ```diff
   # PodMonitoring CRD (Case B: Drop Rule via NotIn)
   spec:
     selector:
       matchLabels:
         app.kubernetes.io/name: payment-gateway
  +    matchExpressions:
  +    - key: prometheus.io/scrape
  +      operator: NotIn
  +      values:
  +      - "false"
  ```
  ```diff
   # Target Workload spec.template.metadata.labels (Companion Patch)
   spec:
     template:
       metadata:
         labels:
           app.kubernetes.io/name: payment-gateway
  +        prometheus.io/scrape: "true"
  ```

#### 5.2 Conflicting Keep Relabelings
* **Annotation**: `[ERROR] Conflicting relabeling keep rules for label "<label>": cannot require both "<val1>" and "<val2>" simultaneously...` (or `[ERROR] Selector conflict: label "<label>" has conflicting values...`)
* **Agent Interactive Step**:
  Check the annotation or `Stderr` logs for conflicting values and ask the user to clarify:
  > *"Detected conflicting label rules for label `env` (`production` vs `staging`). Which workload should this PodMonitoring target?"*
* **Diff (Case A: Update to Target Staging Workload)**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Conflicting relabeling keep rules for label \"env\": cannot require both \"production\" and \"staging\" simultaneously..."
   spec:
     selector:
       matchLabels:
  -      env: production
  +      env: staging
  ```
* **Diff (Case B: Retain Production Workload)**:
  ```diff
   metadata:
  -  annotations:
  -    gmp.googleapis.com/todo-1: "[ERROR] Conflicting relabeling keep rules for label \"env\": cannot require both \"production\" and \"staging\" simultaneously..."
   spec:
     selector:
       matchLabels:
         env: production
  ```

---

### Recipe 6: Scrape Durations, Timeouts & Empty Selectors

#### 6.1 Invalid Duration / Timeout Capping
* **Annotation**: `[ERROR] Invalid scrape interval "<val>"...`
* **Diff**:
  ```diff
   spec:
     endpoints:
     - port: metrics
  -    interval: "30"
  -    timeout: "45s"
  +    interval: 30s
  +    timeout: 30s
  ```

#### 6.2 Empty Selector Warning (Interactive User Guidance)
* **Annotation**: `[WARNING] Resulting PodMonitoring selector is empty and matches all pods in this namespace.`
* **Context**: In GMP, an empty label selector (`matchLabels: {}` or empty `selector`) acts as a wildcard and scrapes every pod in the namespace (or cluster) that exposes the endpoint port.
* **Agent Interactive Step**:
  1. Inspect running workloads in the namespace to identify candidate workloads and their pod template labels:
     ```bash
     kubectl get deployment,daemonset,statefulset -n <namespace> -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.template.metadata.labels}{"\n"}{end}'
     ```
  2. Correlate the monitor's name and port with the discovered workloads.
  3. Prompt the user for confirmation:
     > *"PodMonitoring `<name>` has an empty selector and will scrape all pods in namespace `<namespace>`. Discovered workloads: `[app=redis, app=web]`. Please confirm if you want to target a specific workload label, or keep the wildcard selector to scrape all pods in the namespace."*
  4. If a specific workload is confirmed, apply `matchLabels`. If namespace-wide scraping was intentional, delete the TODO annotation and leave the selector as-is.
* **Diff (Target Specific Workload)**:
  ```diff
   spec:
     selector:
  -    matchLabels: {}
  +    matchLabels:
  +      app.kubernetes.io/name: redis
  ```

---

## 6. Action Item Resolution & TODO Cleanup Protocol

> [!IMPORTANT]
> Generated manifests maintain 100% pure workload selectors. Safety is preserved through stream gating and exit codes.

**Strict Protocol for Finalizing Reconciled Manifests:**
1. Verify that all `TODO_*` placeholders have been replaced with valid ports/names.
2. Verify all companion `kind: Secret` manifests are included in the bundle.
3. Validate scrape intervals, timeouts, and pod selectors against running workloads.
4. **Document Reconciliation Steps**: For every TODO item or placeholder being resolved, maintain a traceable record of:
   - **Original TODO / Placeholder**: The specific annotation key (`gmp.googleapis.com/todo-N`) or placeholder string (e.g. `TODO_RESOLVE_PORT_METRICS`).
   - **Investigation Conducted**: Exact `kubectl` commands, file lookups, or user clarifications used to gather the required context.
   - **Reconciliation Action**: The exact change applied to the manifest (e.g. mapped port `8080`, converted annotation to label selector).
5. Delete all `gmp.googleapis.com/todo-*` annotations from `metadata.annotations` once resolved.

### Reconciliation Audit & Code Diff Reporting Format
When presenting reconciled manifests to the user, always provide:
1. A structured audit table summarizing how each TODO was resolved.
2. Complete, per-file code diffs showing the exact before-and-after modifications for every affected file (including companion Secrets or patched workload Deployments).

#### Example Audit Table:
| Resource (`<Kind>:<ns>/<name>`) | TODO Key / Placeholder | Investigation Step / Source | Resolution Action Taken |
| :--- | :--- | :--- | :--- |
| `PodMonitoring: payments/payment-gateway` | `gmp.googleapis.com/todo-1` (`TODO_RESOLVE_PORT_METRICS`) | Queried `svc/payment-service` via `kubectl get svc` | Mapped Service port `metrics` to container port `8443`. |
| `PodMonitoring: payments/payment-gateway` | `gmp.googleapis.com/todo-2` (Scope Expansion) | Inspected pod labels with `kubectl get pods --show-labels` | Added `prometheus.io/scrape: "true"` to `matchLabels` and created Deployment patch. |
| `PodMonitoring: payments/payment-gateway` | `TODO_SET_VALID_PROXY_URL` | User prompted for corporate proxy endpoint | Stripped embedded basic auth and configured `http://proxy.corp.internal:3128`. |

#### Example Per-File Code Diffs:
```diff
# File: manifests/gmp/podmonitoring-payment-gateway.yaml
 metadata:
   name: payment-gateway-monitor
   namespace: payments
-  annotations:
-    gmp.googleapis.com/todo-1: "[WARNING] Dropped target filtering rule ('keep' on '__meta_kubernetes_pod_annotation_prometheus_io_scrape')..."
-    gmp.googleapis.com/todo-2: "[ERROR] Named port 'metrics' was not found on Service 'payment-service'..."
 spec:
   selector:
     matchExpressions:
     - key: component
       operator: In
       values:
       - checkout-api
       - webhook-handler
     matchLabels:
       app.kubernetes.io/name: payment-gateway
+      prometheus.io/scrape: "true"
   endpoints:
-  - port: TODO_RESOLVE_PORT_METRICS
+  - port: 8443
     interval: 15s
```

```diff
# File: manifests/apps/deployment-payment-gateway.yaml (Companion Workload Patch)
 spec:
   template:
     metadata:
       labels:
         app.kubernetes.io/name: payment-gateway
+        prometheus.io/scrape: "true"
```

---

## 7. Verification & User Agency

Before applying changes to a live cluster:
1. **Present Reconciliation Steps & Per-File Code Diffs**: Output the structured Reconciliation Audit Table detailing the steps taken for each TODO, followed by complete per-file diffs showing the exact before-and-after modifications for all reconciled manifests and companion files.
2. **Server-Side Dry Run**:
   ```bash
   kubectl apply --dry-run=server -f <output-manifests.yaml>
   ```
3. **Explicit User Approval**: Ask the user before running `kubectl apply`.
4. **Post-Apply Verification**:
   ```bash
   kubectl get podmonitoring,clusterpodmonitoring -A
   kubectl describe podmonitoring <name> -n <namespace>
   ```
   Check that `status.conditions` reports `ConfigurationCreateSuccess`.
