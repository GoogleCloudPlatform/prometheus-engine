## Collector Module

The `github.com/GoogleCloudPlatform/prometheus-engine/collector` Go module contains
the code is meant to be imported and used inside Prometheus code for the non-trivial 
forked functionalities that we are eventually upstreaming:

* `export`: In-memory export pipeline to Google Cloud Monitoring gRPC API. This 
Go package will be deprecated once Prometheus and GCM supports [Remote Write 2.0](https://prometheus.io/docs/specs/prw/remote_write_spec_2_0/) fully.
* `secrets`: Kubernetes secret provider implementation required for the scalable 
secure scrape support. This package will be deprecated once [PROM-47](https://github.com/prometheus/proposals/pull/47) is implemented.

See the example use in [Prometheus GMP fork](https://github.com/GoogleCloudPlatform/prometheus/commit/70a64acc1bc145200f2ef9ffe61770463577b4b9).

### Why hosting them in prometheus-engine?

Hosting this code here has the big disadvantage of circular Prometheus module dependency. Especially the `export` package
requires a low-level prometheus packages that can change over time. However, different Prometheus fork versions require to
import `export` code, which causes us to release multiple versions of this `collector` supporting new versions of Prometheus
code.

The mapping is not 1:1, i.e., we don't need to release `collector` module for every minor or patch version of Prometheus. We only need to bump Prometheus dependency on breaking changes, which happen occasionally.

The benefit of storing them here is the smaller forked code surface. Forked functionality has to be propagated and carried
on multiple versions which is tedious and risky; adding more work and risk than the occasional separate collector module [releases](#releasing).

### Releasing

> NOTE: This module follows a separate versioning scheme than prometheus-engine, because of a tight circular dependency on a Prometheus module.

To release it:

1. Decide if you want to perform minor or patch release. Minor release should be performed on **every** Prometheus dependency upgrade and any other major change.
2. Change `./VERSION` to the desired version e.g. `0.2.0`
3. Git tag the module, allowing external repositories to import our `collector` module:
   * For the minor release e.g. **0.1.x -> 0.2.0**:
 
```bash
VERSION="$(< VERSION)" # Assuming you changed the value to 0.2.0.
git switch main
# Ensure you are on the latest main commit.
git pull origin main # or git reset --hard origin/main
# Create a collector release branch "release/collector/0.2".
git switch -c "release/collector/${VERSION%.*}"
# Create a collector, signed git tag, "collector/v0.2.0".
git tag -s "collector/v${VERSION}" -m "collector/v${VERSION}"
# Push both.
git push origin "release/collector/${VERSION%.*}" "collector/v${VERSION}"
```
   * For the patch release e.g. **0.2.0 -> 0.2.1**:

```bash
VERSION="$(< VERSION)" # Assuming you changed the value to 0.2.1.
git switch "release/collector/${VERSION%.*}"
# Ensure you are on the latest collector/0.2 commit.
git pull origin "release/collector/${VERSION%.*}" # or git reset --hard "origin/release/collector/${VERSION%.*}"
# Create a collector, signed git tag, "collector/v0.2.1".
git tag -s "collector/v${VERSION}" -m "collector/v${VERSION}"
# Push the tag.
git push origin "collector/v${VERSION}"
````
