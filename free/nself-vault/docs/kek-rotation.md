# nself-vault — KEK Rotation Procedure

**Version:** v1.1.2
**Audience:** operators running nself-vault in production or staging
**Prerequisite:** familiarity with `nself plugin install`, env cascade
(`.env.dev` → `.env.staging` / `.env.prod` → `.env.secrets`), and the
`nself-vault` schema (`SPEC.md` § Schema).

---

## 1. Concepts

nself-vault uses **envelope encryption** with version-numbered KEKs:

- **KEK** (Key Encryption Key) — 32 bytes of random material that wraps the
  per-record DEKs. The KEK never enters the database. It lives in process
  memory, loaded from `VAULT_KEK_V{N}` env vars (preferred) or
  `/etc/nself-vault/keks/v{N}.key` files (fallback).

- **DEK** (Data Encryption Key) — 32 bytes generated per (tenant, alias). The
  DEK encrypts application data. The DEK is stored in `np_vault_keys` as an
  envelope: `(kek_version, nonce, ciphertext)`. The envelope is decrypted
  with the matching KEK to recover the DEK plaintext at use-time.

- **Envelope rotation** — the act of re-encrypting an existing DEK under a new
  KEK version. The underlying DEK plaintext is unchanged, so application data
  encrypted under that DEK remains decryptable without re-encrypting every
  row.

The loader selects KEKs by version:

- **Write path** uses the highest available version (or `VAULT_CURRENT_KEK_VERSION`
  when set explicitly).
- **Read path** accepts ANY configured version, so old envelopes remain
  decryptable during the grace window.

---

## 2. Storage strategy (v1.1.2)

| Source | Location | Notes |
|---|---|---|
| **Env (preferred)** | `VAULT_KEK_V1`, `VAULT_KEK_V2`, ... | One 64-char lowercase hex value per version. Empty values are ignored. |
| **File (fallback)** | `${VAULT_KEK_DIR:-/etc/nself-vault/keks}/v{N}.key` | One file per version. Body may be raw hex or `VAULT_KEK_V{N}=<hex>`. Mode 0600. |
| **Selector** | `VAULT_CURRENT_KEK_VERSION` | Optional. Picks which version encrypts new DEKs. Defaults to the highest available. |

Env wins when both sources name the same version. KMS-backed loaders (AWS KMS,
GCP KMS, HashiCorp Vault Transit) are planned for v1.2 and will follow the
same `Get(version)` interface so handlers do not change.

---

## 3. Generating a KEK

Use the bundled script:

```bash
# Print 32 random bytes as 64-char lowercase hex to stdout.
./scripts/gen-kek.sh

# Print as a ready-to-paste env line.
./scripts/gen-kek.sh --version=2
# → VAULT_KEK_V2=<hex>

# Write to a file (mode 0600, skips if file exists — idempotent).
./scripts/gen-kek.sh --out=/etc/nself-vault/keks/v2.key
```

Entropy preference: `/dev/urandom` first, `openssl rand` second. Never use
`/dev/random` (blocks unnecessarily) or shell `$RANDOM` (non-cryptographic).

**Operational hygiene:**

- Never commit a KEK to git.
- Never log a KEK. Never write it to a shared chat channel.
- Use `umask 077` or `chmod 600` for any file holding KEK material.
- Rotate any KEK that may have been exposed — see § 5.

---

## 4. Initial provisioning

1. Generate KEK V1:

   ```bash
   ./scripts/gen-kek.sh --version=1 >> .env.secrets
   ```

2. Set `VAULT_PLUGIN_ENABLED=true` in `.env.secrets`.

3. (Optional) Set `VAULT_CURRENT_KEK_VERSION=1` explicitly. When omitted, the
   loader selects the highest known version automatically — fine for
   single-version deployments.

4. Restart the vault service:

   ```bash
   nself plugin restart nself-vault
   ```

5. Verify health:

   ```bash
   curl -s http://127.0.0.1:3823/health
   # → {"status":"ok"}
   ```

---

## 5. Rotation flow

A rotation introduces KEK V{N+1} while keeping V{N} available for decrypt.
The DEK envelopes are re-wrapped lazily (on next encrypt call) or eagerly
(via a backfill loop — see § 6). Old KEK V{N} is retired after the grace
window expires.

### 5.1 Steps

1. **Generate the new KEK** on the operator workstation:

   ```bash
   ./scripts/gen-kek.sh --version=2
   # → VAULT_KEK_V2=<hex>
   ```

2. **Deploy the new KEK** to staging first. Add to `.env.secrets`:

   ```env
   VAULT_KEK_V1=<existing>
   VAULT_KEK_V2=<new>
   # Keep V1 — do NOT remove it yet.
   ```

   Do NOT set `VAULT_CURRENT_KEK_VERSION` yet. The loader will continue using
   V1 because V2 has not been promoted.

3. **Restart and verify** the new KEK is loaded:

   ```bash
   nself plugin restart nself-vault
   nself doctor --deep   # check SEC-VAULT-01 / DEP-LIBSODIUM-01
   ```

4. **Promote V2 to current.** Set `VAULT_CURRENT_KEK_VERSION=2` in
   `.env.secrets`, then restart. New envelopes are now wrapped under V2; old
   envelopes (still wrapped under V1) continue to decrypt cleanly because
   `VAULT_KEK_V1` is still configured.

5. **Soak for the grace window.** Default: 7 days. Monitor:

   - `vault_decrypt_errors_total` rate (alert at >1% over 5 min)
   - `vault_envelope_rotation_pending_total` counter (DEKs still on the old
     KEK)

6. **Re-wrap the remaining envelopes** (eager backfill — optional, see § 6).
   Until this completes, do NOT remove V1 from env.

7. **Retire V1.** When zero envelopes reference `kek_version=1`, remove
   `VAULT_KEK_V1` from `.env.secrets`. Restart. Verify
   `nself doctor --deep` reports no V1 references.

### 5.2 Promote to production

After the staging rotation soaks cleanly for 7 days, repeat steps 1–7 against
the production env. Stagger so production never lacks the same KEK
multiplicity that staging just exercised.

---

## 6. Re-wrap (backfill) procedure

Re-wrapping is performed by the `nself-vault rewrap` admin command (planned —
ticket S03-T0X; until then, drive it through SQL + the `ReWrap` helper):

```go
// Conceptual: iterate np_vault_keys WHERE kek_version != current_version,
// call crypto.ReWrap, UPDATE with new envelope bytes + kek_version.
```

Re-wrap is idempotent: calling it on an already-current envelope rewrites it
with a fresh nonce, so a half-completed re-wrap loop is safe to resume.

---

## 7. Failure modes and remediation

| Symptom | Cause | Fix |
|---|---|---|
| Service refuses to start with `ErrNoKEKsConfigured` | No `VAULT_KEK_V*` in env and no key file in `VAULT_KEK_DIR` | Provision V1 — see § 4 |
| Service refuses to start with `ErrInvalidKEKHex` | Hex is wrong length or contains non-hex chars | Re-run `gen-kek.sh`; never hand-edit |
| Decrypt fails with `ErrKEKVersionNotFound: V{N}` | A DEK envelope references KEK V{N} but V{N} is not in env/files | Restore the missing KEK — recover from backup or retire the affected DEK |
| `nself doctor --deep SEC-VAULT-02` fails | `VAULT_CURRENT_KEK_VERSION` names a version not configured | Either set the var to a configured version, or unset it (auto-pick highest) |

**Never** “fix” `ErrKEKVersionNotFound` by deleting the affected envelope rows
— that destroys the DEK and renders all application data encrypted under it
unrecoverable. Always recover the KEK first.

---

## 8. KMS path (v1.2 preview)

The KeyRing interface (`Get(version)` / `CurrentKey()`) is provider-neutral.
v1.2 will add three loaders selectable via `VAULT_KEK_SOURCE`:

- `env` (default — current behavior)
- `file` (current fallback, promoted to primary in air-gapped deploys)
- `awskms` / `gcpkms` / `vaulttransit` — the KEK never leaves the KMS;
  envelopes are unwrapped by calling KMS `Decrypt`. KEK rotation is delegated
  to the KMS; nself-vault only tracks which version was used per envelope.

See `.claude/memory/decisions.md` § KEK Storage v1.2 (planned).

---

## 9. References

- `SPEC.md` § Storage & Architecture (KEK / DEK model)
- `internal/crypto/kek.go` (loader)
- `internal/crypto/envelope.go` (wrap / unwrap / re-wrap)
- `internal/crypto/kek_test.go` and `envelope_test.go` (regression suite)
- `scripts/gen-kek.sh` (entropy-clean key generation)
- `cli/internal/auth/jwt_rotation.go` (reference rotation pattern with dual-key
  grace window)
