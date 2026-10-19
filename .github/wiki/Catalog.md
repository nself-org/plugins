# Catalog

`catalog.json` is the one list of every plugin across both license tiers, with its bundles and counts
(contract `plugins.catalog` v1). The marketplace worker, the CLI client and the website read it instead of
joining two registries themselves. It is generated together with `registry.json` and `counts.json`; nobody
edits any of the three.

## What is generated

| File | Content | Read by |
|---|---|---|
| `registry.json` | free plugins, the registry released CLIs decode (key names and types unchanged) | CLI, marketplace worker |
| `catalog.json` | one row per slug and tier (free and licensed), the bundle map, embedded counts | worker, CLI, web |
| `counts.json` | the advertised count and the free / licensed / overlap split | `nself plugin count`, docs |
| `releases.json` | version, sha256, tarball URL, per-platform checksums and signature per plugin | the generator only |

`releases.json` is release data. It is written only by `scripts/verify-published-checksums.sh --write <tag>`
from the bytes published on the GitHub release, never by hand. A plugin without a row is left out of the
registry until its release is published.

## Inputs and pins

| Input | Where | Pinned by |
|---|---|---|
| Free manifests | `free/*/plugin.json` | this repo |
| Release data | `releases.json` | this repo |
| Generator (`tools/catalog`) | a cli commit | `scripts/cli-tools.version` |
| Licensed registry and `bundles.json` | `nself-org/bundles` | `scripts/bundles.ref` |

The generator is built from the pinned cli commit's vendor tree (`go run github.com/nself-org/cli/tools/catalog@<ref>`
cannot work while cli's `go.mod` has replace directives). `catalog.json` records the bundles pin as
`generated_from.bundles_ref`. The catalog lags the licensed repo by that pin; bump `scripts/bundles.ref` to take
newer licensed data.

Two licensed entries (`auth`, `nself-audit`) are carried by bundles without a catalog block. The catalog block for them
comes from `scripts/catalog/licensed-catalog-carry.json` until its expiry date (2026-12-31); after that
`plugin-counts.sh` fails, so the carry cannot outlive its owners.

## Regenerate and check

```bash
bash scripts/plugin-counts.sh                    # rewrite registry.json, catalog.json, counts.json
bash scripts/plugin-counts.sh --check            # write nothing; exit 1 and print the diff when stale
bash scripts/plugin-counts.sh --check --free-only  # CI without the bundles token: registry.json only
```

`plugin-counts.sh` reads the licensed side from a clone of `nself-org/bundles` (`BUNDLES_DIR`, default `../bundles`,
`../plugins-pro`, or under `$NSELF`) at the pinned commit. In CI the workflow checks bundles out at that commit when
`PLUGINS_PRO_CHECKOUT_TOKEN` exists (Mode A, all three files verified). Without the token (Mode B) only `registry.json` is
verified, with the bundle membership and licensed slugs read back from the committed `catalog.json`; the job log says
catalog and counts were not verified.

`bash scripts/plugin-counts.sh --json` prints the generated `counts.json` on stdout and writes nothing (the read-only use the cli shim relies on).

Exit codes: 0 ok, 1 drift or expired carry, 2 usage or generator failure, 3 precondition missing (no clone, no pinned commit).

## Freshness drill

A nightly drill proves that what `plugins.nself.org` serves is what main holds. It is `scripts/drill/catalog-freshness/run`,
a `kind: drill` job of the `drills` pipeline in `.nself/ci.yaml` (found by directory, so the pipeline file does not
change), run every night by `.github/workflows/drills-nightly.yml` as `nself ci --pipeline drills`. The worker KV once sat
stale for months without anyone knowing (D-0051); a silent job needs a staleness alert.

| Check | Passes when |
|---|---|
| `catalog.etag` | the `ETag` of `/catalog.json` equals the sha256 of the compact `catalog.json` on main |
| `catalog.body` | the sha256 of the served bytes equals that same digest |
| `registry.free` | the free plugins and versions in `/registry.json` equal `registry.json` on main |
| `registry.licensed` | the licensed plugin names in `/registry.json` equal the licensed slugs of `catalog.json` on main |
| `bundles.members` | the bundles and their members in `/bundles.json` equal the `bundles` map of `catalog.json` on main |
| `stats.lastSync` | `/stats` carries a `lastSync` under 36 hours old (`MAX_SYNC_AGE_HOURS`) |

"Main" means the committed `catalog.json` and `registry.json`; the Plugin Counts workflow already proves they equal the
generator output on every push and nightly (`CATALOG_REGEN=1` also runs `scripts/plugin-counts.sh --check` inside the
drill). A fetch that fails, or a response that cannot be read, fails its check: nothing is skipped silently. The drill
prints one `ci.evidence` document and exits 1 on any failed check, so it stays red until the worker serves current data.

The pipeline's `max_age` is 36 hours. On a machine that keeps a run store, `nself ci evidence --pipeline drills`
reports the job `overdue` (exit 10) when its last passing run is older than that. A GitHub runner keeps no store, so
there a failed or missing nightly run is the alert.

```bash
bash scripts/drill/catalog-freshness/run --fixtures   # offline self-test: eleven recorded cases, no network
bash scripts/drill/catalog-freshness/run              # the live drill against plugins.nself.org
```

## Counts

`counts.json` keeps its shape (`free`, `pro`, `overlap`, `totals`, `advertised`). `generated_from` replaces the old
`generated_at` and `sources` keys. `cron` and `notify` exist in both tiers: each appears once per tier in
`catalog.json` and counts once.

## See Also

- [[Plugin-Registry]]: the generated registry and how to change it
- [[Releasing]]
- [[Home]]
