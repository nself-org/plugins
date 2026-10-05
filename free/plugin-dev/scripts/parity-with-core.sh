#!/usr/bin/env bash
# parity-with-core.sh: prove nself-plugin-dev is a drop-in for core `nself plugin init|new|dev|debug|link|unlink|test`
# (P7-CANON-18).
# Purpose: build core nself from cli origin/main and the head nself-plugin-dev, then run every case in
#   parity_harness.py on identical scripted sandboxes (HOME, working directory, TMPDIR under $TMPDIR) and
#   compare stdout, stderr, exit code, the commands each run executed (recorded by stub go, docker, dlv and
#   air) and the file effects (path, mode and content hash of HOME, cwd and TMPDIR after the run: generated
#   trees, ~/.nself/plugin-links.json, the dev-watch script). Core's root command log (~/.nself/logs) is
#   excluded: it is written by the core root before any plugin code runs.
# Documented normalisations: the usage path `nself plugin <x>` versus `nself plugin-dev <x>`; durations; the
#   "Files created" list order (core prints Go map order); `link --list` row order (core prints map order);
#   `dev --debug` names the re-executed binary (core re-executes itself as `plugin debug`, this binary as `debug`).
# Not compared against core: the smoke install/uninstall phases of `test` (core would call the registry; no
#   network in tests); they are covered by Go tests with a stub nself and curl.
# Inputs:  run from anywhere inside the plugins worktree (HEAD = the change under test).
#   NSELF            checkout holding cli/ (the core build comes from cli origin/main; required)
#   TMPDIR           scratch parent (required)
#   PARITY_HEAD_SRC  directory of the head free/plugin-dev sources (default: this plugin); negative proofs
#                    point it at a mutated copy.
#   PARITY_CLI_REF   cli ref for the core build (default origin/main)
#   PARITY_NSELF_BIN prebuilt core nself (skips the core build)
# Flags:   --self-test  prove the comparator itself: identical sides pass, a planted difference is reported,
#                       a case with no file effect is rejected.
# Exit:    0 all cases equal, 1 a difference or an empty capture, 2 usage or build failure.
set -euo pipefail
SELF_TEST=0
case "${1:-}" in --self-test) SELF_TEST=1;; "") ;; *) echo "usage: parity-with-core.sh [--self-test]" >&2; exit 2;; esac
: "${TMPDIR:?TMPDIR must be set}"
HERE=$(cd "$(dirname "$0")/.." && pwd)
HEAD_SRC=${PARITY_HEAD_SRC:-$HERE}
CLI_REF=${PARITY_CLI_REF:-origin/main}
CLI_LOCK=${PARITY_CLI_LOCK:-/tmp/nself-cli-compile.lock}
PLUG_LOCK=${PARITY_PLUGINS_LOCK:-/tmp/nself-plugins-compile.lock}
WORK=$(mktemp -d "$TMPDIR/pd-parity.XXXXXX"); WORK=$(cd "$WORK" && pwd -P)
cleanup() {
  if [ -n "${NSELF:-}" ] && [ -d "$NSELF/cli" ]; then git -C "$NSELF/cli" worktree remove --force "$WORK/cli" >/dev/null 2>&1 || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT
locked() { local lock=$1; shift; if command -v lockf >/dev/null 2>&1; then lockf -k "$lock" "$@"; else "$@"; fi; }
die() { echo "parity: $*" >&2; exit 2; }
mkdir -p "$WORK/head" "$WORK/core"
( cd "$HEAD_SRC" && CGO_ENABLED=0 locked "$PLUG_LOCK" go build -o "$WORK/head/nself-plugin-dev" ./cmd/ ) || die "cannot build nself-plugin-dev from $HEAD_SRC"
if [ -n "${PARITY_NSELF_BIN:-}" ]; then cp "$PARITY_NSELF_BIN" "$WORK/core/nself"
else
  : "${NSELF:?NSELF must point at the checkout that holds cli/}"
  git -C "$NSELF/cli" worktree add --detach "$WORK/cli" "$CLI_REF" >/dev/null 2>&1 || die "cannot check out cli $CLI_REF"
  ( cd "$WORK/cli" && CGO_ENABLED=0 locked "$CLI_LOCK" go build -o "$WORK/core/nself" ./cmd/nself ) || die "cannot build core nself"
fi
args=()
[ $SELF_TEST -eq 1 ] && args+=(--self-test)
exec python3 "$HERE/scripts/parity_harness.py" "$WORK/core/nself" "$WORK/head/nself-plugin-dev" "$WORK" ${args[@]+"${args[@]}"}
