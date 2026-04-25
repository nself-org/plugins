---
id: webhook-dlq-overflow
title: Webhook DLQ Overflow
trigger:
  alert: WebhookDLQHigh
  severity: warning
steps:
  - action: check_logs
    params:
      service: "webhooks"
      last: "30m"
      level: "error"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Webhook DLQ has {{ dlq_count }} failed jobs. Review and replay or discard."
    requires_confirmation: false
  - action: escalate
    params:
      message: "Webhook DLQ overflow requires manual triage"
    requires_confirmation: true
---

# Webhook DLQ Overflow

**Trigger:** `np_webhooks_dlq` row count exceeds 100 or Redis `queue:dlq` length > 1000.

## Diagnosis

```sql
-- Count DLQ entries by error type
SELECT final_error, count(*)
FROM np_job_dlq
JOIN np_jobs ON np_job_dlq.job_id = np_jobs.id
WHERE np_jobs.queue = 'webhooks'
GROUP BY final_error
ORDER BY count DESC
LIMIT 20;
```

## Resolution

1. **Replay recent DLQ entries**: `nself dlq replay webhooks --max-rows 50 --dry-run` then remove `--dry-run`.
2. **Check target endpoint health**: curl the endpoint URL from a failed job's payload.
3. **Discard stale DLQ entries** (> 7 days old): `DELETE FROM np_job_dlq WHERE dlq_at < now() - interval '7 days';`
4. **Reduce retry aggressiveness** if endpoint is permanently down: update `JOBQUEUE_MAX_ATTEMPTS` in `.env`.
