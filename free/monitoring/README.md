# monitoring

Full observability stack for nself: Prometheus, Grafana, Loki, Promtail, Tempo, Alertmanager, and exporters.

## Overview

The `monitoring` plugin deploys a complete monitoring and observability stack alongside your nself services. It includes metrics collection (Prometheus), dashboards (Grafana), log aggregation (Loki + Promtail), distributed tracing (Tempo), alerting (Alertmanager), and exporters for PostgreSQL, Redis, containers, and the host node.

## Installation

```bash
nself plugin install monitoring
```

## Services Included

| Service | Port | Purpose |
|---|---|---|
| Grafana | 3000 | Dashboards and visualization |
| Prometheus | 9090 | Metrics collection and storage |
| Loki | 3100 | Log aggregation |
| Tempo | 3200 | Distributed tracing |
| Alertmanager | 9093 | Alert routing and management |
| cAdvisor | 8082 | Container metrics |
| Node Exporter | 9100 | Host metrics |
| Postgres Exporter | 9187 | PostgreSQL metrics |
| Redis Exporter | 9121 | Redis metrics |

## Configuration

| Variable | Required | Description |
|---|---|---|
| `GRAFANA_ADMIN_USER` | No | Grafana admin username |
| `GRAFANA_ADMIN_PASSWORD` | No | Grafana admin password |
| `GRAFANA_ROUTE` | No | Nginx route for Grafana |
| `PROMETHEUS_ROUTE` | No | Nginx route for Prometheus |
| `ALERTMANAGER_ROUTE` | No | Nginx route for Alertmanager |
| `PROMETHEUS_RETENTION_TIME` | No | Metric retention (default: 15d) |
| `LOKI_RETENTION_PERIOD` | No | Log retention period |

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

## License

MIT
