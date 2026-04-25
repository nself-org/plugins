---
id: plugin-crash-loop
title: Plugin Crash Loop
trigger:
  alert: PluginCrashLoop
  severity: critical
steps:
  - action: check_logs
    params:
      service: "{{ plugin }}"
      last: "10m"
      level: "error"
    requires_confirmation: false
  - action: restart_service
    params:
      service: "{{ plugin }}"
    requires_confirmation: false
  - action: notify_slack
    params:
      channel: "#nself-alerts"
      message: "Plugin {{ plugin }} crash loop detected. Restarted. Monitor status."
    requires_confirmation: false
  - action: escalate
    params:
      message: "Plugin {{ plugin }} crash loop persists after restart — needs code investigation"
    requires_confirmation: true
---

# Plugin Crash Loop

**Trigger:** A plugin container restarts more than 3 times within 5 minutes.

## Diagnosis

```bash
# Check restart count and last exit reason
docker inspect $(docker ps -aqf "name={{ plugin }}") | jq '.[0].RestartCount, .[0].State'

# Get logs including OOM killer info
docker logs --tail 100 $(docker ps -aqf "name={{ plugin }}")

# Check plugin health endpoint
curl http://localhost:{{ plugin_port }}/health
```

## Common Causes

| Symptom | Cause | Fix |
|---|---|---|
| `OOMKilled` in State | Insufficient memory | Increase `MEM_LIMIT_{{ PLUGIN }}` in `.env` |
| `DB connection refused` | Postgres not ready | Increase startup delay / add health check dep |
| `missing env var` | Config not injected | Check `.env.prod` and `nself build` output |
| Panic + stack trace | Code bug | Pin to previous version: `nself plugin install {{ plugin }}@prev` |

## Resolution

1. Read exit reason from Docker inspect.
2. Fix the root cause per table above.
3. Redeploy: `nself build && nself deploy`.
4. Monitor for 10 minutes: `docker stats $(docker ps -qf "name={{ plugin }}")`.
