# Skill: Linting, Formatting, and Conformity

This skill describes how to run style checkers, code formatters, linter tools, and commit/conformity rules to ensure clean contributions.

## Purpose
Use this skill to verify and polish code formatting, linting rules, and licenses before submitting changes.

## Execution Options

### Option 1: Reformat all Files
Runs automatic code/script/doc formatting:
```bash
./hack/presubmit.sh format
```
This will:
- Tidy all `go.mod` files using `go mod tidy`.
- Format all Go files using `go fmt ./...`.
- Format all Markdown files using `mdox fmt`.
- Format bash scripts (`ops/gmpctl/lib.sh`, `hack/presubmit.sh`) using `shfmt`.

### Option 2: Run golangci-lint
Verifies codebase conventions and checks for code issues:
```bash
make lint
```
* **Notes**: Uses tool dependencies declared in `tools/go.mod`.

### Option 3: Run Conform check
Enforces standard Git commit structures and repository rules:
```bash
make conform
```

## Troubleshooting
- **Linter Timeout**: If the linter times out, verify if docker resources are congested, or run with local tools path if possible.
- **conform Errors**: `conform` enforces commit messages and branches. Check `.conform.yaml` in the root of the repository for rules.
