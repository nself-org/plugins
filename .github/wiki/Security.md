# Security Audit Report

**nself-plugins Security Assessment**
**Version**: 1.0.0
**Audit Date**: January 24, 2026
**Audit Scope**: Complete codebase (3 plugins, shared utilities, Cloudflare Worker, CI/CD workflows)

---

## Table of Contents

- [Executive Summary](#executive-summary)
- [Security Posture Overview](#security-posture-overview)
- [Findings by Component](#findings-by-component)
  - [Stripe Plugin](#stripe-plugin)
  - [GitHub Plugin](#github-plugin)
  - [Shopify Plugin](#shopify-plugin)
  - [Shared Utilities](#shared-utilities)
  - [Cloudflare Worker](#cloudflare-worker)
  - [CI/CD Workflows](#cicd-workflows)
- [Security Best Practices](#security-best-practices)
- [Deployment Security](#deployment-security)
- [Remediation Priorities](#remediation-priorities)
- [Security Checklist](#security-checklist)

---

## Executive Summary

This document presents the findings from a comprehensive security audit of the nself-plugins repository. The audit covered all three production plugins (Stripe, GitHub, Shopify), shared utilities, the Cloudflare Worker registry, and all GitHub Actions workflows.

### Overall Assessment

| Component | Risk Level | Critical Issues | High Issues | Medium Issues |
|-----------|------------|-----------------|-------------|---------------|
| **Stripe Plugin** | Medium | 1 | 1 | 5 |
| **GitHub Plugin** | Medium | 0 | 2 | 5 |
| **Shopify Plugin** | Medium | 1 | 3 | 4 |
| **Shared Utilities** | High | 2 | 2 | 6 |
| **Cloudflare Worker** | High | 3 | 5 | 4 |
| **CI/CD Workflows** | Medium | 2 | 5 | 5 |

### Key Strengths

1. **SQL Injection Prevention**: All database queries use parameterized statements
2. **Webhook Signature Verification**: HMAC-SHA256 verification implemented for all services
3. **Type Safety**: Strong TypeScript typing throughout the codebase
4. **No Dynamic Code Execution**: No use of `eval()` or `Function()` constructor
5. **Soft Deletes**: Data integrity maintained through `deleted_at` timestamps

### Key Concerns (Addressed)

The following issues from the original audit have been **resolved**:

1. ~~**Missing API Authentication**~~ **FIXED**: API key authentication now available via `NSELF_API_KEY` or `{PLUGIN}_API_KEY` environment variables
2. ~~**Optional Webhook Secrets**~~ **FIXED**: Webhook secrets are now mandatory in production (`NODE_ENV=production`)
3. ~~**No Rate Limiting**~~ **FIXED**: Rate limiting middleware now enabled on all endpoints (configurable via `RATE_LIMIT_MAX`, `RATE_LIMIT_WINDOW_MS`)
4. **SSL Certificate Validation Disabled**: Database connections vulnerable to MITM *(Requires manual configuration)*
5. **Information Disclosure**: Error messages leak internal details *(Partially addressed)*

### Remaining Concerns

1. **SSL Certificate Validation**: Set `POSTGRES_SSL=true` and configure proper certificates
2. **API Key Storage**: Use environment variables or secrets manager for API keys

---

## Security Posture Overview

### Authentication & Authorization

| Feature | Status | Notes |
|---------|--------|-------|
| API Authentication | **Implemented** | Set `NSELF_API_KEY` or `{PLUGIN}_API_KEY` to enable |
| Webhook Verification | **Enforced in Production** | Required when `NODE_ENV=production` |
| Rate Limiting | **Implemented** | 100 req/min default, configurable |
| Database Authentication | **Implemented** | Uses PostgreSQL credentials |
| Worker Sync Authentication | **Implemented** | Bearer token required |

### Data Protection

| Feature | Status | Notes |
|---------|--------|-------|
| SQL Injection Prevention | **Implemented** | Parameterized queries throughout |
| XSS Prevention | **N/A** | No HTML rendering |
| Sensitive Data in Logs | **Risk** | Some sensitive fields logged |
| Data Encryption at Rest | **Not Implemented** | Relies on database encryption |
| TLS for API Calls | **Implemented** | HTTPS used for all external APIs |

### Input Validation

| Feature | Status | Notes |
|---------|--------|-------|
| Query Parameter Validation | **Partial** | Basic type casting, no bounds checking |
| Path Parameter Validation | **Not Implemented** | Integer IDs not validated |
| Webhook Payload Validation | **Partial** | JSON parsed, schema not validated |
| CLI Argument Validation | **Partial** | Basic validation only |

---

## Findings by Component

### Stripe Plugin

**Location**: `plugins/stripe/ts/src/`

#### Critical Issues

**1. No Authentication on REST Endpoints**
- **File**: [server.ts](../plugins/stripe/ts/src/server.ts)
- **Impact**: Anyone with network access can read all customer, invoice, and payment data
- **Recommendation**: Implement API key or JWT authentication

```typescript
// Current (VULNERABLE)
app.get('/api/customers', async (request) => {
  const customers = await db.listCustomers(limit, offset);
  return { data: customers };
});

// Recommended
app.get('/api/customers', { preHandler: authenticateRequest }, async (request) => {
  // ... with auth middleware
});
```

#### High Issues

**2. Webhook Signature Verification Optional**
- **File**: [server.ts:80-85](../plugins/stripe/ts/src/server.ts#L80-L85)
- **Impact**: If `STRIPE_WEBHOOK_SECRET` is not set, webhooks accepted without verification
- **Recommendation**: Make webhook secret mandatory in production

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Missing API key format validation | config.ts | 58 | Validate `sk_(live|test)_` prefix |
| Sensitive data in logs | client.ts | 60 | Sanitize log parameters |
| No rate limiting on endpoints | server.ts | 119+ | Add `@fastify/rate-limit` |
| Client secret persistence | types.ts | 371 | Don't store payment secrets |
| Overly permissive CORS | server.ts | 37 | Restrict allowed origins |

#### Secure Patterns

- SQL queries use `$1, $2` parameterization
- Proper error type checking with `error instanceof Error`
- Safe JSON serialization for metadata fields
- Soft deletes preserve data integrity

---

### GitHub Plugin

**Location**: `plugins/github/ts/src/`

#### High Issues

**1. Insufficient Webhook Payload Validation**
- **File**: [webhooks.ts:87-104](../plugins/github/ts/src/webhooks.ts#L87-L104)
- **Impact**: Malformed payloads could cause runtime errors or data corruption

```typescript
// Current (VULNERABLE)
const repository = payload.repository as { id: number; full_name: string };
const [owner, repo] = repository.full_name.split('/');

// Recommended
if (!payload.repository?.full_name || typeof payload.repository.full_name !== 'string') {
  throw new Error('Invalid webhook payload');
}
const parts = payload.repository.full_name.split('/');
if (parts.length !== 2) {
  throw new Error('Invalid repository format');
}
```

**2. Error Information Leakage**
- **File**: [server.ts:122](../plugins/github/ts/src/server.ts#L122)
- **Impact**: Internal error messages exposed to API clients

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Query parameter validation missing | server.ts | 143+ | Validate limit/offset bounds |
| No explicit rate limiting | client.ts | - | Add backoff for GitHub API limits |
| Weak config validation | config.ts | 63 | Validate all required env vars |
| Webhook signature optional | server.ts | 76 | Require secret in production |
| Sensitive data logged | webhooks.ts | 108 | Filter webhook details |

---

### Shopify Plugin

**Location**: `plugins/shopify/ts/src/`

#### Critical Issues

**1. No Authentication on API Endpoints**
- **File**: [server.ts:70-373](../plugins/shopify/ts/src/server.ts#L70-L373)
- **Impact**: Complete data exposure including customer and order information

#### High Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Missing path parameter validation | server.ts | 174 | Validate integer IDs |
| Missing query bounds checking | server.ts | 159 | Add max limit (e.g., 1000) |
| Error details leaked to clients | server.ts | 108 | Return generic error messages |

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Status parameter unvalidated | server.ts | 228 | Whitelist valid statuses |
| Order totals logged | webhooks.ts | 155 | Remove sensitive fields from logs |
| Customer emails logged | webhooks.ts | 229 | Don't log PII |
| No API rate limiting | server.ts | - | Add rate limit middleware |

---

### Shared Utilities

**Location**: `shared/src/`

#### Critical Issues

**1. SSL Certificate Validation Disabled**
- **File**: [database.ts:25](../shared/src/database.ts#L25)
- **Impact**: Database connections vulnerable to man-in-the-middle attacks

```typescript
// Current (VULNERABLE)
ssl: config.ssl ? { rejectUnauthorized: false } : undefined,

// Recommended
ssl: config.ssl ? { rejectUnauthorized: true } : undefined,
```

**2. SQL Injection via Dynamic Identifiers**
- **File**: [database.ts:139-143](../shared/src/database.ts#L139-L143)
- **Impact**: Table/column names interpolated without validation

```typescript
// Current (VULNERABLE)
const sql = `INSERT INTO ${table} (${columns.join(', ')}) VALUES ...`;

// Recommended: Validate identifiers against whitelist
if (!ALLOWED_TABLES.includes(table)) {
  throw new Error('Invalid table name');
}
```

#### High Issues

**3. No URL Validation in HTTP Client**
- **File**: [http.ts:44](../shared/src/http.ts#L44)
- **Impact**: Potential SSRF if baseUrl from user input

**4. No Webhook Payload Size Limits**
- **File**: [webhook.ts:207-212](../shared/src/webhook.ts#L207-L212)
- **Impact**: Large payloads could cause memory exhaustion

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Stripe timestamp tolerance | webhook.ts | 54 | Reduce from 300s to 60s |
| Signature stored in event | webhook.ts | 222 | Don't persist signatures |
| No sensitive data filtering in logger | logger.ts | 57 | Implement field redaction |
| LOG_LEVEL not validated | logger.ts | 130 | Whitelist valid levels |
| No response schema validation | http.ts | 83 | Validate API responses |
| WebhookEvent.data too permissive | types.ts | 32 | Use generic type parameter |

---

### Cloudflare Worker

**Location**: `.workers/plugins-registry/`

#### Critical Issues

**1. Timing-Attack Vulnerable Token Comparison**
- **File**: [src/index.js:308](../.workers/plugins-registry/src/index.js#L308)
- **Impact**: Token can be brute-forced via timing analysis

```javascript
// Current (VULNERABLE)
if (token !== env.GITHUB_SYNC_TOKEN) {

// Recommended: Use constant-time comparison
const crypto = require('crypto');
if (!crypto.timingSafeEqual(Buffer.from(token), Buffer.from(env.GITHUB_SYNC_TOKEN))) {
```

**2. Overly Permissive CORS**
- **File**: [src/index.js:18](../.workers/plugins-registry/src/index.js#L18)
- **Impact**: Any website can access registry data and potentially trigger CSRF

**3. Weak Sync Endpoint Authentication**
- **File**: [src/index.js:297-313](../.workers/plugins-registry/src/index.js#L297-L313)
- **Impact**: No rate limiting, no replay protection, timing-vulnerable

#### High Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Error message information disclosure | src/index.js | 89 | Return generic messages |
| Missing plugin name validation | src/index.js | 176 | Validate against regex |
| Unvalidated GitHub URL construction | src/index.js | 352 | Validate repo/branch format |
| Cache poisoning risk | src/index.js | 142 | Validate registry schema |
| Secret printed to stdout | deploy.sh | 200 | Use secure output |

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| No rate limiting on public endpoints | src/index.js | 42+ | Add Cloudflare rate limiting |
| Weak token generation fallback | deploy.sh | 187 | Ensure strong entropy |
| Hardcoded KV namespace ID | wrangler.toml | 14 | Move to environment |
| No request size limits | src/index.js | - | Validate Content-Length |

---

### CI/CD Workflows

**Location**: `.github/workflows/`

#### Critical Issues

**1. JSON Injection in publish.yml**
- **File**: [publish.yml:107](../.github/workflows/publish.yml#L107)
- **Impact**: Malicious ref names could inject arbitrary JSON

```yaml
# Current (VULNERABLE)
-d "{\"ref\": \"${{ github.ref }}\", \"sha\": \"${{ github.sha }}\"}"

# Recommended: Use proper JSON encoding
jq -n --arg ref "${{ github.ref }}" --arg sha "${{ github.sha }}" '{ref: $ref, sha: $sha}'
```

**2. Command Injection Risk in validate.yml**
- **File**: [validate.yml:56-57](../.github/workflows/validate.yml#L56-L57)
- **Impact**: Special characters in filenames could cause issues

#### High Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Hardcoded infrastructure IDs | wrangler.toml | 14, 26 | Use GitHub secrets |
| Token timing attack in Worker | index.js | 308 | Use constant-time comparison |
| Unvalidated GitHub responses | index.js | 354 | Validate schema |
| Secrets printed to logs | deploy.sh | 200 | Secure output only |
| Weak workflow triggers | publish.yml | 3-22 | Require release tags only |

#### Medium Issues

| Issue | File | Line | Recommendation |
|-------|------|------|----------------|
| Git push error suppression | publish.yml | 96 | Remove `|| true` |
| jq injection from variables | publish.yml | 56 | Use `--arg` parameter |
| Wiki token too permissive | wiki-sync.yml | 37 | Use fine-grained token |
| Missing file existence checks | validate.yml | 44 | Add explicit validation |
| Bash compatibility issues | validate.yml | 65 | Fix regex escaping |

---

## Security Best Practices

### For Plugin Operators

1. **Always Configure Webhook Secrets**
   ```bash
   # Required for security
   STRIPE_WEBHOOK_SECRET=whsec_xxx
   GITHUB_WEBHOOK_SECRET=xxx
   SHOPIFY_WEBHOOK_SECRET=xxx
   ```

2. **Use Network Isolation**
   - Run plugin servers on internal networks only
   - Use reverse proxy with authentication for external access

3. **Enable Database SSL**
   ```bash
   # Use SSL for database connections
   DATABASE_URL=postgresql://user:pass@host:5432/db?sslmode=require
   ```

4. **Monitor Webhook Events**
   - Review `*_webhook_events` tables for anomalies
   - Alert on signature verification failures

5. **Rotate Credentials Regularly**
   - API keys: Every 90 days
   - Webhook secrets: Every 180 days
   - Database passwords: Every 90 days

### For Plugin Developers

1. **Validate All Inputs**
   ```typescript
   // Validate integer parameters
   const id = parseInt(params.id, 10);
   if (isNaN(id) || id <= 0) {
     return reply.status(400).send({ error: 'Invalid ID' });
   }

   // Validate string parameters against whitelist
   const validStatuses = ['active', 'inactive', 'pending'];
   if (!validStatuses.includes(status)) {
     return reply.status(400).send({ error: 'Invalid status' });
   }
   ```

2. **Never Log Sensitive Data**
   ```typescript
   // BAD
   logger.info('Processing payment', { apiKey, customerEmail });

   // GOOD
   logger.info('Processing payment', { customerId: customer.id });
   ```

3. **Return Generic Error Messages**
   ```typescript
   // BAD
   return reply.status(500).send({ error: err.message });

   // GOOD
   logger.error('Operation failed', { error: err.message });
   return reply.status(500).send({ error: 'Internal server error' });
   ```

4. **Enforce Webhook Verification**
   ```typescript
   // Make webhook secret mandatory
   if (!config.webhookSecret) {
     throw new Error('Webhook secret is required for production');
   }
   ```

---

## Deployment Security

### Production Checklist

- [ ] All webhook secrets configured
- [ ] Database SSL enabled (`sslmode=require`)
- [ ] Plugin servers on internal network only
- [ ] Reverse proxy with authentication configured
- [ ] Rate limiting enabled at proxy level
- [ ] Logs sanitized of sensitive data
- [ ] Credentials rotated from defaults
- [ ] Monitoring and alerting configured

### Network Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Public Internet                          │
└──────────────────────────────┬──────────────────────────────────┘
                               │
                    ┌──────────┴──────────┐
                    │   Reverse Proxy     │
                    │  (nginx/Cloudflare) │
                    │  - Rate limiting    │
                    │  - Authentication   │
                    │  - TLS termination  │
                    └──────────┬──────────┘
                               │
                    ┌──────────┴──────────┐
                    │   Internal Network   │
                    │                      │
    ┌───────────────┼───────────────┐     │
    │               │               │     │
┌───┴───┐     ┌────┴────┐    ┌─────┴─────┐
│Stripe │     │ GitHub  │    │ Shopify   │
│Plugin │     │ Plugin  │    │ Plugin    │
│:3001  │     │ :3002   │    │ :3003     │
└───┬───┘     └────┬────┘    └─────┬─────┘
    │              │               │
    └──────────────┼───────────────┘
                   │
          ┌────────┴────────┐
          │   PostgreSQL    │
          │   (SSL/TLS)     │
          └─────────────────┘
```

### Webhook Security

Configure your reverse proxy to forward webhook requests:

```nginx
# nginx configuration example
location /webhook/stripe {
    # Only allow Stripe IPs (optional but recommended)
    allow 3.18.12.63;      # Stripe webhook IPs
    allow 3.130.192.231;
    allow 13.235.14.237;
    allow 13.235.122.149;
    allow 18.211.135.69;
    allow 35.154.171.200;
    allow 52.15.183.38;
    allow 54.88.130.119;
    allow 54.88.130.237;
    allow 54.187.174.169;
    allow 54.187.205.235;
    allow 54.187.216.72;
    deny all;

    proxy_pass http://localhost:3001/webhook;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Real-IP $remote_addr;
}
```

---

## Remediation Priorities

### Immediate (Critical - Fix Before Production)

1. ~~**Add API Authentication** to all plugin REST endpoints~~ **DONE** - Set `NSELF_API_KEY` or `{PLUGIN}_API_KEY`
2. ~~**Make Webhook Secrets Mandatory** in production mode~~ **DONE** - Enforced when `NODE_ENV=production`
3. **Enable SSL Certificate Validation** for database connections - Set `POSTGRES_SSL=true`
4. **Fix Timing Attack** in Cloudflare Worker token comparison
5. **Validate Database Identifiers** against whitelist

### High Priority (Fix Within 2 Weeks)

6. ~~Add input validation for all path and query parameters~~ **DONE** - Added `shared/src/validation.ts`
7. ~~Implement rate limiting on all public endpoints~~ **DONE** - 100 req/min default
8. Fix JSON injection in GitHub workflow
9. Add webhook payload schema validation
10. Remove sensitive data from logs

### Medium Priority (Fix Within 1 Month)

11. Reduce Stripe signature timestamp tolerance
12. Add response schema validation to HTTP client
13. Implement request size limits
14. Add security headers to Worker responses
15. Use fine-grained tokens for CI/CD

### Low Priority (Ongoing Improvements)

16. Implement audit logging for all data modifications
17. Add automated security scanning to CI/CD
18. Implement secrets management integration
19. Add connection pool monitoring
20. Create security runbook for incidents

---

## Security Checklist

### Pre-Deployment

- [ ] Reviewed all environment variables for sensitive data
- [ ] Webhook secrets configured for all plugins
- [ ] Database SSL enabled
- [ ] API endpoints protected by authentication layer
- [ ] Rate limiting configured
- [ ] Logging sanitized

### Ongoing Operations

- [ ] Monitor webhook event failures
- [ ] Review access logs weekly
- [ ] Rotate credentials quarterly
- [ ] Update dependencies monthly
- [ ] Review security alerts from GitHub

### Incident Response

- [ ] Document credential rotation process
- [ ] Create runbook for webhook signature failures
- [ ] Establish escalation path for security issues
- [ ] Test backup and restore procedures

---

## Reporting Security Issues

If you discover a security vulnerability in nself-plugins:

1. **Do NOT** open a public GitHub issue
2. Email security concerns to the maintainers directly
3. Include detailed reproduction steps
4. Allow 90 days for remediation before public disclosure

---

*This security audit was conducted on January 24, 2026. Regular security reviews are recommended every 6 months or after significant changes to the codebase.*
