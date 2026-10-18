package anomaly

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// AlertConfig describes webhook and email delivery targets.
type AlertConfig struct {
	// WebhookURL, if non-empty, receives a POST for every HIGH/CRITICAL anomaly.
	WebhookURL string
	// Email, if non-empty, receives an email alert.
	Email string
	// EnableRealtime enables delivery on every scored anomaly (Enterprise flag).
	EnableRealtime bool
}

// AlertConfigFromEnv builds AlertConfig from environment variables.
func AlertConfigFromEnv() AlertConfig {
	return AlertConfig{
		WebhookURL:     os.Getenv("NSELF_AUDIT_ALERT_WEBHOOK"),
		Email:          os.Getenv("NSELF_AUDIT_ALERT_EMAIL"),
		EnableRealtime: os.Getenv("NSELF_AUDIT_ANALYTICS_REALTIME") == "true",
	}
}

// webhookPayload is the body sent to the alert webhook.
type webhookPayload struct {
	Event       string                 `json:"event"`
	AnomalyType string                 `json:"anomaly_type"`
	Severity    string                 `json:"severity"`
	UserID      string                 `json:"user_id"`
	ZScore      float64                `json:"z_score"`
	Context     map[string]interface{} `json:"context"`
	DetectedAt  time.Time              `json:"detected_at"`
}

// Alerter dispatches alerts for high/critical anomalies.
type Alerter struct {
	cfg    AlertConfig
	client *http.Client
}

// NewAlerter creates an Alerter.
func NewAlerter(cfg AlertConfig) *Alerter {
	return &Alerter{
		cfg:    cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// MaybeAlert sends an alert if the anomaly meets severity criteria and realtime is enabled.
// HIGH and CRITICAL anomalies are always alerted (subject to ɳSelf+/Enterprise tier check).
// MEDIUM and LOW anomalies are only alerted when EnableRealtime is set (Enterprise).
func (a *Alerter) MaybeAlert(ctx context.Context, anomaly Anomaly) {
	if !a.cfg.EnableRealtime {
		// Only alert HIGH/CRITICAL in non-realtime mode.
		if anomaly.Severity != SeverityHigh && anomaly.Severity != SeverityCritical {
			return
		}
	}

	if a.cfg.WebhookURL != "" {
		if err := a.sendWebhook(ctx, anomaly); err != nil {
			slog.Warn("audit-alert: webhook delivery failed", "err", err)
		}
	}
	// Email delivery is handled by the notify plugin; log intent here.
	if a.cfg.Email != "" {
		slog.Info("audit-alert: email alert queued",
			"severity", anomaly.Severity,
			"type", anomaly.AnomalyType,
			"user", anomaly.UserID,
			"email", a.cfg.Email,
		)
	}
}

// sendWebhook POSTs a JSON payload to the configured webhook URL.
func (a *Alerter) sendWebhook(ctx context.Context, anomaly Anomaly) error {
	payload := webhookPayload{
		Event:       "audit.anomaly.detected",
		AnomalyType: anomaly.AnomalyType,
		Severity:    anomaly.Severity,
		UserID:      anomaly.UserID,
		ZScore:      anomaly.ZScore,
		Context:     anomaly.Context,
		DetectedAt:  time.Now().UTC(),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.cfg.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Nself-Event", "audit.anomaly.detected")

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
