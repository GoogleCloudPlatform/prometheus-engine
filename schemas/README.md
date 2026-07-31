# GMP CRD JSON Schemas

JSON schemas extracted from the operator CRDs for validating GMP custom resources with
[kubeconform](https://github.com/yannh/kubeconform) or similar tools.

Schemas follow the naming convention used by
[flux2-schemas](https://github.com/fluxcd-community/flux2-schemas):
`{singular}-{group}-{version}.json` (for example, `podmonitoring-monitoring-v1.json`).

## Kubeconform

Validate manifests against a pinned release of this repository:

```bash
kubeconform -kubernetes-version 1.32.0 \
  -schema-location default \
  -schema-location 'https://raw.githubusercontent.com/GoogleCloudPlatform/prometheus-engine/v0.17.3/schemas/{{ .ResourceKind }}-monitoring-{{ .ResourceAPIVersion }}.json' \
  -summary \
  examples/
```

For a local checkout, point `-schema-location` at this directory:

```bash
kubeconform -kubernetes-version 1.32.0 \
  -schema-location default \
  -schema-location 'schemas/{{ .ResourceKind }}-monitoring-{{ .ResourceAPIVersion }}.json' \
  -summary \
  examples/
```

## Regenerating

Schemas are generated from `charts/operator/crds` when running `./hack/presubmit.sh crdgen`
or `./hack/presubmit.sh all`.
