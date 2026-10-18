# Encryption Plugin

<!-- BEGIN GENERATED:catalog -->
<!-- Generated from catalog.json by scripts/generate-wiki-plugin-page.py. Do not hand edit. -->

| Field | Value |
|---|---|
| **Licence** | Free (MIT) |
| **Maturity** | implemented ([maturity levels](https://github.com/nself-org/cli/wiki/Plugin-Manifest)) |
| **Version** | 1.2.6 |
| **Install** | `nself plugin install encryption` |
| **Commands** | `nself encryption` |
| **Requirements** | nSelf >=1.2.8 |

<!-- END GENERATED:catalog -->

> Bring Your Own Key (BYOK) per-tenant envelope encryption for nSelf Cloud: AWS KMS, GCP Cloud KMS, and HashiCorp Vault Transit, with key rotation and an audit trail.

## Install

```bash
nself plugin install encryption
```

No license key required.

## Description

Bring Your Own Key (BYOK) per-tenant envelope encryption for nSelf Cloud: AWS KMS, GCP Cloud KMS, and HashiCorp Vault Transit, with key rotation and an audit trail.

Category: `compliance`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `BYOK_PLUGIN_URL` | `-` | - |
| `NSELF_API_URL` | `-` | - |
| `NSELF_TENANT_ID` | `-` | - |

## Examples

### Configure

```bash
nself encryption configure
```

### Verify

```bash
nself encryption verify
```

### Rotate

```bash
nself encryption rotate
```

### Status

```bash
nself encryption status
```

## Source

[`plugins/encryption/`](https://github.com/nself-org/plugins/tree/main/encryption)

Manifest: [`plugins/encryption/plugin.json`](https://github.com/nself-org/plugins/tree/main/encryption/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
