# Agent Contribution Guidelines (`prometheus-engine`)

This document defines the repository rules, commit policies, coding conventions, and presubmit workflows for AI coding agents contributing to `GoogleCloudPlatform/prometheus-engine`.

---

## 1. Commit Message & PR Hygiene (`conform`)

Every commit in a pull request is strictly validated in CI by [Sidero Labs Conform](https://github.com/siderolabs/conform) via [`.conform.yaml`](./.conform.yaml). **Every single commit on the branch** (not just the PR title) must pass `make conform`.

### Commit Header Format

```text
<type>(<optional-scope>): <description>
```

### Strict `conform` Rules
1. **Allowed `<type>` values**:
   `feat`, `fix`, `build`, `chore`, `ci`, `docs`, `perf`, `refactor`, `revert`, `style`, `test`
2. **Allowed `<scope>` values** (if a scope is provided, it **must** be one of these exact values; otherwise omit the scope):
   `agents`, `configs`, `deps`, `e2e`, `export`, `main`, `operator`, `prometheus`, `frontend`, `datasource-syncer`, `config-reloader`, `rule-evaluator`, `ops`
3. **Imperative mood (`imperative: true`)**:
   Start `<description>` with an imperative verb (e.g., `add`, `fix`, `bump`, `update`, `remove`, `sort`, `scope`, `ignore`, `wait`, `revert` — **never** `added`, `fixed`, `adds`, `fixes`, `updating`).
4. **Lowercase start (`case: lower`)**:
   The first character of `<description>` after `: ` **must** be lowercase (e.g., `feat(ops): allow gmpctl to work without gh CLI`, **not** `feat(ops): Allow ...`).
5. **No trailing period (`invalidLastCharacters: .`)**:
   Never end the commit header line with a `.`.
6. **Length limits**:
   * Full header (`<type>(<scope>): <description>`): **max 89 characters**.
   * `<description>` part alone: **max 80 characters**.
7. **US English Spellcheck (`spellcheck.locale: US`)**:
   `conform` spellchecks the entire commit header and body using a US English dictionary. Avoid typos or unknown informal abbreviations in prose.
8. **Verify locally**:
   Run `make conform` (or validate against the rules above if Docker is unavailable) before pushing.

### PR Scope & Backportability
* Keep PRs focused on a single logical change. Do not mix unrelated refactoring, formatting, or major dependency upgrades into bug-fix or security-fix PRs so changes can be cleanly cherry-picked to release branches (`release/0.19`, `release/0.18`, etc.).

---

## 2. Code Generation, Formatting & Licenses (`make regen`)

CI enforces a clean working tree after running code generation and formatting (`CHECK=1 make regen` in [`.github/workflows/presubmit.yml`](./.github/workflows/presubmit.yml)).

* **Never edit generated files solely by hand.** If you modify any of the following sources, you **must** regenerate derivatives:
  * Go API types under `pkg/operator/apis/monitoring/v1/` -> regenerates `pkg/operator/generated/`, `charts/operator/crds/`, `manifests/setup.yaml`, and `doc/api.md`.
  * Helm templates/values under `charts/` -> regenerates `manifests/*.yaml` and `cmd/datasource-syncer/datasource-syncer.yaml`.
  * Any `*.md` file at the root, `cmd/**/*.md`, or `ops/gmpctl/*.md` -> formatted via `mdox fmt --soft-wraps`.
  * Bash scripts (`hack/presubmit.sh`, `ops/gmpctl/lib.sh`) -> formatted via `shfmt -l -w`.
* **Fast local formatting**:

  ```bash
  ./hack/presubmit.sh format
  ```
* **Full regeneration (CRDs, Helm manifests, API docs, formatting, license headers)**:

  ```bash
  make regen
  ```
* **License Headers**:
  Every new code, YAML, Dockerfile, or script file (outside `third_party/` and `vendor/`) must include the standard Google LLC Apache 2.0 license header. Run:

  ```bash
  go tool -modfile="tools/go.mod" addlicense -ignore 'third_party/**' -ignore 'vendor/**' .
  ```

---

## 3. Go Coding & Linting Rules (`make lint`)

Code is checked with `golangci-lint` (`make lint`) using [`.golangci.yml`](./.golangci.yml) (`default: all`).

1. **Mandatory Import Aliases (`importas`)**:
   Unaliased imports of the following packages fail `golangci-lint`:
   * `k8s.io/api/apps/v1` -> **must** be aliased as `appsv1`
   * `k8s.io/api/meta/v1` -> **must** be aliased as `metav1`
   * `k8s.io/apimachinery/pkg/api/errors` -> **must** be aliased as `apierrors`
   * `github.com/GoogleCloudPlatform/prometheus-engine/pkg/operator/apis/monitoring/v1` -> **must** be aliased as `monitoringv1`
2. **Comments Must End with a Period (`godot`)**:
   All Go comments (`scope: all`) must end with a period (`.`), except comments starting with `TODO` or Kubernetes code-gen markers starting with `+`.
3. **Deterministic Reconciler & Collector Output**:
   Never rely on Go map iteration order or unsorted Kubernetes list orders when generating Secret data, collector configs, or CRD statuses. Always sort slices deterministically (e.g., `slices.SortFunc`) so reconcilers do not trigger spurious resource updates.
4. **Defensive Error & Nil Handling**:
   * Check for nil pointers when iterating over optional CRD/relabel pointer slices.
   * When polling Kubernetes API resources in `e2e/` or `pkg/operator/`, only ignore transient `apierrors.IsNotFound(err)` errors rather than swallowing all errors (so RBAC or validation errors fail immediately).
5. **Go Language Idioms & Test Contexts**:
   * In tests, use `t.Context()` rather than `context.Background()` so test contexts are automatically canceled when the test finishes.
   * Keep existing pointer helpers (`ptr.To(...)`, `proto.Bool(...)`) rather than rewriting to `new(expr)` (`modernize.newexpr` is intentionally disabled in `.golangci.yml`).

---

## 4. Go Modules, Dependencies & Dockerfiles

This repository contains multiple Go modules (`go.mod`, `tools/go.mod`, `ops/gmpctl/go.mod`, `examples/scripts/go.mod`, `examples/instrumentation/go-synthetic/go.mod`).

1. **Go Version & `GOTOOLCHAIN=local` Parity**:
   Component `Dockerfile`s (`cmd/*/Dockerfile`, `examples/instrumentation/go-synthetic/Dockerfile`) build with `GOTOOLCHAIN=local`. Never bump the `go` directive in `go.mod` or `tools/go.mod` to a version newer than the `golang:<version>` builder image in the `Dockerfile`s (or vice versa).
2. **`replace` Directives & Pins in `go.mod`**:
   When adding or updating a `replace` directive (e.g., for `github.com/prometheus/prometheus`, `client_golang`, `common`, or `thanos`), keep the `require` and `replace` versions consistent and document the reason for the pin in a comment directly above the `replace` entry.
3. **Dockerfile Efficiency & BuildKit Caching**:
   * Scope `COPY` directives in `cmd/<component>/Dockerfile` to only the required paths (`go.mod`, `go.sum`, `pkg/`, `internal/`, `cmd/<component>/`) rather than copying the entire repository or all of `cmd/`.
   * Preserve BuildKit cache mounts (`--mount=type=cache,target=/go/pkg/mod` and `--mount=type=cache,target=/root/.cache/go-build`) and keep heavy local data/binary directories (`ops/gmpctl/data`, `ops/.bin`) excluded in `.dockerignore`. Note that BuildKit cache mounts apply to the entire `RUN` instruction they are defined on, including all parts of a chained command.
4. **Dependency Footprint**:
   * Avoid introducing external dependencies if the standard library can achieve the desired functionality, to keep the dependency footprint minimal.

---

## 5. Task-Specific Workflows & Commands

Refer to the repository's task guides under [`.gemini/skills/`](./.gemini/skills/) for detailed execution steps:

* **Linting, Formatting & Conform**: [`.gemini/skills/lint-and-format.md`](./.gemini/skills/lint-and-format.md)
  * `make lint` — Run `golangci-lint`.
  * `make conform` — Validate commit messages against `.conform.yaml`.
  * `./hack/presubmit.sh format` — Tidy all `go.mod`s, run `go fmt`, `mdox fmt`, and `shfmt`.
* **Code & Manifest Regeneration**: [`.gemini/skills/code-generation.md`](./.gemini/skills/code-generation.md)
  * `make regen` — Regenerate CRDs, Helm manifests, API docs, and license headers.
  * `CHECK=1 make regen` — Verify no uncommitted diffs or missing license headers remain.
* **Unit Testing**: [`.gemini/skills/unit-tests.md`](./.gemini/skills/unit-tests.md)
  * `NO_DOCKER=1 make test` — Run unit tests natively on the host (fast loop).
  * `go test ./pkg/<path>/... -run <TestName>` — Run targeted unit tests.
* **End-to-End (Kind) Testing**: [`.gemini/skills/e2e-tests.md`](./.gemini/skills/e2e-tests.md)
  * `make e2e` — Build images and run the Kind e2e test suite.
  * `TEST_RUN=<TestName> make e2e-only` — Re-run specific e2e tests without rebuilding images.
* **Building Binaries & Images**: [`.gemini/skills/build-deploy.md`](./.gemini/skills/build-deploy.md)
  * `NO_DOCKER=1 make bin` — Compile all component binaries locally to `./build/bin/`.
