---
id: high-error-rate
title: High Error Rate
trigger:
  alert: HighErrorRate
  severity: critical
steps:
  - action: check_logs
    params:
      service: "{{ service }}"
      last: "10m"
      level: "error"
    requires_confirmation: false
  - action: restart_service
    params:
      service: "{{ service }}"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Restarted {{ service }} due to high error rate. Monitor for recurrence."
    requires_confirmation: false
  - action: escalate
    params:
      message: "{{ service }} still failing after restart — manual intervention required"
    requires_confirmation: true
---

# High Error Rate

**Trigger:** More than 10 error-level log lines per minute from a plugin.

## Diagnosis

1. Open Grafana → Explore → Logs.
2. Query: `{nself_plugin="{{ service }}", nself_level="error"}`.
3. Look for patterns: repeated exception type, upstream timeout, DB connection exhaustion.
4. Check trace IDs in logs — click a `trace_id` to jump to Tempo trace.

## Automated Steps

The claw-ops agent executes these automatically:

1. Pulls last 10 minutes of error logs for the affected service.
2. Restarts the service container.
3. Posts a Slack notification to `#nself-alerts`.

## Manual Escalation

If errors continue after restart, investigate:

- DB connection pool exhaustion: `nself db connections`
- Redis unavailability: `nself redis status`
- Upstream API rate limits: check logs for 429 responses
- Code bug: `nself logs {{ service }} --tail 100 --level error`
