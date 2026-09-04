# Plugin Registry

The nSelf plugin registry (`registry.json` at the repository root) is the machine-readable
index of every plugin the CLI, Admin UI, and `plugins.nself.org` marketplace worker read
from. It is validated on every push and PR by `.github/workflows/registry.yml` and
`shared/validate-registry.sh`, and its shape is defined by the JSON Schema at
`registry-schema.json`.

## Top-level shape

```json
{
  "schema_version": "2.0.0",
  "generated_at": "2026-09-02T00:00:00Z",
  "plugins_count": 129,
  "last_updated": "2026-09-02",
  "tier": "free",
  "checksum_algorithm": "sha256",
  "note": "...",
  "plugins": {
    "ai-cli": { "...": "..." },
    "audit-analytics": { "...": "..." }
  }
}
```

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
| `dependencies` | string[] | Other plugin names this plugin depends on |
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

- [[Plugin-Marketplace]]: the API surface that serves this registry to the CLI, Admin UI, and cloud console
- [[Plugin-Development]]: building a plugin whose `plugin.json` feeds this registry
- [[Home]]
