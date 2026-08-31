# nself-audit-analytics

Advanced audit log analytics plugin for nSelf. Provides anomaly detection, user behaviour heatmaps, privileged-action review queues, and real-time webhook/email alerts.

## Features

- **Anomaly Detection** — z-score statistical analysis over `np_audit_log` events. Configurable threshold (default 3.0σ). Catches bulk deletes, after-hours admin access, and unusual access patterns.
- **User Behaviour Heatmaps** — hourly/daily action frequency heatmaps per user and action type. Identifies hot paths and off-schedule activity.
- **Privileged Action Reviews** — queues all privileged actions (admin writes, role changes, data exports) for human review with SLA tracking.
- **Risk Scoring** — composite risk score per user session combining frequency deviation, privilege level, and time-of-day signals.
- **Real-Time Alerts** — optional webhook and email alerts for high-risk anomalies (requires `NSELF_AUDIT_ANALYTICS_REALTIME=true`).

## Port

`3714`

> **Port conflict note:** Port 3714 is also registered for the voice plugin (`ɳClaw` bundle). These two plugins MUST NOT run on the same host simultaneously. Assign `AUDIT_ANALYTICS_PORT` to an alternate port in that scenario. A future F10 registry update will resolve this conflict.

## Requirements

- nSelf CLI >= 1.0.9
- License: ɳSelf+ (set `NSELF_AUDIT_ANALYTICS=true`)
- Postgres with `np_audit_log` table populated by the `audit` plugin

## Installation

```bash
nself plugin install audit-analytics
```

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `DATABASE_URL` | Yes | — | Postgres connection string |
| `NSELF_AUDIT_ANALYTICS` | Yes | `false` | Set to `true` to enable (ɳSelf+ license gate) |
| `AUDIT_ANALYTICS_PORT` | No | `3714` | HTTP listen port |
| `AUDIT_ANALYTICS_SHARED_SECRET` | No | — | Bearer token for endpoint auth (open-dev mode if unset) |
| `NSELF_AUDIT_ANOMALY_ZSCORE` | No | `3.0` | Z-score threshold for anomaly detection |
| `NSELF_AUDIT_ANALYTICS_REALTIME` | No | `false` | Enable real-time alerts |

## API Endpoints

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Liveness check |
| `GET` | `/analytics/anomalies` | List recent anomalies |
| `GET` | `/analytics/heatmap` | User behaviour heatmap data |
| `GET` | `/analytics/risk` | Risk scores by user |
| `GET` | `/analytics/reviews` | Privileged action review queue |
| `POST` | `/analytics/reviews/{id}/approve` | Approve a privileged action |

## Anomaly Detection

Uses z-score analysis over a rolling 24-hour window per user:

- **z ≥ 3.0** — anomaly flagged, stored in `np_audit_anomalies`
- **bulk_delete** — ≥50 DELETE events in 60s regardless of z-score
- **after_hours_admin** — admin writes between 20:00–06:00 UTC

## Multi-App Isolation

All tables include `source_account_id TEXT NOT NULL DEFAULT 'primary'` for multi-app isolation per the nSelf Multi-Tenant Convention Wall.

## Bundle

Unbundled — available as standalone plugin or with ɳSelf+.
