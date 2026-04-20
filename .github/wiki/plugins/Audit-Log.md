# Audit Log Plugin

Append-only audit log for security-relevant events: authentication, privilege changes, secret access, and plugin install/uninstall. Queryable from Admin with filters by event type, actor, severity, and time range.

| Property | Value |
|----------|-------|
| **Plugin name** | `audit-log` |
| **Port** | `3308` |
| **Category** | `compliance` |
| **License** | MIT (free) |
| **Status** | Stable |
| **Min nself** | `1.0.0` |
| **Multi-App** | `source_account_id` (UUID) |

---

## Install

```bash
nself plugin install audit-log
nself build
nself restart
```

No license key required — this is a free MIT plugin.

---

## Configuration

### Required Environment Variables

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `PLUGIN_INTERNAL_SECRET` | Shared secret for plugin-to-plugin HTTP calls (`X-Internal-Token` header) |

### Optional Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `3308` | HTTP server port |
| `HASURA_GRAPHQL_ADMIN_SECRET` | — | Hasura admin secret (for Hasura metadata registration) |

---

## Database Tables

| Table | Description |
|-------|-------------|
| `np_auditlog_events` | All recorded audit events (append-only) |

---

## What Gets Logged

The audit-log plugin captures events from the CLI and other plugins automatically when installed:

- User authentication (login, logout, token refresh)
- Privilege changes (role grants and revocations)
- Secret access (reads of encrypted secrets via `nself secrets get`)
- Plugin install, update, and uninstall operations
- Admin-panel access events

Custom events can be written by other plugins via the internal API.

---

## Querying Audit Logs

From the nself Admin UI: open the Audit Log section in the sidebar. Filter by event type, actor, severity (`info`, `warning`, `critical`), or time range.

Via the CLI:

```bash
# View recent events
nself plugin run audit-log events --limit 50

# Filter by event type
nself plugin run audit-log events --type auth.login

# JSON output for SIEM export
nself plugin run audit-log events --format json --since 24h
```

Via GraphQL (Hasura):

```graphql
query AuditEvents($since: timestamptz!) {
  np_auditlog_events(
    where: { created_at: { _gte: $since } }
    order_by: { created_at: desc }
  ) {
    id
    event_type
    actor_id
    severity
    payload
    created_at
  }
}
```

---

## Retention

Events are append-only — no updates or deletes via the API. Retention cleanup (if needed) must be done via direct database maintenance. Future versions will add configurable retention policies.

---

## Related

- [[Security]] — nself security overview
- [[cmd-backup]] — back up audit logs along with your Postgres data
- [Plugin Development](Plugin-Development) — write your own plugins
