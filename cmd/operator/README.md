# Operator

See the [package documentation](../../cmd/operator/README.md) for testing
instructions.

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
kubectl apply -k cmd/operator/deploy/crds/
kubectl apply -k cmd/operator/deploy/operator/
```

Finally, wait until the operator starts up. You will see a status of `Running`
for the gmp-operator pod:

```bash
kubectl get all -ngmp-system
```

## Run Locally

Deploy all CRDs. In this directory:

```bash
kubectl apply -k deploy/crds/
```

Deploy all of the required configurations besides the operator deployment
otherwise you will have an operator deployed in addition to your local one. The
easiest way to do this is by removing `deployment.yaml` in
[deploy/operator/kustomization.yaml](deploy/operator/kustomization.yaml).
Additionally, you may also consider removing the webhook configurations because
they apply to other configurations in the group. This avoids errors when
updating configurations and the operator is not present to process the webhook.

Run the operator locally (requires active kubectl context to have all
permissions the operator needs):

```bash
go run main.go
```

The operator updates the configuration of all collectors after which they start
scraping various metric endpoints.

Verify by port-forwarding an arbitrary collector and inspect its UI. You should
see various targets being scraped successfully.

```bash
kubectl -n gmp-system port-forward --address 0.0.0.0 collector 19090
```

Go to `http://localhost:19090/targets`.

## Teardown

Simply stop running the operator locally and remove all manifests in the cluster
with:

```bash
kubectl delete -f deploy/ --recursive
```
