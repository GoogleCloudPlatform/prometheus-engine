# UI

Our goal is to provide the upstream Prometheus UI with the only change that it
queries data from Google Cloud Prometheus Engine and with support for pages
other than `/graph` removed.

Since the UI is not a public NPM package, importing the specific React components
for a custom apps is very difficult. Thus we use a simpler approach where we load
the upstream UI at a given version and apply a minimal set of overrides files.

## How it works

### Development

The `sync.sh` script syncs the upstream repository into `build/` at a fixed git tag.
All files and directories provided in `override/` are copied to the `web/ui/react-app`
path of the upstream checkout.

Invoking the sync script ensures this synchronization and runs any following command
in the `web/ui/react-app` directory.

To initialize:

```bash
./sync.sh yarn
```

To run the UI locally:

```bash
./sync.sh yarn start
```

The webserver will proxy requests to `http://localhost:9090`. During development a local Prometheus
server or the frontend can be run under this address for manual testing.

At build time the app is statically compiled into the `ui` Go package. Binaries including the UI
must also host a Prometheus-compatible read API as expected by the React app.

### Building

The final app is statically compiled into the Go binary for release. To generate the static
asset Go file run:

```bash
./build.sh
```