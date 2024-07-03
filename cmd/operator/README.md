# Operator

This binary is a Kubernetes operator that provides Managed Collection for Google
Cloud Prometheus Engine on Kubernetes.

The operator can be run and tested two ways. The first and arguably better way,
in lieu of matching actual deployment, is by creating a docker image and pushing
it to Google Cloud and telling Kubernetes to fetch and run it. The second is by
pushing the configurations to Kubernetes running and instead of Kubernetes
running an image, you run the operator locally on your machine.

## Running through Google Cloud

As a pre-requisite, ensure that your Kubernetes cluster is setup and you are
connected to it via the `gcloud` CLI. The easiest way is by clicking the
`Connect` button which reveals a command after selecting your cluster in the
Google Cloud Console. Ensure that your service account is [configured to read
images](https://cloud.google.com/kubernetes-engine/docs/troubleshooting#permission_denied_error).
If you are using the default service account for your Kubernetes node pool, you
can find the email via `IAM & Admin > Service Accounts` and looking for the
account that is listed as "Compute Engine default service account".

First, build and push the operator image. In the root directory:

```bash
DOCKER_PUSH=1 make operator
```

Note, that you can configure the [Makefile](/Makefile) to use certain
environment variables, such as `PROJECT_ID` but these are configured
automatically if they are not set. The command will give you the uploaded image
URL and update all necessary configurations to use it.

Next, apply the Kubernetes configuration files, starting with the CRDs:

```bash
kubectl apply -f cmd/operator/deploy/crds/
kubectl apply -f cmd/operator/deploy/operator/
```

Finally, wait until the operator starts up. You will see a status of `Running`
for the gmp-operator pod:

```bash
kubectl get all -ngmp-system
```

## Run Locally

Deploy all CRDs. In this directory:

```bash
kubectl apply -f deploy/crds/
```

Deploy all of the operator required configurations besides the operator
deployment, otherwise you will have an operator deployed in addition to your
local one.

```bash
kubectl apply -f deploy/operator/00-namespace.yaml
kubectl apply -f deploy/operator/01-priority-class.yaml
kubectl apply -f deploy/operator/02-service-account.yaml
kubectl apply -f deploy/operator/03-role.yaml
kubectl apply -f deploy/operator/04-rolebinding.yaml
```

Run the operator locally (requires active kubectl context to have all
permissions the operator needs):

```bash
go run main.go
```

Finally because the webhooks are configured to the operator apply the rest of
the configurations in a separate terminal session:

```bash
kubectl apply -f deploy/operator/06-service.yaml
kubectl apply -f deploy/operator/07-operatorconfig.yaml
kubectl apply -f deploy/operator/08-validatingwebhookconfiguration.yaml
kubectl apply -f deploy/operator/09-mutatingwebhookconfiguration.yaml
kubectl apply -f deploy/operator/10-collector.yaml
kubectl apply -f deploy/operator/11-rule-evaluator.yaml
```

The operator updates the configuration of all collectors after which they start
scraping various metric endpoints.

Verify by port-forwarding an arbitrary collector and inspect its UI. You should
see various targets being scraped successfully.

```bash
kubectl -n gmp-system port-forward --address 0.0.0.0 collector 19090
```

Go to `http://localhost:19090/targets`.

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

Simply stop running the operator locally and remove all manifests in the cluster
with:

```bash
kubectl delete -f deploy/ --recursive
```
