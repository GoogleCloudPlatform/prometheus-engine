#!/usr/bin/env bash
#
# License: Apache-2.0
#
# Description: Convert go-kit/log to slog calls
#
# Authors:
# - Ben Kochie (@superq)
# - TJ Hoplock (@tjhop)
#
# Example scripted usage against all go files in a repository:
#       find . -type f -name '*.go' -print -exec ./convert_go-kit_slog.bash {} \;
#
# Recommendations:
# - ensure deps are up to date for things like prometheus/common, prometheus/exporter-toolkit, etc (ex: `go get -u ./...`)
# - manually review/verify -- automation doesn't catch everything! ;)
# - tidy modules after, and hopefully drop `go-kit/log` and `go-logfmt/logfmt` deps from builds (ex: `go mod tidy`)

if [[ $# -ne 1 ]]; then
  echo "usage: $(basename $0) <file>"
  exit 1
fi

file="$1"

sed_script="$(cat << 'SEDSCRIPT'
s/promlog/promslog/g
s/log.Logger/*slog.Logger/g
s/log.NewNopLogger\(\)/promslog.NewNopLogger()/g
s/stdlog.New\(log.NewStdlibAdapter\(level.(Debug|Info|Warn|Error)\((\w+)\)\), "", 0\)/slog.NewLogLogger(\2.Handler(), slog.Level\1)/g
s/level\.(Debug|Info|Warn|Error)\((.+)\).Log\("\w+", /\2.\1(/
s/log\.With\((\w+), (.+)\)/\1.With(\2)/
SEDSCRIPT
)"

sed -i -E "${sed_script}" "${file}"

goimports -w "${file}"

go fmt "${file}"
