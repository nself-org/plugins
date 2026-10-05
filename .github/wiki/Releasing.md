# Releasing

## Binary targets

The free-plugin release (`.github/workflows/release-tarballs.yml`) compiles a command binary for every
plugin that ships one, for five platforms. Which binaries, and which commands they carry, is not read
from `plugin.json` by the workflow. `scripts/release/cli-targets.sh DIR` prints them, one
`binary<TAB>command` line each, from the cli tool `manifestv2migrate -targets`.

- The tool normalizes a v1 or v2 `plugin.json` and applies the installer's own rule
  (`manifestv2.CLITargets`, the same function `nself add` uses to decide which binaries to link), so
  the release builds exactly what the installer will look for.
- A plugin with no command prints nothing and exits 0. A manifest the tool refuses exits 1 and fails the
  release; it is never skipped silently.
- The tool comes from the cli commit pinned in `scripts/cli-tools.version`. The workflow builds it once
  (Go, from that commit's vendor directory) and shares it with the parallel plugin builds. Changing the
  pin is the way to pick up a new rule.
- Fixtures: `tests/fixtures/manifest-v1-cli/` and its v2 twin `tests/fixtures/manifest-v2-cli/` must print
  identical targets.
