# Releasing

## Binary targets

The free-plugin release (`.github/workflows/release-tarballs.yml`) compiles a command binary for every
plugin that ships one, for five platforms. Which binaries, and which commands they carry, is not read
from `plugin.json` by the workflow. `scripts/release/cli-targets.sh DIR` prints them, one
`binary<TAB>command` line each, from the cli tool `manifestv2migrate -targets`.

- The tool normalizes a v1 or v2 `plugin.json` and applies the installer's own rule
  (`manifestv2.CLITargets`, the same function `nself add` uses to decide which binaries to link), so
  the release builds exactly what the installer will look for.
- `scripts/release/cli-targets-gate.sh` runs over every free plugin first and decides what may ship. It blocks a plugin
  whose commands include a core verb or top-level command of the pinned cli (read from that commit's
  `.github/command-registry.json`), a name outside `^[a-z][a-z0-9-]*$`, a binary another plugin also yields, or a
  manifest the normalizer refuses. Each blocked plugin needs an unexpired entry (rule `cli-targets`, owner_ref naming
  the fixing Ticket, expiry within the v1.5.0 cap) in `tests/conformance/exceptions.yaml`, else the run fails.
  Blocked plugins ship no binary and are listed in the job summary.
- A plugin with no command prints nothing and exits 0. A manifest the tool refuses exits 1; the gate turns that into a block that needs an exception.
- The tool comes from the cli commit pinned in `scripts/cli-tools.version`. The workflow builds it once
  (Go, from that commit's vendor directory) and shares it with the parallel plugin builds. Changing the
  pin is the way to pick up a new rule.
- Fixtures: `tests/fixtures/manifest-v1-cli/` and its v2 twin `tests/fixtures/manifest-v2-cli/` must print
  identical targets.
