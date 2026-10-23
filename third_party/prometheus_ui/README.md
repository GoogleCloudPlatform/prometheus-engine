# Third-Party Prometheus UI

This directory contains a vendored snapshot of the upstream [Prometheus React UI](https://github.com/prometheus/prometheus/tree/main/web/ui) (`base/`) alongside GMP-specific override files (`override/`).
At build time, [`pkg/ui/build.sh`](../../pkg/ui/build.sh) merges `override/` onto `base/` to produce a subset of the Prometheus UI scoped to the `/graph` page.

> [!CAUTION]
> **Do not manually edit any files under `third_party/prometheus_ui/base/`**, and **do not follow the upstream build instructions inside `third_party/prometheus_ui/base/**/README.md`**.
> The entire `third_party/prometheus_ui/base/` directory is deleted and regenerated from upstream whenever [`./hack/update-ui.sh`](../../hack/update-ui.sh) is run. All GMP UI customizations must be placed in `third_party/prometheus_ui/override/`.

## How to Sync, Modify, and Build

Refer to [`pkg/ui/README.md`](../../pkg/ui/README.md) for complete instructions on syncing upstream Prometheus UI releases, editing `override/` files, running `npm audit`, and compiling embedded UI assets for `cmd/frontend`.
