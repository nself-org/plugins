# S09-T11: Monitoring Plugin Decision

**Date:** 2026-04-17
**Decision:** Keep `monitoring` as `language: "config"` meta-plugin. No Go rewrite.

## Rationale

The `monitoring` plugin in `plugins/free/monitoring/` is an infrastructure orchestration
plugin — it enables the 10-service monitoring stack (Prometheus, Grafana, Loki, Promtail,
Tempo, Alertmanager, cAdvisor, Node Exporter, Postgres Exporter, Redis Exporter) on a
user's nself instance. It has no HTTP API, no database tables, and no runtime logic of its own.

The PPI Go-first policy applies to plugins that run as services with HTTP endpoints. The
monitoring plugin does not run as a service — it configures and starts other services. A Go
rewrite would add a thin HTTP wrapper around docker process management, which provides no
user value and adds build complexity.

F05 (PLUGIN-INVENTORY-MONITORING) documents the 10 monitoring services themselves. The
monitoring plugin in F03 is the install/enable mechanism — it is NOT a duplicate of F05.
Both are needed.

## Action

- No code change. `plugin.json` retains `"language": "config"`.
- No removal from F03 — it is a real, useful plugin.
- Monitoring plugin actions (status, dashboards, alerts, logs) are implemented by the CLI
  directly via `nself monitor` commands, not by the plugin itself.
