# E2EE Plugin

> Public-key directory for end-to-end encryption: X3DH prekeys plus post-quantum Kyber-1024 prekeys. **Free — MIT licensed.**

## Install

```bash
nself plugin install e2ee
```

No license key required.

## Description

End-to-end encryption key directory: X3DH prekey distribution + Kyber-1024 (ML-KEM-1024) post-quantum prekeys for nchat. Server stores PUBLIC keys only; private keys never leave the client.

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `authentication`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | *(required)* | Required. |
| `E2EE_PLUGIN_PORT` | *(see plugin.json)* | Optional. |
| `E2EE_PLUGIN_HOST` | *(see plugin.json)* | Optional. |
| `E2EE_LOG_LEVEL` | *(see plugin.json)* | Optional. |
| `E2EE_MAX_ONE_TIME_PREKEYS` | — | Optional. |
| `E2EE_MAX_KYBER_PREKEYS` | — | Optional. |

## Ports

| Port | Purpose |
|------|---------|
| 3055 | E2EE service |

## Database Schema

8 table(s) added to your Postgres database (prefix: `np_e2ee_`):

- `np_e2ee_identity_keys`
- `np_e2ee_signed_prekeys`
- `np_e2ee_one_time_prekeys`
- `np_e2ee_kyber_prekeys`
- `np_e2ee_prekey_bundles_served`
- `np_e2ee_verification_states`
- `np_e2ee_safety_numbers`
- `np_e2ee_audit_log`

## REST API

```
POST /keys/identity       — Publish an identity key
POST /keys/prekeys        — Upload signed + one-time + Kyber prekeys
GET  /keys/bundle/{user}  — Fetch a prekey bundle for a user
GET  /safety-number/{a}/{b} — Compute a safety-number verification code
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/e2ee/` | E2EE key-directory REST API |

## Examples

### Check health

```bash
curl http://localhost:3055/health
```

## Source

[`plugins/e2ee/`](https://github.com/nself-org/plugins/tree/main/e2ee)

Manifest: [`plugins/e2ee/plugin.json`](https://github.com/nself-org/plugins/tree/main/e2ee/plugin.json)

## See Also

- [[Notify]] — multi-channel notification service
- [[Push]] — APNs + FCM push relay

← [[Home]] →
