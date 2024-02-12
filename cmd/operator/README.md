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

Next, install the Kubernetes configuration files using Helm:

```bash
helm install gmp-operator ./charts/operator
```

Finally, wait until the operator starts up. You will see a status of `Running`
for the gmp-operator pod:

```bash
kubectl get all -ngmp-system
```

## Run Locally

Deploy all CRDs. From the root directory of the repo:

```bash
helm install ./charts/operator --set deployOperator=false
```

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
helm uninstall gmp-operator
```
