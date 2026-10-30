#!/bin/sh
# Usage: bash .github/scripts/check-doc-commands.sh <page.md>...
#
# Runs the bash blocks of the given doc pages in clean fixture projects (free/ci/tools/doccmds).
# The blocks call `nself ci ...`. On a host the installed `nself` would resolve that through the signed
# ci release (E107 when the CLI pins no key for it), so the check builds the plugin from this checkout
# and puts a shim `nself` first on PATH: `nself ci ARGS...` runs the built plugin binary. The check
# therefore needs no installed or signed plugin and behaves the same on a laptop, mm6 and a hosted runner.
# Set DOCCMDS_HOST_NSELF=1 to run the blocks against the `nself` already on PATH instead.
root=$(cd "$(dirname "$0")/../.." && pwd) || exit 2
if [ -z "${DOCCMDS_HOST_NSELF:-}" ]; then
  bin=$(mktemp -d "${TMPDIR:-/tmp}/doccmds-bin.XXXXXX") || exit 2
  trap 'rm -rf "$bin"' EXIT INT TERM
  CGO_ENABLED=0 go -C "$root/free/ci" build -mod=vendor -o "$bin/nself-ci" ./cmd || exit 2
  cat > "$bin/nself" <<'SHIM'
#!/bin/sh
# nself ci ARGS... -> nself-ci ARGS... (the plugin binary built from this checkout)
if [ "${1:-}" != ci ]; then
  echo "nself shim: only 'nself ci' is available in the doc check" >&2
  exit 127
fi
shift
exec "$(dirname "$0")/nself-ci" "$@"
SHIM
  chmod +x "$bin/nself"
  PATH="$bin:$PATH"
  export PATH
fi
env CGO_ENABLED=0 GOFLAGS=-mod=vendor DOCCMDS_CALLER_DIR="$PWD" \
  go -C "$root/free/ci" run ./tools/doccmds "$@"
