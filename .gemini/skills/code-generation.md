# Skill: Regenerate CRDs, Code, and Manifests

This skill details how to regenerate Go helper files, Kubernetes Custom Resource Definition (CRD) manifests, Helm charts, and API docs when API models or configurations change.

## Purpose
Use this skill whenever you modify struct files under `pkg/operator/apis/` or template charts under `charts/`.

## Execution Steps

### 1. Make Changes to API Types
Edit Go struct fields in directories under:
- `pkg/operator/apis/monitoring/v1/`

### 2. Regenerate Code & Manifests
Run the global regeneration task:
```bash
make regen
```
This performs the following actions:
- Format and tidy all `go.mod` files.
- Run `controller-gen` to generate CRD manifests.
- Combine CRDs into `manifests/setup.yaml`.
- Generate Helm charts template outputs.
- Update API documentation in `doc/api.md`.
- Ensure license headers exist on all code and YAML files.

### 3. Check for Diff / Compliance
To run strict validation that the generated code is clean and up to date:
```bash
make regen CHECK=1
```
Or to check if any untracked or uncommitted changes exist:
```bash
./hack/presubmit.sh diff
```

## Troubleshooting
- **Missing Tools**: If commands fail due to missing utilities, verify if you should run using the Docker build target (e.g. `make regen`) which uses containerized tools instead of local bin.
- **Merge-base Warning**: If `codegen_diff` warns that the branch is not descendant of main, run `git pull` or rebase your branch on `main`.
