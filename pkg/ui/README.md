# UI

Our goal is to provide the upstream Prometheus UI with the only change that it
queries data from Google Cloud Prometheus Engine and with support for pages
other than `/graph` removed.

Since the UI is not a public NPM package, importing the specific React components
for a custom apps is very difficult. Thus we use a simpler approach where we load
the upstream UI at a given version and apply a minimal set of overrides files.

## How it works

### Development

The `hack/update-ui.sh <PROMETHEUS_TAG>` script syncs the upstream UI into
`third_party/prometheus_ui/base` at a fixed git tag.
All files and directories provided in `thid_party/prometheus_ui/override/`
are copied over the `web/ui/react-app` path of the upstream checkout. After updating
the upstream tag, the override files may need adjustment.

### Building

The final app is statically compiled into the Go binary for release. To build the UI, run:

```bash
./build.sh
```

This creates the final gziped files in `./build` and generated `./embed.go` to instruct the Go
compiler to include them into the final Go binary.