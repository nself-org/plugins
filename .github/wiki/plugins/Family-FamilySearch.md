# Family FamilySearch Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | planned ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install family-familysearch` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> FamilySearch → nFamily migration helper (PLANNED).

## Install

```bash
nself plugin install family-familysearch
```

No license key required.

## Description

FamilySearch → nFamily migration helper (PLANNED). Free public API from FamilySearch (LDS). Lowest-friction next-step importer after family-geni.

Category: `social`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |

## Ports

| Port | Purpose |
|------|---------|
| 3510 | Family FamilySearch service port |

## Examples

```bash
nself plugin install family-familysearch
```

## Source

[`plugins/family-familysearch/`](https://github.com/nself-org/plugins/tree/main/family-familysearch)

Manifest: [`plugins/family-familysearch/plugin.json`](https://github.com/nself-org/plugins/tree/main/family-familysearch/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
