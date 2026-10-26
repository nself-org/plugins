#!/bin/sh
# Usage: bash .github/scripts/check-doc-commands.sh <page.md>...
exec env CGO_ENABLED=0 GOFLAGS=-mod=vendor DOCCMDS_CALLER_DIR="$PWD" \
  go -C "$(dirname "$0")/../../free/ci" run ./tools/doccmds "$@"
