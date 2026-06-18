# Changelog — nself-e2ee

## 1.0.0

Initial release. E2EE key directory for nchat.

- X3DH prekey distribution: identity keys, signed prekeys, one-time prekeys.
- Kyber-1024 (ML-KEM-1024) post-quantum one-time prekeys via `github.com/cloudflare/circl`.
- Atomic one-time / Kyber prekey consumption on bundle fetch (no replay window).
- Ed25519 signature verification of signed prekeys and Kyber prekeys on upload.
- Safety-number + per-peer verification state storage.
- Append-only audit log (`np_e2ee_audit_log`).
- Multi-app isolation via `source_account_id` + FORCE row-level security on every table.
- Free tier per the Security-Always-Free Doctrine.
