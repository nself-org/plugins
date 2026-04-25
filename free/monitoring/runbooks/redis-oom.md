---
id: redis-oom
title: Redis Out of Memory
trigger:
  alert: RedisMemoryHigh
  severity: critical
steps:
  - action: check_logs
    params:
      service: "redis"
      last: "10m"
      level: "warn"
    requires_confirmation: false
  - action: run_command
    params:
      command: "redis-cli INFO memory"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Redis memory high. Used: {{ used_memory_human }}. Max: {{ maxmemory_human }}."
    requires_confirmation: false
  - action: restart_service
    params:
      service: "redis"
    requires_confirmation: true
---

# Redis Out of Memory

**Trigger:** Redis `used_memory` approaches `maxmemory` or OOM killer activates.

## Diagnosis

```bash
redis-cli INFO memory | grep -E "used_memory_human|maxmemory_human|mem_fragmentation_ratio"
redis-cli INFO keyspace
redis-cli DBSIZE
```

## Resolution Steps

1. **Check eviction policy**: `CONFIG GET maxmemory-policy`. If `noeviction`, switch to `allkeys-lru` for cache use cases.
2. **Scan for large keys**: `redis-cli --bigkeys`
3. **Flush job queue DLQ if bloated**: `redis-cli LLEN queue:dlq` — if > 10k, consider `redis-cli LTRIM queue:dlq 0 999`
4. **Increase maxmemory**: Edit `REDIS_MAXMEMORY` in `.env.prod` and `nself deploy`.
5. **Last resort — restart**: `nself service restart redis` (clears all in-memory state including job queues; job-queue plugin will re-sync from `np_jobs` on startup).
