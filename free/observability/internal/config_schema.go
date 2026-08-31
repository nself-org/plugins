// Package internal: dynamic config schema endpoints (OpenClaw pattern).
package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

const monitoringConfigSchema = `{
  "$schema": "https://json-schema.org/draft-07/schema#",
  "title": "Monitoring Plugin Configuration",
  "description": "Configuration for the nself-observability monitoring plugin (alert rules, silences, watchdog).",
  "type": "object",
  "properties": {
    "watchdog": {
      "type": "object",
      "properties": {
        "enabled":                {"type": "boolean", "default": true},
        "check_interval_seconds": {"type": "integer", "minimum": 5, "default": 60}
      }
    },
    "alert_rules": {
      "type": "array",
      "description": "Alert rules. Each rule fires when its condition matches.",
      "items": {
        "type": "object",
        "properties": {
          "name":      {"type": "string"},
          "metric":    {"type": "string"},
          "condition": {"type": "string", "enum": [">", "<", ">=", "<=", "==", "!="]},
          "threshold": {"type": "number"},
          "for_secs":  {"type": "integer", "minimum": 0},
          "channels":  {"type": "array", "items": {"type": "string"}},
          "severity":  {"type": "string", "enum": ["info", "warning", "critical"]}
        },
        "required": ["name", "metric", "condition", "threshold"]
      }
    },
    "silences": {
      "type": "array",
      "description": "Silenced alerts (matched by name or label).",
      "items": {
        "type": "object",
        "properties": {
          "match":     {"type": "string"},
          "until":     {"type": "string", "format": "date-time"},
          "reason":    {"type": "string"}
        },
        "required": ["match", "until"]
      }
    }
  }
}`

const monitoringAIInstructions = `plugin: monitoring
description: |
  Monitoring is the observability plugin: service uptime checks, alert rules, silences,
  and a watchdog loop. Use it to track services, define alert thresholds, and route
  incidents to notification channels.
capabilities:
  - service_uptime_checks
  - alert_rules
  - silences
  - incident_history
  - watchdog_loop
  - health_summary
setup_flow:
  - step: choose_targets
    prompt: "What services or URLs should I monitor?"
    action: register_services
  - step: define_alerts
    prompt: "When should I alert? (e.g. 'response time > 500ms for 1 min', 'service down for 30s')"
    action: create_alert_rules
  - step: choose_channels
    prompt: "Where should alerts go? (telegram, slack, email, discord)"
    action: set_alert_channels
  - step: confirm
    prompt: "Saving monitoring config. Confirm?"
    action: put_config
examples:
  - user: "Monitor my server and alert me on Telegram if it goes down"
    actions:
      - tool: plugin_update_config
        args:
          plugin: monitoring
          config:
            alert_rules:
              - name: server_down
                metric: up
                condition: "=="
                threshold: 0
                for_secs: 30
                channels: [telegram]
                severity: critical
  - user: "Silence alerts during maintenance window tonight"
    actions:
      - tool: plugin_update_config
        args:
          plugin: monitoring
          config:
            silences:
              - match: "*"
                until: "2026-04-08T06:00:00Z"
                reason: "scheduled maintenance"
`

// HandleMonitoringConfigSchema returns the JSON Schema for monitoring configuration.
func HandleMonitoringConfigSchema(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(monitoringConfigSchema)) //nolint:errcheck
}

// HandleMonitoringAIInstructions returns the AI instruction YAML.
func HandleMonitoringAIInstructions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(monitoringAIInstructions)) //nolint:errcheck
}

// HandleMonitoringGetConfig returns the current monitoring runtime config.
func (h *Handlers) HandleMonitoringGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := loadMonitoringConfig(r.Context(), h)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(cfg) //nolint:errcheck
}

// HandleMonitoringPutConfig validates and stores a new monitoring runtime config.
func (h *Handlers) HandleMonitoringPutConfig(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}
	if !json.Valid(body) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := saveMonitoringConfig(r.Context(), h, body); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body) //nolint:errcheck
}

func loadMonitoringConfig(ctx context.Context, h *Handlers) (json.RawMessage, error) {
	if _, err := h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_monitoring_config (
			key TEXT PRIMARY KEY,
			value JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return nil, err
	}
	var raw []byte
	err := h.pool.QueryRow(ctx, `SELECT value FROM np_monitoring_config WHERE key = 'runtime'`).Scan(&raw)
	if err != nil {
		return json.RawMessage(`{"watchdog":{"enabled":true,"check_interval_seconds":60},"alert_rules":[],"silences":[]}`), nil
	}
	return json.RawMessage(raw), nil
}

func saveMonitoringConfig(ctx context.Context, h *Handlers, value json.RawMessage) error {
	if _, err := h.pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS np_monitoring_config (
			key TEXT PRIMARY KEY,
			value JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`); err != nil {
		return err
	}
	_, err := h.pool.Exec(ctx, `
		INSERT INTO np_monitoring_config (key, value, updated_at) VALUES ('runtime', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`, value)
	return err
}
