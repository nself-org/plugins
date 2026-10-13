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

Exit codes: 0 ok, 1 drift or expired carry, 2 usage or generator failure, 3 precondition missing (no clone, no pinned commit).

## Counts

`counts.json` keeps its shape (`free`, `pro`, `overlap`, `totals`, `advertised`). `generated_from` replaces the old
`generated_at` and `sources` keys. `cron` and `notify` exist in both tiers: each appears once per tier in
`catalog.json` and counts once.

## See Also

- [[Plugin-Registry]]: the generated registry and how to change it
- [[Releasing]]
- [[Home]]
