---
id: loki-disk-full
title: Loki Disk Full
trigger:
  alert: LokiDiskFull
  severity: warning
steps:
  - action: check_logs
    params:
      service: "loki"
      last: "5m"
      level: "warn"
    requires_confirmation: false
  - action: run_command
    params:
      command: "df -h /loki"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Loki storage above 85%. Compaction running. Manual cleanup may be needed."
    requires_confirmation: false
  - action: escalate
    params:
      message: "Loki disk full — manual volume expansion or log purge required"
    requires_confirmation: true
---

# Loki Disk Full

**Trigger:** Loki data directory exceeds 85% capacity.

## Diagnosis

```bash
# Check disk usage
df -h /loki
du -sh /loki/chunks/*

# Check compaction status
curl -s http://localhost:3100/loki/api/v1/status/buildinfo
curl -s http://localhost:3100/metrics | grep loki_compactor
```

## Resolution

1. **Force compaction**: Loki compactor runs every 10m — wait one cycle, then recheck.
2. **Reduce retention**: Lower `retention_period` in `loki.yaml` from `720h` to `360h` and restart Loki.
3. **Delete old chunks manually** (only if retention is not clearing them):
   ```bash
   find /loki/chunks -mtime +30 -name "*.gz" -delete
   ```
4. **Expand volume**: Increase Docker volume size in `docker-compose.yml` and redeploy.
5. **Archive to S3**: Configure Loki `object_store: s3` in `loki.yaml` for long-term storage.
