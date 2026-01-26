#!/usr/bin/env bash
#
# compress static assets

set -euo pipefail

cd web/ui

GZIP_OPTS="-fk"
# gzip option '-k' may not always exist in the latest gzip available on different distros.
if ! gzip -k -h &>/dev/null; then GZIP_OPTS="-f"; fi

find static -type f -name '*.gz' -delete
find static -type f -exec gzip $GZIP_OPTS '{}' \;
FILES=$(find static -name "*.gz" -type f | tr '\n' ' ')
sed -i "s|// {{go:embed}}|//go:embed $FILES|" embed.go