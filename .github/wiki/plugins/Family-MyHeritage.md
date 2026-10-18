# Family MyHeritage Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | planned ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install family-myheritage` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> MyHeritage → nFamily migration helper (PLANNED).

## Install

```bash
nself plugin install family-myheritage
```

No license key required.

## Description

MyHeritage → nFamily migration helper (PLANNED). MyHeritage is Geni.com's parent company, lowest legal risk, potential partnership path.

Category: `social`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |

## Ports

| Port | Purpose |
|------|---------|
| 3511 | Family MyHeritage service port |

## Examples

```bash
nself plugin install family-myheritage
```

## Source

[`plugins/family-myheritage/`](https://github.com/nself-org/plugins/tree/main/family-myheritage)

Manifest: [`plugins/family-myheritage/plugin.json`](https://github.com/nself-org/plugins/tree/main/family-myheritage/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
