# Plugin Registry

The nSelf plugin registry (`registry.json` at the repository root) is the machine-readable
index of every free plugin the CLI, Admin UI, and `plugins.nself.org` marketplace worker read
from. **It is generated; nobody edits it.** `scripts/plugin-counts.sh` runs the pinned cli
`tools/catalog` over `free/*/plugin.json`, `releases.json` and the licensed bundles data, and
writes `registry.json`, `catalog.json` and `counts.json` together (see [[Catalog]]).
A drift gate (`plugin-counts.yml`, `registry-check.yml`) fails when a committed file differs from
what the generator produces. Its shape is defined by the JSON Schema at `registry-schema.json`
and checked by `shared/validate-registry.sh`.

## Worker endpoints

`plugins.nself.org` is the Cloudflare worker in `.workers/plugins-registry/`. It has one source,
`src/index.ts` (plus the modules it imports); `wrangler.toml` deploys `main = "src/index.ts"`.
Every path also answers under a `/v1` prefix. Tests run with `pnpm test` there: it bundles
`src/index.ts` with esbuild and runs `node --test` against the bundle.

| Route | What it serves |
|---|---|
| `GET /catalog.json` | The generated catalog, compact JSON, see below |
| `GET /registry.json`, `/registry`, `/` | Combined free + licensed registry (`?tier=free\|pro`, `?nocache=1`) |
| `GET /plugins`, `/registry/list` | Plugin list (`?tier`, `?category`) |
| `GET /plugins/:name`, `/plugins/:name/:version` | One plugin |
| `GET /plugins/:name/tarball` | 302 to the free tarball (R2 first, GitHub Releases fallback). **401 for every tier other than `free`** |
| `GET /plugins/:name/signature` | Ed25519 signature metadata |
| `GET /plugins/revocations` | Revoked plugin versions (`revoked`; `revocations` is the same list under the old key) |
| `GET /.well-known/revoked-authors.json` | Revoked author keys |
| `GET /categories`, `/manifest.json`, `/stats`, `/health` | Categories, flat manifest, counters, health |
| `GET /bundles.json`, `/bundles-schema.json` | Bundle membership map and its schema (`X-Cache` HIT or MISS) |
| `GET /marketplace`, `/marketplace/ratings/:name` | Marketplace view and ratings |
| `POST /marketplace/ratings/:name`, `/plugins/:name/install-event` | Submit a rating, count an install |
| `POST /api/sync` | Bearer `GITHUB_SYNC_TOKEN`; drops every KV cache key (including `catalog:json`) and refetches |

### `/catalog.json`

The worker reads `catalog.json` from the plugins repo (GitHub Contents API with the token, else the raw URL),
serialises it compactly (no indentation), and keeps the body and its hash together in KV (`catalog:json`, fresh for
`CACHE_TTL`, 300 s).

- `ETag: "<sha256 hex of the exact body bytes>"`: a strong tag. The same catalog gives the same tag from either source; any change to the catalog changes it.
- `If-None-Match` with that tag (also a list of tags, a `W/` form, or `*`) returns `304` with an empty body and the same `ETag`.
- `Cache-Control: public, max-age=300`, `Content-Type: application/json`, `X-Cache: HIT | MISS | STALE`.
- If GitHub cannot be reached and an expired copy exists, the worker serves it as `STALE` (`max-age=60`); with no copy at all it returns `502`.
- A fetched body that is not a catalog (not an object with a `plugins` collection) is never cached or served.

### Licensed tarballs stay behind the licence gate

`/plugins/:name/tarball` hands out a redirect only when the registry entry's tier is exactly `free`. `pro`, `max`, an unknown or
future tier, an odd spelling (`FREE`, `free `) and a missing tier on a licensed registry all get `401` with a pointer to
`ping.nself.org/plugins/:name/download`, before the revocation check runs. The route tests cover encoded slashes and dots,
dot segments, case changes, version aliases, doubled slashes, `/v1` prefixes, other methods and hostile headers.

## Changing the registry

| You want to | Do this |
|---|---|
| Change a description, tag, dependency or port | Edit the plugin's `plugin.json`, then `bash scripts/plugin-counts.sh` and commit the three files |
| Publish a release | Upload the assets, then `scripts/verify-published-checksums.sh --write <tag>` (writes `releases.json` from the published bytes), then `bash scripts/plugin-counts.sh` |
| Check for drift | `bash scripts/plugin-counts.sh --check` (needs a bundles clone; CI without the token runs `--check --free-only`) |

The generator does not write `generated_at`, `last_updated` or a `note`; nothing in this repo or the marketplace worker reads them.
Released CLIs install every listed dependency edge, optional ones included, and fail the install on a name they cannot fetch
or on a licensed plugin without a licence. So `registry.json` lists only the required edges whose target is a free registry
entry. Every other edge (optional, licensed-tier, unreleased such as `minio`, or a core service such as `database`) stays in the
plugin's `plugin.json` and is left off the wire; `plugin-counts.sh` names each one on stderr.

A plugin with no row in `releases.json` has no published bytes and is left out of `registry.json`
until the release pipeline adds a row. Checksums, tarball URLs and per-platform checksums live
in `releases.json` only.

## Top-level shape

```json
{
  "schema_version": "2.0.0",
  "_generated": "GENERATED BY tools/catalog — DO NOT HAND EDIT",
  "generated_from": { "bundles_ref": "<scripts/bundles.ref>", "generated_at": "1970-01-01T00:00:00Z", "plugins_ref": "" },
  "plugins_count": 129,
  "tier": "free",
  "checksum_algorithm": "sha256",
  "plugins": {
    "ai-cli": { "...": "..." },
    "audit-analytics": { "...": "..." }
  }
}
```

The generated file carries no wall-clock time and no hand-written note, so it is byte-identical for identical inputs.

`plugins` is a keyed object (aggregated v2 format) mapping plugin name to its entry.
A flat array of plugin objects is also accepted for backwards compatibility
(`shared/validate-registry.sh` handles both).

## Plugin entry fields

Each entry under `plugins` follows the `plugin` definition in `registry-schema.json`.
Only `version` and `description` are strictly required; the fields actually populated
by this repo's plugins include:

| Field | Type | Description |
|---|---|---|
| `name` | string | Plugin identifier, lowercase alphanumeric with hyphens |
| `version` | string | Semantic version |
| `description` | string | Short description. Must match the plugin's own `plugin.json`, with no license/tier claims that contradict `tier`/`requires_license` |
| `category` | string | Category identifier (see Categories below) |
| `tier` | string | `free` or `pro`. Must match the plugin's directory (`free/` or a `plugins-pro` paid tier) |
| `license` | string | License identifier (`MIT` for every free plugin) |
| `requires_license` | boolean | Whether a paid nSelf license is required to run the plugin |
| `language` | string | Implementation language (`go`, `typescript`, etc.) |
| `min_nself_version` / `minNselfVersion` | string | Minimum required nself CLI version (snake_case canonical, camelCase alias accepted) |
| `tarball` / `download_url` | string (URI) | Release tarball and registry-served download URL |
| `tags` | string[] | Searchable tags |
| `dependencies` | string[] or object | Other plugin names this plugin depends on; `{required, optional}` when the manifest declares optional dependencies |
| `tables` | string[] | `np_*` Postgres tables the plugin creates |
| `implementation` | object | Runtime details: language, entry point, plugin type, binary name |
| `envVars` | object | Required/optional environment variables |
| `bundles` | string[] | Named bundles this plugin ships in, if any |

## Categories

The optional top-level `categories` object maps a category id to `{ name, description, icon }`,
used by the Admin UI and marketplace worker to group plugins for display.

## Validation

Run the validator locally before opening a PR that touches `registry.json` or any
`free/*/plugin.json`:

```bash
bash shared/validate-registry.sh
```

It runs 13 canonical checks (tier/directory consistency, required fields, plugin name
format, semver, port uniqueness, duplicate names, alphabetical order, bundle membership,
and registry-vs-filesystem consistency) and exits non-zero on any error.

## See Also

- [[Catalog]]: the generated `catalog.json` and `counts.json`, the inputs and the pins
- [[Plugin-Marketplace]]: the API surface that serves this registry to the CLI, Admin UI, and cloud console
- [[Plugin-Development]]: building a plugin whose `plugin.json` feeds this registry
- [[Home]]
