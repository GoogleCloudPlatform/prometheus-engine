# Operator

This binary is a Kubernetes operator that provides Managed Collection for Google
Cloud Prometheus Engine on Kubernetes.

## Run

Deploy all CRDs:

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

## Teardown

Simply stop running the operator locally and remove all manifests in the cluster with:

```bash
kubectl delete -f deploy/ --recursive
```
