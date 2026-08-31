# Monitoring Plugin

> The full observability stack: Prometheus, Grafana, Loki, Promtail, Tempo, and Alertmanager. **Free — MIT licensed.**

## Install

```bash
nself plugin install monitoring
```

No license key required.

## Description

Full monitoring stack: Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, and exporters

This plugin runs as its own container in your nSelf stack (rebuild with `nself build && nself start` after install).

Category: `infrastructure`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `MONITORING_ENABLED` | *(see plugin.json)* | Optional. |
| `GRAFANA_ENABLED` | *(see plugin.json)* | Optional. |
| `PROMETHEUS_ENABLED` | *(see plugin.json)* | Optional. |
| `LOKI_ENABLED` | *(see plugin.json)* | Optional. |
| `TEMPO_ENABLED` | *(see plugin.json)* | Optional. |
| `ALERTMANAGER_ENABLED` | *(see plugin.json)* | Optional. |
| `GRAFANA_ADMIN_USER` | *(see plugin.json)* | Optional. |
| `GRAFANA_ADMIN_PASSWORD` | — | Optional. |
| `GRAFANA_ROUTE` | *(see plugin.json)* | Optional. |
| `PROMETHEUS_ROUTE` | *(see plugin.json)* | Optional. |
| `ALERTMANAGER_ROUTE` | *(see plugin.json)* | Optional. |
| `PROMETHEUS_RETENTION_TIME` | *(see plugin.json)* | Optional. |
| `LOKI_RETENTION_PERIOD` | *(see plugin.json)* | Optional. |
| `REDIS_ENABLED` | *(see plugin.json)* | Optional. |

## REST API

```
Prometheus, Grafana, Loki, Tempo and Alertmanager each expose their own upstream REST/query APIs at their configured routes
```

## Nginx Routes

| Route | Target |
|-------|--------|
| `/grafana/` | Grafana |
| `/prometheus/` | Prometheus |
| `/alertmanager/` | Alertmanager |

## Examples

### Check health

```bash
docker ps | grep monitoring
```

## Source

[`plugins/monitoring/`](https://github.com/nself-org/plugins/tree/main/monitoring)

Manifest: [`plugins/monitoring/plugin.json`](https://github.com/nself-org/plugins/tree/main/monitoring/plugin.json)

## See Also

- [[Monitor]] — upgrade Grafana dashboards
- [[Alerts]] — Prometheus alert rules and silences
- [[Watchdog]] — self-healing container watchdog

← [[Home]] →
