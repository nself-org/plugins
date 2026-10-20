# BYOK Plugin

> Bring Your Own Key per-tenant encryption with customer-managed keys (CMK).

**Tier:** Free (MIT) — no license required.

BYOK gives each tenant control of its own encryption key. Application data is sealed with AES-256-GCM, and the data key is wrapped by a key the customer owns in AWS KMS, GCP Cloud KMS, or HashiCorp Vault Transit. nSelf never holds the unwrappable root key, which satisfies HIPAA, FedRAMP High, FFIEC, and DORA key-control requirements.

## Install

```bash
nself plugin install byok
```

The plugin listens on port **3743** (per F10-PORT-REGISTRY; paypal uses 3741).

## Envelope Encryption Scheme

BYOK uses two-layer envelope encryption:

1. **DEK (Data Encryption Key)** — a fresh 256-bit key generated per record group. Plaintext is sealed with AES-256-GCM using a 12-byte random nonce (no nonce reuse; each encryption produces a distinct IV and ciphertext). The plaintext DEK is zeroed in memory immediately after use.
2. **CMK (Customer Master Key)** — the tenant's key inside their own KMS. The DEK is wrapped (`WrapKey`) by the CMK and stored alongside the ciphertext in `np_encrypted_values`. To read a record, the wrapped DEK is unwrapped (`UnwrapKey`) by the CMK at request time.

Each stored bundle records the `KMSKeyRef` of the CMK that wrapped its DEK, so revocation and audit can be scoped per tenant. If a CMK is revoked or disabled, `UnwrapKey` returns an error and the plugin surfaces a decryption failure — it never falls back to a cached plaintext DEK.

## Tenant Isolation

A record encrypted under tenant A's CMK cannot be read with tenant B's CMK: B's KMS unwraps to a different DEK and AES-256-GCM authentication rejects it. Wrapped DEKs differ across tenants even for identical plaintext, so a leaked wrapped DEK from one tenant is useless against another. See `go/crypto/tenant_isolation_test.go`.

## KMS Setup

### AWS KMS (CMK)

1. Create a symmetric CMK in your AWS account.
2. Grant the nSelf workload IAM principal the minimum policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["kms:Encrypt", "kms:Decrypt", "kms:DescribeKey"],
      "Resource": "arn:aws:kms:<region>:<account>:key/<key-id>"
    }
  ]
}
```

3. Register the key reference in `np_kms_configs` with `provider = 'aws'` and `key_ref = '<CMK ARN>'`.

`kms:Encrypt` wraps DEKs, `kms:Decrypt` unwraps them, and `kms:DescribeKey` reads key state. GCP Cloud KMS and Vault Transit are configured the same way via `provider = 'gcp'` / `'vault'`.

## Key Rotation

Rotation re-encrypts every record for a tenant under fresh DEKs while reusing the existing CMK (`crypto/rotate.go`). To rotate the CMK itself, rotate it in your KMS, update `np_kms_configs.key_ref`, then run the rotation job. Rotation progress is tracked per job in `np_encryption_key_events`.

## Feature Flags

| Flag | Default | Description |
|------|---------|-------------|
| `NSELF_BYOK` | `false` | Enable BYOK customer-managed key encryption. |
| `NSELF_BYOK_DEK_CACHE` | `false` | Cache decrypted DEKs in memory to reduce KMS API calls. |
| `NSELF_BYOK_MULTI_REGION` | `false` | Enable multi-region KMS replication support. |

## Tables

- `np_kms_configs` — per-tenant KMS provider and CMK reference.
- `np_encrypted_values` — ciphertext, wrapped DEK, IV, and CMK reference per record.
- `np_encryption_key_events` — rotation and key-state audit trail.

## Testing

```bash
cd go && go test ./...
```

Tests cover envelope round-trip, IV uniqueness, revoked-key error handling, and tenant isolation.

## Database migrations

The plugin applies `migrations/*.sql` at boot through the nSelf migration helper (ledger `np_byok.schema_migrations`); the tables live in `public`. The columns `tenant_id` and `triggered_by` no longer carry foreign keys to `np_tenants` and `np_users`: those core tables do not exist on a fresh database, and the tenant wall is enforced by the `tenant_id` filters and the row-level-security policies. RLS is forced. Every handler (and the key rotator) runs its statements in a transaction that first sets `app.current_tenant_id` to the requesting tenant (`tenantdb.With`), so the plugin works with an ordinary role as well as a superuser or `BYPASSRLS` role. Every `/api/v1` route needs the tenant in `X-Tenant-ID`, including `GET /encryption/rotate/{job_id}`. Migration `005` makes an empty setting match no rows instead of raising, adds the `iv` and `event_type` CHECKs on databases created by the old inline schema (NOT VALID), and drops the unnamed tenant foreign keys that duplicate the named ones from `004`.
