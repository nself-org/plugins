#!/usr/bin/env bash
# Purpose: keep the v1 API private while manifest-only conformance skips it.
# Inputs: the deployed compose fragment and the repository portbind checker.
# Outputs: violations on stdout and a nonzero exit status on unsafe bindings.
# Constraints: run from any directory; require valid Compose YAML (fail closed).
set -euo pipefail

plugin_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
repo_dir=$(cd "$plugin_dir/../.." && pwd)
cd "$repo_dir/scripts/conformance/portbind"
go run . -require "$plugin_dir/docker-compose.plugin.yml"
