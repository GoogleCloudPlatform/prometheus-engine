# gmpctl

`gmpctl` is an interactive CLI for common operations on GMP projects.

It's a starting point for smaller or bigger automation on OSS side (e.g. releasing, syncing or even debugging).

> NOTE: This script is far from perfect, but it's better than doing things manually. Feel free to contribute,
> fix bugs and add more automation for common tasks!

## Setup

1. To start using `gmpctl` you need to have a clone of `prometheus-engine` on your machine (you probably have already one!
   to fetch the latest `main` for the best experience (latest scripts).

2. The next this is to obtain NVD API key to avoid rate-limits when querying CVE DB. [Request an API Key](https://nvd.nist.gov/developers/request-an-api-key) and save this key to `ops/vulnupdatelist/api.text`

3. Ensure you have installed:
   * new-ish `bash` (MacOS: `brew install bash`)
   * `gsed` (MacOS: `brew install gsed`)
   * `gcloud` (https://docs.cloud.google.com/sdk/docs/install-sdk) (and `gcloud auth login`)
   * `gpg` (MacOS: `brew install gpg`)

4. You can configure different work directory for gmpctl via `-c` flag. By default, `gmpctl` does the work in `ops/gmpctl/.data`)

Enjoy!

## Usage

Generally `gmpctl` does not need flags for general usage. It interactively asks you for
key information and confirmations e.g.

```bash
./ops/gmpctl.sh release                         
┃ What do you want to release? 
 ┃ > release/0.17
 ┃   release/0.15
 ┃   release/0.14
 ┃   release/0.12
 ┃   release-2.45.3-gmp
 ┃   release-2.53.5-gmp
↑ up • ↓ down • / filter • enter submit
```

`gmpctl` maintains 1 git clone for each project and uses `git worktree` for each command and branch.

`gmpctl` commands are aimed to be **idempotent**, meaning you should be able to run it multiple times with the
same parameters, and it will continue the previous work or at least yield same results. This is crucial when iterating
on breaking go mod updates for vulnerabilities or fork sync conflicts.

```text mdox-exec="bash ops/gmpctl.sh --help"
Usage: gmpctl [COMMAND] [FLAGS]
  -c string
    	Path to the configuration file. See config.go#Config for the structure. (default ".gmpctl.default.yaml")
  -git.prefer-https
    	If true, uses HTTPS protocol instead of git for remote URLs. 
  -v	Enabled verbose, debug output (e.g. logging os.Exec commands)

--- Commands ---
[release] Usage of release:
  -b string
    	Release branch to work on; Project is auto-detected from this
  -patch
    	If true, and --tag is empty, forces a new patch version as a new TAG.
  -t string
    	Tag to release. If empty, next TAG version will be auto-detected (double check this!)

[vulnfix] Usage of vulnfix:
  -b string
    	Release branch to work on; Project is auto-detected from this
  -pr-branch string
    	(default: $USER/BRANCH-vulnfix) Upstream branch to push to (user-confirmed first).
  -sync-dockerfiles-from
    	Optional branch name to sync Dockerfiles from. Useful when things changed.
```

## `gmpctl` development

Some rules to follow:

* Downstream functions should literally use `panicf` for error handling. This improves readability and enormously help
  with debugging errors. The obvious exception is when code needs to handle this error. Then swap panic with a proper `err error` pattern.
* Offer choice, be interactive! See `dialog.go` and https://github.com/charmbracelet/huh on what's possible.

## Bash development

While the `gmpctl` is written in Go, you might notice some functionalities are in Bash.

Bash is funky, but sometimes more readable than Go/easier to iterate.
Eventually, we could rewrite more critical pieces to Go, but you're welcome to add some quick
pieces in bash to automate some stuff.

It's trivial to call bash function from `gmpctl` e.g.:

```go
if err := runLibFunction(dir, opts, "release-lib:vulnfix"); err != nil {
	return err
} 
```

Some rules to follow:

* CI checks bash formatting via https://github.com/mvdan/sh?tab=readme-ov-file#shfmt. You can install this on your IDE for formatting.
* Write only libraries (functions). The starting point for scripts should be always Go gmpctl CLI.
* Function names have `release-lib::` prefix to figure out where they come from.
* Function check their required arguments/envvars; always.
* Especially for functions that return strings via stdout:
  * Ensure all error messages are redirected to stderr, use log_err func for this.
  * Be careful with pushd/popd which log to stdout, you can redirect those to stderr too.

## TODO / Known issues

* [ ] Port bash to Go for stable commands.
* [ ] Ability to configure NVD API key in gmpctl config.
* [ ] Port fork-sync script from the old PR.
* [ ] Generate some on-demand query of vulnerabilities for all releases (aka dashboard.)
* [ ] Fix NPM vulns (although it's rate).
* [ ] Ability to schedule multiple scripts at once and managing that? (lot's of work vs multiple terminals)
