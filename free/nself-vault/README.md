# nself-vault Plugin

Per-device envelope encryption for nSelf. Each registered device holds its own encrypted copy of a credential's key material. Only the owning device can decrypt its envelope.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install nself-vault
nself build && nself start
```

## Schema

```
np_vault_devices     — registered devices (pubkey, label, platform)
np_vault_records     — logical credential entries (kind, label)
np_vault_envelopes   — per-device ciphertext (one row per record+device pair)
np_vault_audit       — immutable audit log (insert-only)
```

All tables include `tenant_id UUID` nullable for Cloud multi-tenancy.

## Endpoints

Service binds `127.0.0.1:3823` (proxied through nginx).

```
POST   /vault/v1/devices              Register a new device
GET    /vault/v1/devices              List user's active devices
DELETE /vault/v1/devices/{id}         Revoke a device

POST   /vault/v1/records              Create a record + initial envelope
GET    /vault/v1/records?since=<RFC3339>  List records (optional cursor)
GET    /vault/v1/records/{id}/envelope?device_id=<uuid>  Fetch device envelope
POST   /vault/v1/records/{id}/envelopes  Add envelope for a new device
DELETE /vault/v1/records/{id}         Soft-delete record + cascade wipe envelopes

GET    /health                        Liveness check
```

All `/vault/v1/*` routes require `Authorization: Bearer <nself-jwt>`.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | yes | — | Postgres connection string |
| `VAULT_KEK_V1` | yes | — | 32-byte hex KEK for envelope wrapping |
| `NSELF_JWT_SECRET` | yes | — | HMAC secret for JWT validation |
| `VAULT_PLUGIN_ENABLED` | no | `false` | Must be `true` to start service |
| `VAULT_BIND` | no | `127.0.0.1` | Bind address |
| `VAULT_PORT` | no | `3823` | Listen port |
| `VAULT_KEK_V2` | no | — | Secondary KEK for rotation |
| `VAULT_CURRENT_KEK_VERSION` | no | `1` | Active KEK version |

## Security Notes

- `envelope_ciphertext` is never logged by the service (enforced at handler layer).
- Audit log table has `REVOKE UPDATE, DELETE` — entries are immutable.
- Service makes zero outbound network calls (only connects to Postgres).
- Binds `127.0.0.1` only — external access via nginx proxy.

## License

MIT license — no key required.
