# Operator

This binary is the Kubernetes operator (`gmp-operator`) that reconciles Google Cloud Managed Service for Prometheus (GMP) Custom Resources into collector and rule-evaluator workloads.

The operator can be run and tested in two ways:
1. **In-cluster deployment**: Build and push a container image to a registry accessible by your Kubernetes cluster and apply the manifests in [`manifests/`](../../manifests/).
2. **Local out-of-cluster execution**: Apply the cluster prerequisites (`manifests/setup.yaml` and `manifests/operator.yaml`), scale down the in-cluster `gmp-operator` Deployment, and run `go run ./cmd/operator` locally against your cluster's `kubeconfig`.

## Running in a Kubernetes Cluster

Ensure that your Kubernetes cluster is set up and connected via `kubectl`, and that your node service account is [configured to pull images](https://cloud.google.com/kubernetes-engine/docs/troubleshooting#permission_denied_error) if using a private registry.

From the repository root, build and push the operator image:

```bash
DOCKER_PUSH=1 make operator
```

You can configure [`Makefile`](../../Makefile) environment variables such as `IMAGE_REGISTRY` and `TAG_NAME` to control the target image repository.

Next, apply the CRDs, RBAC, and operator manifests from the repository root:

```bash
kubectl apply -f manifests/setup.yaml
kubectl apply -f manifests/operator.yaml
```

Wait until the `gmp-operator` pod in `gmp-system` reaches `Running`:

```bash
kubectl get all -n gmp-system
```

## Running Locally (Out-of-Cluster)

To run the operator binary on your local workstation against a `kind` or GKE cluster (from the repository root):

1. Apply the CRDs, namespaces, and base operator resources:

```bash
kubectl apply -f manifests/setup.yaml
kubectl apply -f manifests/operator.yaml
```

2. Scale down the in-cluster `gmp-operator` Deployment so it does not compete with your local process, and remove the validating webhook configuration (unless you configure local webhook TLS certificates and network routing from the API server):

```bash
kubectl -n gmp-system scale deploy/gmp-operator --replicas=0
kubectl delete validatingwebhookconfiguration gmp-operator.gmp-system.monitoring.coreos.com --ignore-not-found
```

3. Run the operator locally from the repository root:

```bash
go run ./cmd/operator \
  --project-id=test-proj \
  --cluster=test-cluster \
  --location=us-central1-a \
  --kubeconfig="$HOME/.kube/config"
```

The operator reconciles `OperatorConfig/config` in `gmp-public` and updates the configuration of all `collector` pods in `gmp-system`.

4. Verify target discovery by port-forwarding to the `collector` DaemonSet:

```bash
kubectl -n gmp-system port-forward daemonset/collector 19090
```

Open `http://localhost:19090/targets` in your browser to inspect active scrape targets.

## Namespace & Secret RBAC Boundaries

* **`gmp-public` (`--public-namespace`)**: Holds the singleton `OperatorConfig` named `config` and any Secrets referenced directly by `OperatorConfig` (such as `collection.credentials`, `rules.credentials`, or Alertmanager auth Secrets). The operator mirrors those Secrets into `gmp-system`.
* **`gmp-system` (`--operator-namespace`)**: Holds operator-managed workloads (`collector` DaemonSet, `rule-evaluator` Deployment) and generated configuration Secrets/ConfigMaps (`collection`, `rules-generated`).
* **Secrets referenced by `PodMonitoring` / `ClusterPodMonitoring`**: Secrets used for scrape endpoint TLS, BasicAuth, Bearer tokens, or OAuth2 must reside in the same namespace as the `PodMonitoring` resource (or in `gmp-system` for cluster-scoped `ClusterPodMonitoring` / `ClusterNodeMonitoring`). Because the collector reads these Secrets via the Kubernetes API, you must grant `get` permission on `secrets` in that namespace to `ServiceAccount` `gmp-system:collector` via a `Role` and `RoleBinding`.

## Flags

```bash mdox-exec="bash hack/format_help.sh operator"
Usage of operator:
  -ca-cert-base64 string
    	The base64-encoded certificate authority.
  -cert-dir string
    	The directory which contains TLS certificates for webhook server. (default "/etc/tls/private")
  -cleanup-unless-annotation-key string
    	Clean up operator-managed workloads without the provided annotation key.
  -cluster string
    	Name of the cluster the operator acts on. May be left empty on GKE.
  -kubeconfig string
    	Paths to a kubeconfig. Only required if out-of-cluster.
  -location string
    	Google Cloud region or zone where your data will be stored. May be left empty on GKE.
  -metrics-addr string
    	Address to emit metrics on. (default ":18080")
  -operator-namespace string
    	Namespace in which the operator manages its resources. (default "gmp-system")
  -probe-addr string
    	Address to outputs probe statuses (e.g. /readyz and /healthz) (default ":18081")
  -project-id string
    	Project ID of the cluster. May be left empty on GKE.
  -public-namespace string
    	Namespace in which the operator reads user-provided resources. (default "gmp-public")
  -tls-cert-base64 string
    	The base64-encoded TLS certificate.
  -tls-key-base64 string
    	The base64-encoded TLS key.
  -v int
    	Logging verbosity
  -webhook-addr string
    	Address to listen to for incoming kube admission webhook connections. (default ":10250")
```

## Teardown

Stop the local operator process (if running) and remove the deployed manifests from the cluster:

```bash
kubectl delete -f manifests/operator.yaml --ignore-not-found
kubectl delete -f manifests/setup.yaml --ignore-not-found
```
