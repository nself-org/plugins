# Plugin Development Standards

**Version**: 2026.02.11
**Status**: MANDATORY for all plugins
**Enforcement**: Automated validation in CI/CD

---

## Overview

This document defines the **mandatory standards** for all nself-plugins. These standards ensure consistency, prevent conflicts, and maintain professional quality across the ecosystem.

**⚠️ ALL standards marked as MANDATORY are enforced by automated validation and will cause build failures if violated.**

---

## 🚨 MANDATORY STANDARDS

### 1. Universal Table Prefix

**REQUIREMENT**: ALL database tables MUST use the `np_` prefix.

```sql
-- ✅ CORRECT
CREATE TABLE np_stripe_customers (...);
CREATE TABLE np_github_repos (...);
CREATE TABLE np_chat_messages (...);

-- ❌ WRONG - Will fail validation
CREATE TABLE stripe_customers (...);
CREATE TABLE github_repos (...);
CREATE TABLE chat_messages (...);
```

**Rationale**:
- Clear namespace separation (plugin tables vs user tables)
- Prevents collisions with user-defined tables
- Professional branding (nPlugins = np_)
- Future-proof architecture

**Table Naming Pattern**: `np_{plugin_abbreviation}_{table_name}`

Examples:

| Plugin | Abbreviation | Table Examples |
|--------|-------------|----------------|
| geocoding | `geoc` | `np_geoc_cache`, `np_geoc_geofences` |
| geolocation | `geoloc` | `np_geoloc_locations`, `np_geoloc_fences` |
| file-processing | `fileproc` | `np_fileproc_jobs`, `np_fileproc_thumbnails` |
| stream-gateway | `streamgw` | `np_streamgw_sessions`, `np_streamgw_rules` |
| stripe | `stripe` | `np_stripe_customers`, `np_stripe_invoices` |

**Validation**:
```bash
# All tables in plugin.json must start with np_
jq '.tables[] | select(startswith("np_") | not)' plugins/*/plugin.json
# Should return nothing
```

---

### 2. Multi-App Isolation

**REQUIREMENT**: ALL plugins MUST use `source_account_id` for multi-tenant isolation.

**Correct plugin.json configuration**:

```json
{
  "multiApp": {
    "supported": true,
    "isolationColumn": "source_account_id",
    "pkStrategy": "uuid",
    "defaultValue": "primary"
  }
}
```

**Forbidden**:
- ❌ `isolationColumn: "app_id"`
- ❌ `isolationColumn: "tenant_id"`
- ❌ `isolationColumn: "account_id"`
- ❌ `supported: false` (unless absolutely required with documented justification)

**Database Schema Requirements**:

```sql
CREATE TABLE np_plugin_resource (
    id UUID PRIMARY KEY,
    source_account_id VARCHAR(255) NOT NULL,  -- MANDATORY
    -- ... other columns ...

    -- Index for multi-tenant queries
    INDEX idx_np_plugin_resource_account (source_account_id)
);
```

**Validation**:
```bash
# Check all plugins use source_account_id
jq -r '.plugins[] | select(.multiApp.isolationColumn != "source_account_id") | .name' registry.json
# Should return nothing
```

---

### 3. Category Assignment

**REQUIREMENT**: Use one of the 13 official categories ONLY.

**Official Categories** (as of 2026.02.11):

1. **authentication** - Auth, identity, access control
2. **automation** - Workflows, bots, task automation
3. **commerce** - Payments, billing, donations, e-commerce
4. **communication** - Chat, messaging, notifications, streaming
5. **content** - CMS, social, moderation, knowledge bases
6. **data** - Data operations, documents, location tracking
7. **development** - Dev tools, GitHub, meetings, productivity
8. **infrastructure** - Core services (CDN, storage, search, jobs, etc.)
9. **integrations** - AI, web3, external service integrations
10. **media** - Video, photos, EPG, content progress
11. **streaming** - Live streaming microservices architecture
12. **sports** - Sports data and statistics
13. **compliance** - GDPR, audit, privacy, legal

**Plugin.json Example**:

```json
{
  "name": "my-plugin",
  "category": "commerce",  // Must be one of the 13 above
  ...
}
```

**❌ DO NOT**:
- Create new categories without team approval
- Use deprecated categories: `billing`, `payments`, `monetization`, `ai-ml`, `voice-video`, `privacy-legal`, etc.

**Validation**:
```bash
# Check for invalid categories
VALID_CATS="authentication|automation|commerce|communication|content|data|development|infrastructure|integrations|media|streaming|sports|compliance"
jq -r --arg cats "$VALID_CATS" '.plugins[] | select(.category | test($cats) | not) | "\(.name): \(.category)"' registry.json
# Should return nothing
```

---

### 4. Plugin Naming Format

**REQUIREMENT**: Plugin names MUST use lowercase-with-hyphens format.

```json
// ✅ CORRECT
"name": "data-operations"
"name": "file-processing"
"name": "link-preview"
"name": "stream-gateway"

// ❌ WRONG
"name": "DataOperations"
"name": "file_processing"
"name": "linkPreview"
"name": "StreamGateway"
```

**Directory Structure**:
```
plugins/
├── data-operations/      ✅
├── file-processing/      ✅
├── link-preview/         ✅
└── DataOperations/       ❌ WRONG
```

---

### 5. View Naming Convention

**REQUIREMENT**: Database views MUST follow the same `np_` prefix pattern.

```sql
-- ✅ CORRECT
CREATE VIEW np_stripe_mrr AS ...;
CREATE VIEW np_geoc_hit_rate AS ...;
CREATE VIEW np_streamgw_concurrent_viewers AS ...;

-- ❌ WRONG
CREATE VIEW stripe_mrr AS ...;
CREATE VIEW geocoding_stats AS ...;
```

**plugin.json**:

```json
{
  "views": [
    "np_stripe_mrr",
    "np_stripe_arr",
    "np_stripe_churn_rate"
  ]
}
```

---

### 6. Index Naming Convention

**REQUIREMENT**: Indexes MUST follow the pattern: `idx_np_{plugin}_{table}_{column}`

```sql
-- ✅ CORRECT
CREATE INDEX idx_np_stripe_customers_email
    ON np_stripe_customers(email);

CREATE INDEX idx_np_geoloc_locations_timestamp
    ON np_geoloc_locations(timestamp DESC);

-- ❌ WRONG
CREATE INDEX stripe_customers_email_idx ...;
CREATE INDEX idx_customers_email ...;
```

---

## 📋 RECOMMENDED STANDARDS

### File Structure

**Recommended plugin structure**:

```
plugins/my-plugin/
├── plugin.json           # REQUIRED
├── README.md             # Recommended
└── ts/                   # TypeScript implementation
    ├── package.json
    ├── tsconfig.json
    ├── .env.example
    └── src/
        ├── types.ts      # All TypeScript interfaces
        ├── config.ts     # Environment config
        ├── database.ts   # Schema + CRUD
        ├── client.ts     # API client (if external service)
        ├── sync.ts       # Data sync logic
        ├── webhooks.ts   # Webhook handlers
        ├── server.ts     # Fastify HTTP server
        ├── cli.ts        # Commander.js CLI
        └── index.ts      # Module exports
```

### Environment Variables

**Naming Pattern**: `{PLUGIN_NAME_UPPERCASE}_{VAR_NAME}`

```bash
# ✅ CORRECT
STRIPE_API_KEY=sk_test_...
STRIPE_WEBHOOK_SECRET=whsec_...
GEOCODING_GOOGLE_API_KEY=AIza...

# ❌ WRONG (too generic)
API_KEY=...
WEBHOOK_SECRET=...
GOOGLE_KEY=...
```

### Port Assignment

**Current port ranges** (informational, not enforced):

- Authentication: 3000-3099
- Commerce: 3100-3199
- Infrastructure: 3200-3299
- Communication: 3400-3499
- Content: 3500-3599
- Data: 3600-3699
- Media: 3700-3799
- Streaming: 3800-3899
- Development: 3900-3999
- Integrations: 4000-4099

**Note**: Port ranges are guidelines only. Existing ports should not be changed without coordination.

---

## ✅ Validation Checklist

Before submitting a plugin or PR, verify:

- [ ] All tables start with `np_` prefix
- [ ] Plugin uses `source_account_id` for multi-app isolation
- [ ] Category is one of the 13 official categories
- [ ] Plugin name uses lowercase-with-hyphens
- [ ] All views start with `np_` prefix
- [ ] All indexes follow `idx_np_*` pattern
- [ ] plugin.json is valid JSON
- [ ] Environment variables follow naming convention
- [ ] README.md exists with setup instructions
- [ ] Wiki documentation is comprehensive

### Automated Validation

Run these commands before committing:

```bash
# Validate JSON syntax
jq empty plugins/my-plugin/plugin.json

# Check table prefixes
jq -r '.tables[] | select(startswith("np_") | not)' plugins/my-plugin/plugin.json

# Check multi-app isolation
jq -r '.multiApp.isolationColumn' plugins/my-plugin/plugin.json
# Should output: source_account_id

# Check category
jq -r '.category' plugins/my-plugin/plugin.json
# Should be one of the 13 official categories
```

---

## 🚫 Common Violations

### Violation: Missing np_ prefix

**Error**:
```
❌ Table 'stripe_customers' does not start with 'np_' prefix
```

**Fix**:
```json
// Before
"tables": ["stripe_customers", "stripe_invoices"]

// After
"tables": ["np_stripe_customers", "np_stripe_invoices"]
```

### Violation: Wrong isolation column

**Error**:
```
❌ Plugin 'my-plugin' uses 'app_id' instead of 'source_account_id'
```

**Fix**:
```json
// Before
"multiApp": {
  "isolationColumn": "app_id"
}

// After
"multiApp": {
  "isolationColumn": "source_account_id"
}
```

### Violation: Invalid category

**Error**:
```
❌ Category 'billing' is deprecated. Use 'commerce' instead.
```

**Fix**:
```json
// Before
"category": "billing"

// After
"category": "commerce"
```

---

## 📚 Migration Guide

### Migrating Existing Plugins

If you have an existing plugin that doesn't follow these standards:

1. **Update plugin.json**:
   - Add `np_` prefix to all table names
   - Change `isolationColumn` to `source_account_id`
   - Update category to one of the 13 official ones

2. **Update database schemas**:
   ```sql
   -- Rename tables
   ALTER TABLE old_table RENAME TO np_plugin_table;

   -- Add source_account_id if missing
   ALTER TABLE np_plugin_table ADD COLUMN source_account_id VARCHAR(255);

   -- Update indexes
   CREATE INDEX idx_np_plugin_table_account ON np_plugin_table(source_account_id);
   ```

3. **Update all code references**:
   - Search/replace old table names with new ones
   - Update all SQL queries
   - Update TypeScript interfaces

4. **Update registry.json**:
   - Run `node update-registry.js` to sync changes

5. **Update wiki documentation**:
   - Update all table name references
   - Update schema examples
   - Update SQL query examples

---

## 🆘 Getting Help

If you have questions about these standards:

1. Check [DEVELOPMENT.md](.wiki/DEVELOPMENT.md) for examples
2. Review existing plugins for reference implementations
3. Ask in the nself-plugins GitHub discussions
4. Submit an issue for clarification requests

---

## 📅 Standard Updates

**Current Version**: 2026.02.11

**Change Log**:
- 2026.02.11: Initial standards document
  - Mandatory `np_` table prefix
  - Mandatory `source_account_id` isolation
  - Category consolidation (18 → 13)

**Review Cycle**: Standards are reviewed quarterly and updated as needed.

---

**Status**: ✅ ENFORCED in CI/CD as of 2026.02.11
