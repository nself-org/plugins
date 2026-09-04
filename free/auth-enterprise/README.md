# ɳSelf Auth Enterprise Plugin

MFA enforcement (TOTP + WebAuthn policy) and SSO via SAML 2.0 and OIDC for Google Workspace, Okta, and Microsoft Entra ID. Port 3826. **Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install auth-enterprise
```

## What It Does

Adds enterprise-grade authentication controls to your nSelf deployment. MFA enforcement is always active and free for all users per the Security-Always-Free doctrine. SSO requires `NSELF_SSO=true`.

Key features:
- **TOTP** — TOTP enrollment, verification, and 30-second window challenge (RFC 6238, 1-step drift tolerance).
- **Recovery codes** — bcrypt-hashed single-use codes; regeneratable on demand.
- **MFA policy** — per-tenant enforcement policy (optional, required, role-gated).
- **SAML 2.0** — SP-initiated SSO with Okta, Google Workspace, Microsoft Entra ID, and other SAML 2.0 IdPs.
- **OIDC** — Authorization code flow for Google Workspace and any OIDC-compliant IdP.
- **WebAuthn** — policy-level enforcement (hardware key enrollment in roadmap).

MFA is always enabled and free. SSO is gated by `NSELF_SSO=true`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_SSO` | `false` | Enable SSO endpoints (SAML 2.0 + OIDC) |
| `AUTH_ENTERPRISE_TOTP_ISSUER` | `nSelf` | TOTP issuer name shown in authenticator apps |
| `AUTH_ENTERPRISE_SSO_SP_ENTITY_ID` | — | SAML SP entity ID (URI). Required when SSO + SAML configured |
| `AUTH_ENTERPRISE_SSO_ACS_URL` | — | SAML Assertion Consumer Service URL |
| `AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL` | — | OIDC redirect URI registered with your IdP |

## Database Tables

| Table | Purpose |
|-------|---------|
| `np_mfa_enrollments` | TOTP and WebAuthn enrollment records |
| `np_mfa_recovery_codes` | Bcrypt-hashed single-use recovery codes |
| `np_mfa_policies` | Per-tenant MFA enforcement policy |
| `np_sso_providers` | SAML/OIDC identity provider configurations |
| `np_sso_sessions` | Active SSO session records |
| `np_sso_state_cache` | OIDC/SAML state nonce storage |

All tables include `source_account_id` for multi-app isolation.

## Per-Provider Setup

### Google Workspace (OIDC)
1. Create OAuth2 credentials in Google Cloud Console.
2. Set authorized redirect URI to `AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL`.
3. Add provider via `POST /auth/sso/providers` with `type: "oidc"`.

### Okta (SAML 2.0)
1. Create a SAML app in Okta. Set the ACS URL and SP entity ID.
2. Download the IdP metadata XML.
3. Add provider via `POST /auth/sso/providers` with `type: "saml"` and the metadata XML.

### Microsoft Entra ID (SAML 2.0 or OIDC)
1. Register the application in Entra ID (Azure AD).
2. For SAML: configure the identifier (entity ID) and reply URL (ACS URL).
3. For OIDC: set the redirect URI and copy the client ID + secret.

## Security Notes

- TOTP secrets stored as plaintext base32 — enable pgcrypto column-level encryption for at-rest encryption in production.
- Recovery codes are bcrypt-hashed (cost 10); raw codes are never persisted.
- SSO client secrets in `np_sso_providers` should be encrypted by the application before storage.
- MFA is always free per the Security-Always-Free doctrine; SSO requires `NSELF_SSO=true`.
