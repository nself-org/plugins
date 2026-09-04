# Compliance Plugin

> Compliance and audit platform covering GDPR, CCPA, HIPAA, SOC 2, and PCI. DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and reporting.

**Tier:** Free (MIT) — no license required.

## Install

```bash
nself plugin install compliance
nself build
```

## Description

Compliance and audit platform covering GDPR, CCPA, HIPAA, SOC 2, and PCI. DSARs, consent tracking, data retention, breach notification, immutable audit logging, SIEM integration, and reporting.

Toggle frameworks (GDPR, CCPA, HIPAA, SOC 2, PCI) per project, then issue DSARs, capture consent, enforce retention, and generate immutable audit trails. SIEM integration ships out of the box for the major providers; breach notification workflows trigger via the `notify` plugin.

## Configuration

| Env Var | Required | Default | Description |
|---------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Required env var |
| `COMPLIANCE_PLUGIN_PORT` | No | — | Optional override |
| `COMPLIANCE_PLUGIN_HOST` | No | — | Optional override |
| `COMPLIANCE_APP_IDS` | No | — | Optional override |
| `COMPLIANCE_LOG_LEVEL` | No | — | Optional override |
| `COMPLIANCE_GDPR_ENABLED` | No | — | Optional override |
| `COMPLIANCE_CCPA_ENABLED` | No | — | Optional override |
| `COMPLIANCE_HIPAA_ENABLED` | No | — | Optional override |
| `COMPLIANCE_SOC2_ENABLED` | No | — | Optional override |
| `COMPLIANCE_PCI_ENABLED` | No | — | Optional override |
| `COMPLIANCE_DSAR_DEADLINE_DAYS` | No | — | Optional override |
| `COMPLIANCE_BREACH_NOTIFICATION_HOURS` | No | — | Optional override |
| `COMPLIANCE_CONSENT_REQUIRED` | No | — | Optional override |
| `COMPLIANCE_CONSENT_EXPIRY_DAYS` | No | — | Optional override |
| `COMPLIANCE_RETENTION_ENABLED` | No | — | Optional override |
| `COMPLIANCE_AUDIT_ENABLED` | No | — | Optional override |
| `COMPLIANCE_AUDIT_RETENTION_DAYS` | No | — | Optional override |
| `COMPLIANCE_EXPORT_FORMAT` | No | — | Optional override |
| `COMPLIANCE_EXPORT_EXPIRY_HOURS` | No | — | Optional override |
| `COMPLIANCE_EXPORT_MAX_ROWS` | No | — | Optional override |
| `COMPLIANCE_API_KEY` | No | — | Optional override |
| `COMPLIANCE_RATE_LIMIT_MAX` | No | — | Optional override |
| `COMPLIANCE_RATE_LIMIT_WINDOW_MS` | No | — | Optional override |
| `AUDIT_FALLBACK_LOG_PATH` | No | — | Optional override |
| `AUDIT_SIEM_SPLUNK_HEC_URL` | No | — | Optional override |
| `AUDIT_SIEM_SPLUNK_HEC_TOKEN` | No | — | Optional override |
| `AUDIT_SIEM_ELK_URL` | No | — | Optional override |
| `AUDIT_SIEM_ELK_INDEX` | No | — | Optional override |
| `AUDIT_SIEM_ELK_API_KEY` | No | — | Optional override |
| `AUDIT_SIEM_DATADOG_API_KEY` | No | — | Optional override |
| `AUDIT_SIEM_DATADOG_SITE` | No | — | Optional override |
| `AUDIT_DEFAULT_RETENTION_DAYS` | No | — | Optional override |
| `AUDIT_COMPLIANCE_FRAMEWORKS` | No | — | Optional override |
| `AUDIT_ALERT_WEBHOOK_URL` | No | — | Optional override |

Reference vault credentials. Never hardcode secrets.

## Ports

- Default port: `3211`
- Bound to `127.0.0.1` per nSelf service-binding rules; reach via Nginx, never directly.

## Database Schema

Tables created (prefix `np_`):

- `np_compliance_dsars`
- `np_compliance_dsar_activities`
- `np_compliance_consents`
- `np_compliance_consent_history`
- `np_compliance_privacy_policies`
- `np_compliance_policy_acceptances`
- `np_compliance_retention_policies`
- `np_compliance_retention_executions`
- `np_compliance_processing_records`
- `np_compliance_data_processors`
- `np_compliance_data_breaches`
- `np_compliance_breach_notifications`
- `np_compliance_audit_log`
- `np_compliance_audit_events`
- `np_compliance_audit_retention_policies`
- `np_compliance_audit_alert_rules`
- `np_compliance_audit_webhook_events`

All tables use `source_account_id` for multi-app isolation where applicable.

## REST API

Public endpoints exposed by the plugin. Internal admin endpoints exist but are not part of the public surface.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness probe |
| GET | `/` | Plugin index / capability list |

Refer to the plugin's OpenAPI spec (under `paid/compliance/`) for the full route list.

## Examples

File a Data Subject Access Request:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/compliance/dsars -d '{"subject_email":"u@example.com","type":"access"}'
```

Record consent:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/compliance/consents -d '{"subject_id":"u_xxx","purpose":"marketing","granted":true}'
```

Run retention policy now:

```bash
curl -X POST -H 'Authorization: Bearer $TOKEN' https://api.example.com/compliance/retention/run -d '{"policy_id":"ret_xxx"}'
```


## Source

MIT licensed, source included in this repository: [`free/compliance/`](https://github.com/nself-org/plugins/tree/main/free/compliance)

## Distinct from the `hipaa` plugin

This plugin covers multi-regulation consent and DSAR workflows (GDPR, CCPA, HIPAA, SOC 2, PCI). The separate `hipaa` plugin is narrower and focuses on the PHI column registry. Use `compliance` for cross-regulation consent, retention, and DSAR management; use `hipaa` when you need PHI column-level tracking.

## See Also

- `.github/docs/licensing/bundles.md` for bundle membership reference
- `.github/docs/licensing.md` for the 7-tier pricing matrix
- `.github/docs/bundles.md` for the public-facing bundle guide
- `plugin.json` in this directory for the canonical manifest
