## Charts

> IMPORTANT: This directory contains helm charts we maintain only for templating purposes. 
> They are not designed to be "imported" (used directly for the installation/upgrade/deploy); they
are not tested for this use and can change anytime.

### Usage

Those charts are used in [`hack/presubmit.sh manifests`](/hack/presubmit.sh). 

Each chart requires `values.global.yaml` in this directory and (optionally) its own
`values.yaml` file.

The global value file allows us to have a single source of truth file to variables
we use for our example manifests for `/manifests/operator.yaml`, standalone
`/manifests/rule-evaluator.yaml` and `/cmd/datasource-syncer/datasource-syncer.yaml`.

To render them, use `helm template` command with multiple `-f` flags (scripted in `make regen` command):

```bash
  ${HELM} template "${REPO_ROOT}/charts/rule-evaluator" \
   -f "${REPO_ROOT}/charts/rule-evaluator/values.yaml" \
   -f "${REPO_ROOT}/charts/values.global.yaml" \
    > "${REPO_ROOT}/manifests/rule-evaluator.yaml"
```
