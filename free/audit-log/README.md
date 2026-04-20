# audit-log plugin

Free nSelf plugin. Provides an append-only audit log for security-relevant events across all nSelf plugins.

## What it stores

Events such as `auth.login`, `auth.login_failed`, `auth.mfa_enabled`, `privilege.change`, `secret.accessed`, `plugin.installed`, `plugin.uninstalled`. Each event records the actor, resource, IP address, severity, and arbitrary metadata.

The table is append-only. `DELETE` and `UPDATE` are permanently disabled via RLS policies.

## Installation

```bash
nself plugin install audit-log
nself build && nself start
```

## Multi-tenancy

### source_account_id (v1.0.9)

`np_auditlog_events.source_account_id` separates events by app within a single nSelf deploy (multi-APP isolation). This is the primary isolation mechanism used in v1.0.9.

### tenant_id (forward-compat, v1.0.9+)

`np_auditlog_events.tenant_id` is a nullable UUID column added in S74-T03 for forward compatibility with nSelf Cloud multi-tenancy.

- In v1.0.9: `tenant_id` is NULL for all rows. Single-user and multi-app deploys are unaffected.
- In v1.1.0+: a backfill migration will populate `tenant_id` from the user-to-tenant mapping table once that table exists.

**The two columns are not interchangeable.** See [multi-tenant-conventions.md](../../../../.claude/docs/architecture/multi-tenant-conventions.md) for the canonical decision tree.

### Hasura permission (v1.0.9)

The `user` role `select` permission uses an `_or` clause:
- When `X-Hasura-Tenant-Id` header is absent: rows with `tenant_id IS NULL` are visible (single-user behaviour, unchanged).
- When `X-Hasura-Tenant-Id` header is present: only rows matching `tenant_id` are visible (Cloud isolation).

## Migrations

| File | Description |
|---|---|
| `001_audit_log_init.sql` | Creates `np_auditlog_events` (partitioned, append-only, RLS) |
| `002_add_source_target_plugin.sql` | Adds `source_plugin` and `target_plugin` columns |
| `003_add_tenant_id_forward_compat.sql` | Adds nullable `tenant_id UUID` for Cloud multi-tenancy (S74-T03) |

## See also

- [nSelf audit-log plugin page](https://docs.nself.org/plugins/audit-log)
- [Multi-tenant conventions](https://docs.nself.org/multi-tenancy/conventions)
