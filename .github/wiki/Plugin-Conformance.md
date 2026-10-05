# Plugin Conformance

The conformance gate checks every free plugin whose `plugin.json` has `manifest_version: 2`. Plugins still on manifest v1 keep the checks in `.github/workflows/validate.yml` and are only counted by the gate. Three codemods convert a v1 plugin, and an oracle proves that the released CLI v1.4.12 still installs the converted result.

## Run the gate

```bash
bash scripts/conformance/check.sh --plugins nself-foo,nself-bar   # named plugins
bash scripts/conformance/check.sh --changed                        # plugins changed against origin/main
bash scripts/conformance/check.sh --all                            # every plugin under free/
bash scripts/conformance/check.sh --fixtures                       # proves the gate itself (see below)
bash scripts/conformance/check.sh --plugins x --rules schema,compat   # a subset; only those rules run
```

Exit 0 means every selected rule passed. A rule that cannot run (no Docker, no tool) fails; nothing passes by being skipped. A named plugin that does not exist fails, and `--plugins` or `--all` that checks no manifest v2 plugin fails too (a gate that ran nothing is not a pass). Only `--changed` with nothing changed passes.

## Rules

| Rule | What it requires |
|---|---|
| `exceptions` | Every entry in `scripts/conformance/exceptions.yaml` is complete, expires on or before the v1.5.0 cap date and has not expired. |
| `schema` | `plugin.json` validates against the pinned v2 schema `schemas/plugin-manifest.v2.schema.json`. |
| `compat` | The manifest is canonical v2, has no forbidden v1 key, and the generated compatibility keys equal what `manifestv2migrate` projects. |
| `healthcheck` | A compose plugin has a real health check: a compose `healthcheck:` that is not disabled (`disable: true`, `test: NONE`), or a Dockerfile `HEALTHCHECK` whose last instruction is not `NONE`. A disabled compose healthcheck fails even when the Dockerfile has one. |
| `replace` | No `go.mod` or `go.work` `replace` or `use` points outside the plugin directory, quoted targets included. |
| `docker-socket` | No `docker.sock`, `/var/run/docker` or `/var/run` bind in the compose fragment named by `service.compose` (whatever it is called), in any `*.yml` or `*.yaml` of the plugin, or in a Dockerfile (ADR 0027), unless a dated exception names the plugin. |
| `migrations` | A plugin with `migrations/*.sql` declares `migrations.apply: boot`, and a Go plugin calls `migrate.Apply` at boot in non-test source: a call in a comment or only in a `_test.go` file does not count. |
| `repository` | A free manifest names `https://github.com/nself-org/plugins`; a licensed one names `https://github.com/nself-org/bundles`. |
| `ports` | No port is shared by two plugins or claimed from `scripts/conformance/reserved-ports.yaml`. |
| `build` | The plugin builds from its own extracted deterministic tarball with no network (`build-from-tarball.sh`). |

`ports.sh` reads the top-level `port`, `config.port`, `service.port` and every `*_PORT` default under the optional env of each manifest. `ports.sh --all --report` lists collisions among all current plugins (legacy v1 included) and exits 0.

## Exceptions

`scripts/conformance/exceptions.yaml` lists the only allowed rule breaks. Each entry has `rule`, `plugin`, `reason`, `owner_ref` and `expires`. An expired entry fails the gate, and `expires` may not be later than the v1.5.0 cap date. The cap is a constant in `scripts/conformance/lib.sh` (`release_cap`), not read from the data file, so a file cannot raise its own cap. Remove an entry in the same change that lands its owner Ticket. Today: `docker-socket` for `forgejo` and `monitoring`.

## Pinned schema

The schema copy is pulled, never edited. `scripts/cli-tools.version` pins the `nself-org/cli` commit that holds `tools/manifestv2migrate`, the schema and `sdk/go/migrate`. `sync-schema.sh` copies the schema from that commit and records its sha256 in `schemas/SOURCE`; `sync-schema.sh --check` fails on any mismatch. To move the pin, change the sha, then run `sync-schema.sh`.

## Codemods

All three are idempotent: a second run reports `noop` and changes nothing. Each plugin is converted as ONE all-or-nothing unit: `run.sh` snapshots the plugin directory and restores it exactly if any step fails or is manual, and each script restores its own writes on failure. Boot and vendor run only after the manifest step converted (or was already a no-op); a plugin whose manifest is blocked or manual is not touched.

```bash
bash scripts/codemods/run.sh --plugins nself-foo            # convert in place
bash scripts/codemods/run.sh --dry-run --all --report r.txt # per-plugin report and summary, writes nothing
bash scripts/codemods/run.sh --fixtures --twice             # prove them on tests/conformance/fixtures
bash tests/conformance/realtree.sh --drop-dev-tool-keys --plugins a,b   # idempotence on the real free/ tree (twice, zero changes)
```

| Codemod | Does |
|---|---|
| `manifest-v2.sh` | Runs `manifestv2migrate` with compatibility keys, sets `repository` on free manifests, drops the dead keys in the cli drop list. Keys that plugins dev tools still read (`dev-tool-keys.txt`: actions, config, binary_name, env_required, env_optional, defaultPort) are KEPT, so a manifest carrying one reports `blocked: <keys>` and nothing is written, until a Ticket whose scope updates the readers passes `--drop-dev-tool-keys`. A v1 key that is neither mapped nor on a drop list also blocks. A converted manifest whose `service.compose` fragment does not exist reports `manual` and is not written. |
| `vendor.sh` | Copies out-of-dir `replace` targets into `third_party/`, runs `go mod vendor` for the plugin's own modules (not `third_party/`, not a module reached through a local replace), adds `-mod=vendor` to the Dockerfile build. A Dockerfile that runs `go mod download` reports `manual` before anything is written. |
| `boot-migrations.sh` | For a Go plugin with `migrations/`: `migrations.apply: boot`, a `go:embed` package, one `Apply` call after the pgx pool is created, a `/health` migrations member, and the SDK module copied under `third_party/cli-sdk-go` with its LICENSE and tidy `go.mod`/`go.sum`. A plugin with more than one `go.mod`, or whose pool pattern is not found, reports `manual` and nothing is written. |

`run.sh` prints one `plugin <name> manifest=<s> boot=<s> vendor=<s>` line per plugin and ends with `summary` lines (counts per step and the blocked and manual plugin lists). States: `converted`, `noop`, `none` (nothing to do), `manual` (needs hand wiring), `blocked` (unmapped key, failed step) and `skipped`. Outside `--dry-run` a blocked plugin makes the run exit 1; a dry run always exits 0 because the report is its product.

## Fresh-database boot kit

`bash tests/boot/run.sh --fixtures` builds each converted fixture image, starts `postgres:16`, boots the plugin twice and requires: the first boot applies every migration, the second applies none, and `/health` reports `applied == expected`. Use `--plugins a,b` for real plugins and `--sequential` on small machines.

## v1.4.12 oracle

`bash scripts/conformance/v14-oracle.sh --fixtures` runs its container as the host uid (`--user`), so the files it writes into the work directory are readable by the host user on native Linux. It downloads the `nself` v1.4.12 linux release (nothing else), verifies its sha256 against the release `checksums.txt`, then in a container with no network installs each plugin twice from a local registry: once as its v1 original, once converted. It diffs the generated compose, `compose-files.txt`, the `PLUGIN_*_INTERNAL_URL` lines in `.nself/compose.env`, the `np_plugins` seed SQL, `plugin list --installed --detailed` and the `plugin remove` guard on a depended-on plugin. Any difference fails unless `tests/conformance/oracle-deltas.yaml` lists it (`slug`, `file`, `key`, `from`, `to`, `owner_ref`, `expires`). A row matches literally (never a regex), only in its own output file, and only for a plugin under test: `from: "-"` removes only a converted-side line exactly equal to `key`; otherwise lines containing `key` have `to` replaced by `from`. A row that matches nothing while its plugin is under test is stale and fails, an expired row fails, and an `expires` after the v1.5.0 cap (a constant in `lib.sh`) fails. `tests/conformance/oracle-deltas-test.sh` holds the negative proofs. With `--plugins a,b` the originals come from `origin/main`.

Known difference: v1.4.12 parses `dependencies` only as an array. The object form `{required, optional}` used by most free manifests makes it skip that plugin when it wires `PLUGIN_<DEP>_INTERNAL_URL`, so the converted twin gains the line. The delta rows for the fixtures document it.

## Proofs that the gate works

`check.sh --fixtures` converts the v1 fixtures, requires the gate to pass on them, then plants each defect on a copy (legacy key, bad enum, compatibility drift, missing healthcheck, `HEALTHCHECK NONE`, disabled healthcheck, out-of-dir replace incl. quoted and `go.work` forms, unapplied migration, unwired migration, `migrate.Apply` only in a comment or a test file, wrong repository, docker socket without an exception incl. in the `service.compose` fragment and other yaml files, unvendored build, expired exception, an exception that raises its own cap, a gate that checks no plugin) and requires the named rule to fail. The `conformance` job in `.github/workflows/validate.yml` runs the gate on changed plugins and all of these proofs on every pull request.
