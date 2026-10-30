# nSelf Vault Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.13 |
| **Install** | `nself plugin install nself-vault` |
| **Requirements** | nSelf >=1.0.0 |

<!-- END GENERATED:catalog -->

> nSelf-managed envelope encryption KMS.

## Install

```bash
nself plugin install nself-vault
```

No license key required.

## Description

nSelf-managed envelope encryption KMS. Provides per-row/column selective encryption with key rotation, audit logging, and Hasura Action surface. Eliminates ad-hoc per-team AES wrappers.

Category: `security`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `VAULT_KEK_V1` | `(required)` | - |
| `VAULT_KEK_V2` | `-` | - |
| `VAULT_CURRENT_KEK_VERSION` | `1` | - |
| `VAULT_ENCRYPT_RATE_RPM` | `500` | - |
| `VAULT_DECRYPT_RATE_RPM` | `1000` | - |
| `VAULT_AUDIT_RETENTION_DAYS` | `365` | - |
| `VAULT_PLUGIN_ENABLED` | `false` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3823 | nSelf Vault service port |

## Examples

```bash
nself plugin install nself-vault
```

## Source

[`plugins/nself-vault/`](https://github.com/nself-org/plugins/tree/main/nself-vault)

Manifest: [`plugins/nself-vault/plugin.json`](https://github.com/nself-org/plugins/tree/main/nself-vault/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
