# Config Reloader

Small binary, a wrapper on top of github.com/thanos-io/thanos/pkg/reloader for extra checks and tuning.
Meant to be run as a sidecar.

## Flags

```bash mdox-exec="bash hack/format_help.sh config-reloader"
Usage of config-reloader:
  -config-dir string
    	config directory to watch for changes
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
