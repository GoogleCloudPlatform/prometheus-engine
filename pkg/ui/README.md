# UI

Our goal is to provide the upstream Prometheus UI for `frontend` with the 
only change that it queries data from Google Cloud Prometheus Engine and with
support for pages other than `/graph` removed.

Since the UI is not a public NPM package, importing the specific React components
for a custom apps is very difficult. Thus, we use a simpler approach where we load
the upstream UI at a given version and apply a minimal set of overrides files.

## How it works

* [`/third_party/prometheus_ui/base`](/third_party/prometheus_ui/base) hosts the code from Prometheus.
Do not change any of the files manually. This should be an exact copy of Prometheus
of the relevant UI parts: scripts and [`web/ui`](https://github.com/prometheus/prometheus/blob/main/web/ui/)
directory from the version (git tag) captured in [`/third_party/prometheus_ui/base/VERSION`](/third_party/prometheus_ui/base/VERSION) file.
The built [`/third_party/prometheus_ui/base`](/third_party/prometheus_ui/base) is then compiled into Go embedded asset during frontend docker image creation.
The frontend then [imports that Go assets and handler](/cmd/frontend/main.go#L144).
  * The only change vs upstream Prometheus is that use [vendoring](https://cloud.google.com/software-supply-chain-security/docs/dependencies). This means
there is a committed [/`third_party/prometheus_ui/base/web/ui/node_module/`](/third_party/prometheus_ui/base/web/ui/node_modules)
directory, which should contain all dependencies with the exact version from
[`/third_party/prometheus_ui/base/web/ui/package-lock.json`](/third_party/prometheus_ui/base/web/ui/package-lock.json).

> NOTE: In theory, we only need subset of modules ("dependencies" in package.json).
> Unfortunately many packages does not follow the distinction between ["dev", "peer" and normal dependencies](https://www.geeksforgeeks.org/difference-between-dependencies-devdependencies-and-peerdependencies/)
> thus we need to vendor and install all dependencies to build UI.

* [`/third_party/prometheus_ui/override`](/third_party/prometheus_ui/override) hosts
files we are replacing in the Prometheus UI. Currently, we do that to change
UI title and remove all links but /graph. Override files are applied during [build](#building-ui)
stage.

### Security Vulnerabilities

Npm packages have generally high load of the security vulnerabilities. If a
scanned marked a security issue on our UI:

* Ensure it relates to our UI. The fact we are vendoring node_modules tend to trigger
many false positives for dev dependencies of our dev dependencies etc. To uncover 
if we are affected, go to [`/third_party/prometheus_ui/base`](/third_party/prometheus_ui/base)
and run `npm audit`.
* If audit shows critical vulnerabilities, try to [update our base](#updating-base-ui)
with the latest Prometheus release. It's likely Prometheus already updated deps accordingly.
* If Prometheus does not fixed it, report and fix on Prometheus first. This might mean
assessing actual damage radius (often not relevant to what we do). This can be discussed
with Prometheus team (e.g. Julien or Julius).

To avoid distractions and old vulnerabilities, [update base](#updating-base-ui) regularly.

### Updating base UI

The [`bash hack/update-ui.sh <PROMETHEUS_TAG>`](/hack/update-ui.sh) script syncs
the upstream UI into `third_party/prometheus_ui/base` at a fixed git tag and adds
vendoring.

If the relevant files in [`/third_party/prometheus_ui/base/web/ui/`](/third_party/prometheus_ui/base/web/ui/)
changed, we might have build or semantics issues with our overrides. Refer to
[Modifying override files](#modifying-override-files) on how to fix and resolve those.

### Modifying override files

Before modifying [the override files](/third_party/prometheus_ui/override) refer
to the header comments to understand the changes applied. It's recommended
to recreate the override files in the event of bigger base UI changes.

The recreation of the override flow could look like this:

1. Use file diff (e.g. online) between original and overriden file to understand changes to apply.
2. Copy new original file from base. 
3. Looking on diff apply manually changes.
4. Use [`bash pkg/ui/lint-override.sh`](/pkg/ui/lint-override.sh) to format code.
5. Test the UI build (refer to [next section](#building-ui))

### Building UI

There is no need to manually compile the assets or build UI. This is done as a
part of frontend docker image build. To test the build and UI locally, run from
the repo root:

```
make frontend
```

### Testing UI

We don't have automatic test for UI. To test frontend UI manually, run:

```
docker run --net=host -it --rm gmp/frontend:latest --query.project-id=whatever
```
