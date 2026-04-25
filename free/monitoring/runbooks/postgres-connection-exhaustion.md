---
id: postgres-connection-exhaustion
title: Postgres Connection Exhaustion
trigger:
  alert: PostgresConnectionsHigh
  severity: critical
steps:
  - action: check_logs
    params:
      service: "postgres"
      last: "5m"
      level: "error"
    requires_confirmation: false
  - action: run_query
    params:
      query: "SELECT count(*), state FROM pg_stat_activity GROUP BY state"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Postgres connection pool near exhaustion. Active connections: {{ active_count }}."
    requires_confirmation: false
  - action: restart_service
    params:
      service: "hasura"
    requires_confirmation: true
---

# Postgres Connection Exhaustion

**Trigger:** `pg_stat_activity` count approaches `max_connections`.

## Diagnosis

```sql
-- Active connections by state
SELECT state, count(*) FROM pg_stat_activity GROUP BY state;

-- Long-running queries
SELECT pid, now() - query_start AS duration, query
FROM pg_stat_activity
WHERE state = 'active' AND query_start < now() - interval '30s'
ORDER BY duration DESC;

-- Idle connections consuming slots
SELECT count(*) FROM pg_stat_activity WHERE state = 'idle';
```

## Resolution

1. Terminate idle connections: `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state = 'idle' AND query_start < now() - interval '5m';`
2. Reduce `pool_size` in Hasura environment if pool is oversized.
3. Restart Hasura to reset connection pool: `nself service restart hasura`.
4. Consider PgBouncer if connections remain exhausted after restart.
