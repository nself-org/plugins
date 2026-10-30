# Family Ancestry Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | planned ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install family-ancestry` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> Ancestry.com → nFamily migration helper (PLANNED).

## Install

```bash
nself plugin install family-ancestry
```

No license key required.

## Description

Ancestry.com → nFamily migration helper (PLANNED). Imports profiles, photos, documents, sources into the family plugin. Pattern mirrors family-geni.

Category: `social`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |

## Ports

| Port | Purpose |
|------|---------|
| 3513 | Family Ancestry service port |

## Examples

```bash
nself plugin install family-ancestry
```

## Source

[`plugins/family-ancestry/`](https://github.com/nself-org/plugins/tree/main/family-ancestry)

Manifest: [`plugins/family-ancestry/plugin.json`](https://github.com/nself-org/plugins/tree/main/family-ancestry/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
