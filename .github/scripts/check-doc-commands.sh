#!/usr/bin/env bash
# check-doc-commands.sh — run the bash blocks of doc pages in clean fixture projects and check their exit codes.
#
# Usage: bash .github/scripts/check-doc-commands.sh <page.md>...
#   A bash block must exit 0. A block directly under <!-- doc-command: expect-fail --> must exit non-zero.
#   Each block runs in a fresh temp project with a git repo and NSELF_V15=1. Nothing in the working tree changes.
# All logic lives in free/ci/tools/doccmds. This script only finds free/ci and execs it.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: check-doc-commands.sh <page.md>..." >&2
  exit 2
fi

script_dir="$(cd "$(dirname "$0")" && pwd)"
ci_dir="$(cd "$script_dir/../../free/ci" && pwd)"

# go -C changes directory, so pages given relative to the caller's directory are made absolute first.
pages=()
for page in "$@"; do
  case "$page" in
    /*) pages+=("$page") ;;
    *) pages+=("$PWD/$page") ;;
  esac
done

export CGO_ENABLED=0
exec go -C "$ci_dir" run -mod=vendor ./tools/doccmds "${pages[@]}"
