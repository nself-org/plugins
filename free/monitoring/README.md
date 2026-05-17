# monitoring

Full observability stack for nSelf: Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, cAdvisor, Node Exporter, Postgres Exporter, and Redis Exporter.

**Tier:** Free (MIT) — no license required.

## Installation

```bash
nself plugin install monitoring
nself build
nself start
```

## Overview

The `monitoring` plugin deploys a complete, pre-configured observability stack alongside your nSelf services. It covers metrics (Prometheus), dashboards (Grafana), log aggregation (Loki + Promtail), distributed tracing (Tempo), alerting (Alertmanager), and exporters for containers, host, PostgreSQL, and Redis.

This is a **config-type plugin** — it orchestrates Docker services via compose fragments. No separate nSelf Go service binary. `nself build` injects all 10 monitoring services into your compose stack.

## Services

| Service | Port | Purpose |
|---|---|---|
| Grafana | 3000 | Dashboards and visualization — pre-loaded nSelf dashboards |
| Prometheus | 9090 | Metrics collection, storage, and query engine |
| Loki | 3100 | Log aggregation and query (LogQL) |
| Promtail | — | Log shipper — tails Docker logs into Loki |
| Tempo | 3200 | Distributed tracing — OpenTelemetry-compatible |
| Alertmanager | 9093 | Alert routing, grouping, silencing, and notifications |
| cAdvisor | 8082 | Per-container resource metrics (CPU, RAM, I/O, net) |
| Node Exporter | 9100 | Host metrics (CPU, disk, memory, network) |
| Postgres Exporter | 9187 | PostgreSQL metrics (connections, queries, replication lag) |
| Redis Exporter | 9121 | Redis metrics (memory, commands, keyspace, connected clients) |

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `MONITORING_ENABLED` | No | `true` | Enable/disable the monitoring stack |
| `GRAFANA_ENABLED` | No | `true` | Enable/disable Grafana |
| `PROMETHEUS_ENABLED` | No | `true` | Enable/disable Prometheus |
| `LOKI_ENABLED` | No | `true` | Enable/disable Loki log aggregation |
| `TEMPO_ENABLED` | No | `true` | Enable/disable Tempo tracing |
| `ALERTMANAGER_ENABLED` | No | `true` | Enable/disable Alertmanager |
| `GRAFANA_ADMIN_USER` | No | `admin` | Grafana admin username |
| `GRAFANA_ADMIN_PASSWORD` | No | `admin` | Grafana admin password (change in production) |
| `GRAFANA_ROUTE` | No | `/grafana` | Nginx route to proxy Grafana |
| `PROMETHEUS_ROUTE` | No | `/prometheus` | Nginx route to proxy Prometheus |
| `ALERTMANAGER_ROUTE` | No | `/alertmanager` | Nginx route to proxy Alertmanager |
| `PROMETHEUS_RETENTION_TIME` | No | `15d` | Metric retention period |
| `LOKI_RETENTION_PERIOD` | No | `744h` | Log retention period (default: 31 days) |
| `REDIS_ENABLED` | No | `false` | Enable Redis Exporter (requires Redis in the stack) |

## Usage

```bash
# Show status of all monitoring services
nself plugin run monitoring status

# List available Grafana dashboards
nself plugin run monitoring dashboards

# Show active alerts from Alertmanager
nself plugin run monitoring alerts

# Tail logs from monitoring services
nself plugin run monitoring logs
```

## Grafana Dashboards

The plugin pre-loads nSelf-specific dashboards for:

- **nSelf Overview** — all services health, request rates, error rates
- **PostgreSQL** — query stats, connection pool, lock waits, replication
- **Redis** — memory usage, evictions, hit/miss ratio, command throughput
- **Docker Containers** — per-container CPU, memory, I/O
- **Host Node** — system CPU, disk, memory, network
- **Hasura** — GraphQL request rates, errors, subscription counts

Access Grafana at `http://127.0.0.1:3000` or via your configured `GRAFANA_ROUTE`.

## Alerting

Alertmanager handles alert routing. To configure receivers (email, Slack, PagerDuty):

```bash
# Edit alertmanager config in your nself stack config directory
# Then rebuild:
nself build
nself restart alertmanager
```

Built-in alert rules cover: service down, high error rate, disk space low, PostgreSQL connection saturation, high memory usage.

## Tracing

Services that emit OpenTelemetry traces send them to Tempo at `http://tempo:4317` (gRPC) or `http://tempo:4318` (HTTP). Grafana's Tempo data source is pre-configured for trace exploration and trace-to-log correlation with Loki.

## Port Summary

All monitoring ports bind to `127.0.0.1`. Access via Nginx routes or direct localhost.

| Service | Local URL |
|---|---|
| Grafana | http://127.0.0.1:3000 |
| Prometheus | http://127.0.0.1:9090 |
| Loki | http://127.0.0.1:3100 |
| Tempo | http://127.0.0.1:3200 |
| Alertmanager | http://127.0.0.1:9093 |

## See also

- [plugin-audit-log](plugin-audit-log.md) — security audit log (free, always available)
- [ɳSentry bundle](bundle-nsentry.md) — product-layer observability: uptime, status pages, incident management (planned v1.1.0)
- [nSelf CLI: nself plugin](cmd-plugin.md) — plugin management
