# Audit Analytics Plugin

> Advanced audit analytics over np_audit_log: anomaly detection (z-score baseline), user behaviour heatmaps, privileged-action review queue, and webhook/email alerts. **Free — MIT licensed.**

## Install

```bash
nself plugin install audit-analytics
```

No license key required.

## Description

Advanced audit analytics over np_audit_log: anomaly detection (z-score baseline), user behaviour heatmaps, privileged-action review queue, and webhook/email alerts. ɳSelf+ required. Free tier retains full audit capture and basic search.

Category: `compliance`. Current version: `1.0.0`.

## Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `DATABASE_URL` | `(required)` | PostgreSQL connection string |
| `NSELF_AUDIT_ANALYTICS` | `-` | - |
| `NSELF_AUDIT_ANALYTICS_REALTIME` | `-` | - |
| `NSELF_AUDIT_ANOMALY_ZSCORE` | `-` | - |
| `NSELF_AUDIT_HEATMAP_REFRESH` | `-` | - |
| `NSELF_AUDIT_PRIVILEGED_REVIEW_TTL` | `-` | - |
| `NSELF_AUDIT_ALERT_WEBHOOK` | `-` | - |
| `NSELF_AUDIT_ALERT_EMAIL` | `-` | - |
| `AUDIT_ANALYTICS_PORT` | `3218` | - |
| `AUDIT_ANALYTICS_HOST` | `-` | - |
| `AUDIT_ANALYTICS_LOG_LEVEL` | `-` | - |

## Ports

| Port | Purpose |
|------|---------|
| 3218 | Audit Analytics service port |

## Database Schema

2 table(s) added to your Postgres database:

- `np_audit_anomalies`
- `np_audit_privileged_reviews`

## REST API

```
GET    /health
GET    /audit/analytics/anomalies
GET    /audit/analytics/anomalies/{id}
PATCH  /audit/analytics/anomalies/{id}
GET    /audit/analytics/heatmap
GET    /audit/analytics/top-actors
GET    /audit/privileged-reviews
POST   /audit/privileged-reviews/{id}
GET    /audit/privileged-reviews/overdue
POST   /audit/analytics/refresh
GET    /audit/analytics/status
```

## Examples

### Health check

```bash
curl http://localhost:3218/health
```

## Source

[`plugins/audit-analytics/`](https://github.com/nself-org/plugins/tree/main/audit-analytics)

Manifest: [`plugins/audit-analytics/plugin.json`](https://github.com/nself-org/plugins/tree/main/audit-analytics/plugin.json)

## See Also

- [[Plugin-Marketplace]] — full plugin index
- [[Plugin-Development]] — write your own plugin

← [[Home]] →
