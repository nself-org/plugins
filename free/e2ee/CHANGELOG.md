# Changelog — nself-e2ee

## Unreleased

Security fixes (CR-C BLOCK — authorization layer).

- **Gateway-trust authentication enforced.** All key-directory routes now
  require the gateway-forwarded authenticated principal (`X-Hasura-User-Id` +
  source account); requests missing it fail closed with 401 (`RequireAuth`
  middleware).
- **Ownership enforced on writes.** Identity registration, signed-prekey upload,
  one-time-prekey upload, and safety-number posting now reject (403) any attempt
  to write keys for a user other than the authenticated principal — closes the
  E2EE-defeating key-overwrite MITM.
- **Bundle requester is authenticated.** `GetPreKeyBundle` derives the requester
  from the principal, not a query param (no audit forgery).
- **RLS activated at runtime.** Every handler runs its DB work in a transaction
  that sets `app.source_account_id` + `app.current_user_id` GUCs, so the
  existing row-level-security policies finally enforce isolation. New migration
  `002_e2ee_rls_consume.sql` adds the policies needed for cross-user prekey
  consumption + served-bundle audit under RLS.
- **Generic client errors.** Internal errors are logged server-side and return a
  generic message (no `err.Error()` leak).
- Removed the unused `identity_key_public` field from the one-time-prekey upload
  request (the stored identity key is always used to verify Kyber signatures).
- README: documented the gateway trust boundary; port 3055 must be gateway-only.

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
