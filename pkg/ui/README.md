# UI (`pkg/ui`)

This package provides the embedded Prometheus web UI served by [`cmd/frontend`](../../cmd/frontend/README.md), customized to query Google Cloud Managed Service for Prometheus (GMP) with navigation restricted to the `/graph` view (including Metric Metadata).

Because the upstream Prometheus UI is not published as a standalone NPM package, we vendor the upstream UI source tree at a pinned Prometheus release tag and overlay a minimal set of override files at build time.

## How It Works

* [`third_party/prometheus_ui/base/`](../../third_party/prometheus_ui/base) contains an unmodified copy of the upstream Prometheus UI scripts and [`web/ui`](https://github.com/prometheus/prometheus/tree/main/web/ui) directory from the git tag recorded in [`third_party/prometheus_ui/base/VERSION`](../../third_party/prometheus_ui/base/VERSION).
  * **Do not edit any files under `third_party/prometheus_ui/base/` manually.** Running `./hack/update-ui.sh` wipes and re-creates `third_party/prometheus_ui/base/`.
  * To support hermetic container builds, [`third_party/prometheus_ui/base/web/ui/node_modules/`](../../third_party/prometheus_ui/base/web/ui/node_modules) is vendored in git matching [`third_party/prometheus_ui/base/web/ui/package-lock.json`](../../third_party/prometheus_ui/base/web/ui/package-lock.json).
* [`third_party/prometheus_ui/override/`](../../third_party/prometheus_ui/override) contains the GMP-specific React override files (customizing the navbar/title and keeping only `/graph`).
* [`pkg/ui/build.sh`](build.sh) (which must be executed from the **repository root**) copies `base/`, overlays `override/`, runs the NPM build, and outputs the compiled static assets into `pkg/ui/embed/static/`.
* [`cmd/frontend/main.go`](../../cmd/frontend/main.go) serves the UI via [`pkg/ui/ui.go`](ui.go) using one of two build-tag-controlled asset loaders:
  * [`assets_embed.go`](assets_embed.go) (`//go:build builtinassets`): embeds `pkg/ui/embed/static/` directly into the Go binary (used by `cmd/frontend/Dockerfile` and `go run -tags builtinassets ./cmd/frontend`).
  * [`assets_local.go`](assets_local.go) (`//go:build !builtinassets`): serves `./static/` from the current working directory on disk when `-tags builtinassets` is not set.

## Security Vulnerabilities (`npm audit`)

Because `node_modules` is vendored in git, security scanners may flag vulnerabilities in transitive devDependencies:

* To inspect NPM vulnerabilities directly, run `npm audit` from the [`third_party/prometheus_ui/base/web/ui/`](../../third_party/prometheus_ui/base/web/ui) directory (where `package.json` and `package-lock.json` reside):

```bash
cd third_party/prometheus_ui/base/web/ui && npm audit
```

* If `npm audit` reports critical vulnerabilities affecting the UI bundle, first try [updating the base UI](#updating-the-base-ui) to a newer upstream Prometheus release tag.
* If upstream Prometheus has not yet resolved the vulnerability, evaluate whether the affected package is used at runtime or only during build/test, and coordinate an upstream fix in `prometheus/prometheus`.

## Updating the Base UI

From the **repository root**, run [`./hack/update-ui.sh`](../../hack/update-ui.sh) with the target upstream Prometheus git tag:

```bash
bash hack/update-ui.sh <PROMETHEUS_TAG>
```

This script wipes `third_party/prometheus_ui/base/`, clones the upstream Prometheus UI at `<PROMETHEUS_TAG>`, and vendors `node_modules/`.
If upstream files in `third_party/prometheus_ui/base/web/ui/` changed significantly, reconcile [`third_party/prometheus_ui/override/`](../../third_party/prometheus_ui/override) as described below.

## Modifying Override Files

Before modifying files in [`third_party/prometheus_ui/override/`](../../third_party/prometheus_ui/override), inspect their header comments and diff them against the corresponding files in `third_party/prometheus_ui/base/` to see the exact GMP customizations:

1. Diff the existing file in `override/` against `base/` to identify the GMP-specific changes.
2. Copy the updated file from `third_party/prometheus_ui/base/` into `third_party/prometheus_ui/override/` and re-apply the GMP changes.
3. From the repository root, run [`bash pkg/ui/lint-override.sh`](lint-override.sh) to format and lint the override files.
4. Rebuild and test the UI assets from the repository root:

```bash
./pkg/ui/build.sh --test
```

## Building & Testing the UI

The UI is compiled automatically during `make frontend` (Docker build). To compile the static assets into `pkg/ui/embed/static/` locally from the **repository root**, run:

```bash
./pkg/ui/build.sh
```

To build the `gmp/frontend` Docker image and test the UI manually (listening on `:19090` by default at `http://localhost:19090/graph`):

```bash
make frontend
docker run --net=host -it --rm gmp/frontend:latest --query.project-id=test-proj
```

Or run directly on the host from the repository root after running `./pkg/ui/build.sh`:

```bash
go run -tags builtinassets ./cmd/frontend --query.project-id=test-proj
```
