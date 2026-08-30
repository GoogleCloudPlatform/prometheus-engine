# Config Reloader

Small binary, a wrapper on top of github.com/thanos-io/thanos/pkg/reloader for extra checks and tuning.
Meant to be run as a sidecar.

## Flags

```bash mdox-exec="bash hack/format_help.sh config-reloader"
Usage of config-reloader:
  -config-dir string
    	config directory to watch for changes
  -config-dir-from-configmap-namespace string
    	namespace to list ConfigMaps from (required when --config-dir-from-configmap-selector is set)
  -config-dir-from-configmap-selector string
    	label selector to discover ConfigMaps via the Kubernetes API (e.g. monitoring.googleapis.com/rules-shard=true). When set, entries of the matching ConfigMaps are materialized into --config-dir, which must be writable by this process.
  -config-dir-output string
    	config directory to write with interpolated environment variables
  -config-file string
    	config file to watch for changes
  -config-file-output string
    	config file to write with interpolated environment variables
  -listen-address string
    	address on which to expose metrics (default ":19091")
  -ready-startup-probing-interval duration
    	how often to poll ready endpoint during startup (default 1s)
  -ready-startup-probing-no-conn-threshold int
    	how many times ready endpoint can fail due to no connection failure. This can happen if the config-reloader starts faster than the config target endpoint readiness server. (default 5)
  -ready-url string
    	ready endpoint of the configuration target that returns a 200 when ready to serve traffic. If set, the config-reloader will probe it on startup (default "http://127.0.0.1:19090/-/ready")
  -reload-url string
    	reload endpoint of the configuration target that triggers a reload of the configuration file (default "http://127.0.0.1:19090/-/reload")
  -watched-dir value
    	directory to watch for file changes (for rule and secret files, may be repeated)
```

## ConfigMap sync

With `--config-dir-from-configmap-selector`, a background syncer lists the matching ConfigMaps and materializes their entries into `--config-dir`, using the layout the kubelet uses for ConfigMap volumes:

```
/etc/rules/
  ..2026_05_20_12_00_00.000000000/          # payload, one file per ConfigMap key
  ..data -> ..2026_05_20_12_00_00.000000000
  rules__default__foo.yaml -> ..data/rules__default__foo.yaml
```

Publishing a new set is a single rename of `..data`, so a reader listing the directory never sees a half-written set of files. The reloader then copies `--config-dir` to `--config-dir-output` as usual.

Keys are unique across the ConfigMaps, so the ConfigMap an entry came from is not part of the file name: re-sharding entries across ConfigMaps never renames a file. A duplicate key is taken from the first ConfigMap in name order and logged.
