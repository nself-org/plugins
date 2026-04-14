# SLO Definitions

Seven SLOs with multi-window multi-burn-rate (MWMBR) alerting.
Review quarterly. Track in `.claude/memory/slo-review-YYYY-QN.md`.

## SLO Table

| # | SLO | Target | Window | Budget | Alert Windows |
|---|-----|--------|--------|--------|---------------|
| 1 | Platform Uptime | 99.9% | 30d | 43.2 min | 5m/1h, 30m/6h |
| 2 | Chat P95 Latency | < 2s | 28d | 0.1% of requests | 5m/1h, 30m/6h |
| 3 | API P99 Latency | < 500ms | 28d (excl. AI) | 0.1% of requests | 5m/1h, 30m/6h |
| 4 | Mux Classification F1 | > 0.85 | 7d | 15% error budget | 5m/1h, 30m/6h |
| 5 | Backup Success Rate | 100% | 30d | 0 failures | 5m/1h, 30m/6h |
| 6 | AI Call Success | > 99% | 7d | 1% error budget | 5m/1h, 30m/6h |
| 7 | Auth Success | > 99.5% | 28d | 0.5% error budget | 5m/1h, 30m/6h |

## Burn Rate Windows

Fast-burn (pages immediately): 5m and 1h windows, burn rate 14.4x.
Slow-burn (warns before budget exhaustion): 30m and 6h windows, burn rate 6x.

Both conditions must fire simultaneously (AND logic) to reduce false positives.

## Error Budget Calculation

Each SLO has a recording rule in `slo-recording.yml` that tracks remaining error budget
as a ratio (1.0 = full, 0.0 = exhausted). Visualized in Grafana dashboard `12-slo.json`.

## Files

- Alert rules: `prometheus/rules/slo-alerts.yml`
- Recording rules: `prometheus/rules/slo-recording.yml`
- Dashboard: `grafana/dashboards/12-slo.json`
