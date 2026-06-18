# nself-e2ee

End-to-end encryption **key directory** for nchat. Provides X3DH prekey
distribution plus **Kyber-1024 (ML-KEM-1024)** post-quantum prekeys for the
PQXDH-style handshake.

> **Free plugin.** Per the nSelf Security-Always-Free Doctrine, end-to-end
> encryption is core security and ships free (`plugins/free/e2ee/`).

## Security model

This server is a **key directory only**. It stores **PUBLIC** key material and
distributes prekey bundles. It does **not**:

- store, receive, or transmit any **private** key,
- perform KEM **encapsulation** or **decapsulation** (that happens client-side),
- ever see a user passphrase or master key.

The X3DH + Kyber handshake runs entirely on the clients. The server's job is to
hand the initiator a bundle of the recipient's public keys and to consume
one-time prekeys exactly once.

### Cryptographic dependencies

| Concern | Library | Notes |
|---|---|---|
| ML-KEM-1024 public-key validation | `github.com/cloudflare/circl/kem/mlkem/mlkem1024` | FIPS 203; vetted, audited. No KEM math hand-rolled. |
| Ed25519 signature verification | `crypto/ed25519` (stdlib) | Verifies signed prekeys + Kyber prekey signatures. |

## Port

`3055` (registered in SPORT F10-PORT-REGISTRY, free-plugin block).

## Endpoints

| Method | Path | Purpose |
|---|---|---|
| GET  | `/health` | Liveness. |
| GET  | `/ready` | DB readiness. |
| POST | `/api/v1/e2ee/identity/register` | Publish a device PUBLIC identity key. |
| POST | `/api/v1/e2ee/signed-prekey` | Upload a signed prekey (signature-verified). |
| POST | `/api/v1/e2ee/one-time-prekeys` | Batch-upload classic + Kyber one-time prekeys. |
| GET  | `/api/v1/e2ee/bundle/{userId}?device_id=&requested_by=` | Fetch a prekey bundle; consumes one OTPK + one Kyber prekey atomically. |
| GET  | `/api/v1/e2ee/replenish/{userId}?device_id=` | Remaining unconsumed prekey counts. |
| POST | `/api/v1/e2ee/safety-number` | Post a computed safety number + verification flag. |
| GET  | `/api/v1/e2ee/verification/{userId}/{peerId}` | Per-peer verification state. |
| GET  | `/api/v1/e2ee/audit/{userId}` | Recent append-only audit entries. |

All key fields on the wire are base64-encoded **public** bytes.

## Tables

`np_e2ee_identity_keys`, `np_e2ee_signed_prekeys`, `np_e2ee_one_time_prekeys`,
`np_e2ee_kyber_prekeys`, `np_e2ee_prekey_bundles_served`,
`np_e2ee_verification_states`, `np_e2ee_safety_numbers`, `np_e2ee_audit_log`.

Every table carries `source_account_id TEXT NOT NULL DEFAULT 'primary'`
(Multi-App Isolation, Convention A) with `FORCE ROW LEVEL SECURITY`.

### nchat table-name mapping

| nchat frontend name | this plugin |
|---|---|
| `nchat_identity_keys` | `np_e2ee_identity_keys` |
| `nchat_signed_prekeys` | `np_e2ee_signed_prekeys` |
| `nchat_one_time_prekeys` | `np_e2ee_one_time_prekeys` |
| `nchat_kyber_prekeys` | `np_e2ee_kyber_prekeys` |
| `nchat_prekey_bundles` | `np_e2ee_prekey_bundles_served` |
| `nchat_verification_states` | `np_e2ee_verification_states` |
| `nchat_safety_numbers` | `np_e2ee_safety_numbers` |
| `nchat_e2ee_audit_log` | `np_e2ee_audit_log` |

## Development

```sh
go build ./...
go test ./...
```

Migrations live in `migrations/001_e2ee_init.sql` (+ `.down.sql`).
