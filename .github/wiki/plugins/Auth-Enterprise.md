# Auth Enterprise Plugin

> MFA enforcement (TOTP + WebAuthn policy) and SSO via SAML 2.0 and OIDC for Google Workspace, Okta, and Microsoft Entra ID. **Free — MIT licensed.**

## Install

```bash
nself plugin install auth-enterprise
```

No license key required.

## Description

MFA enforcement (TOTP + WebAuthn policy) and SSO via SAML 2.0 and OIDC for Google Workspace, Okta, and Microsoft Entra ID.

Category: `authentication`. Current version: `1.1.2`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `NSELF_SSO` | `false` | Enable SSO (SAML 2.0 + OIDC). Set to 'true' to activate. |
| `AUTH_ENTERPRISE_TOTP_ISSUER` | `nSelf` | TOTP issuer name shown in authenticator apps (e.g. 'My Company'). Defaults to 'nSelf'. |
| `AUTH_ENTERPRISE_SSO_SP_ENTITY_ID` | `-` | SAML SP entity ID (URI). Required when NSELF_SSO=true and SAML providers are configured. |
| `AUTH_ENTERPRISE_SSO_ACS_URL` | `-` | SAML Assertion Consumer Service (ACS) URL. Required when NSELF_SSO=true and SAML providers are configured. |
| `AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL` | `-` | OIDC redirect URI registered with your IdP. Required when NSELF_SSO=true and OIDC providers are configured. |

## Ports

| Port | Purpose |
|------|---------|
| 3826 | Auth Enterprise service port |

## Database Schema

6 table(s) added to your Postgres database:

- `np_mfa_enrollments`
- `np_mfa_recovery_codes`
- `np_mfa_policies`
- `np_sso_providers`
- `np_sso_sessions`
- `np_sso_state_cache`

## REST API

```
GET    /health
GET    /auth/mfa/policy
PUT    /auth/mfa/policy
POST   /auth/mfa/recovery
GET    /auth/mfa/recovery/codes
POST   /auth/mfa/recovery/regenerate
GET    /auth/mfa/status
POST   /auth/mfa/totp/challenge
POST   /auth/mfa/totp/setup
POST   /auth/mfa/totp/verify
GET    /auth/sso/metadata
GET    /auth/sso/oidc/callback
```

## Examples

### Health check

```bash
curl http://localhost:3826/health
```

## Source

[`plugins/auth-enterprise/`](https://github.com/nself-org/plugins/tree/main/auth-enterprise)

Manifest: [`plugins/auth-enterprise/plugin.json`](https://github.com/nself-org/plugins/tree/main/auth-enterprise/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
