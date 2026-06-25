package internal

import (
	"fmt"
	"os"
	"strings"
)

// Config holds PayPal plugin configuration loaded from environment variables.
type Config struct {
	ClientID      string
	ClientSecret  string
	Environment   string // "sandbox" or "live"
	WebhookID     string
	WebhookSecret string
	Accounts      []AccountConfig
	SyncInterval  string
}

// AccountConfig holds credentials for a single PayPal account in multi-account setups.
type AccountConfig struct {
	Label        string
	ClientID     string
	ClientSecret string
}

// BaseURL returns the PayPal API base URL based on the configured environment.
func (c *Config) BaseURL() string {
	if c.Environment == "live" {
		return "https://api-m.paypal.com"
	}
	return "https://api-m.sandbox.paypal.com"
}

// LoadConfig reads PayPal configuration from environment variables.
// It returns an error if PAYPAL_CLIENT_IDS and PAYPAL_CLIENT_SECRETS have mismatched counts.
// Size-cap exception: config loader — 54L of env-var reads with validation; single cohesive unit, splitting fragments the config contract.
func LoadConfig() (*Config, error) {
	env := os.Getenv("PAYPAL_ENVIRONMENT")
	if env == "" {
		env = "sandbox"
	}

	cfg := &Config{
		ClientID:      os.Getenv("PAYPAL_CLIENT_ID"),
		ClientSecret:  os.Getenv("PAYPAL_CLIENT_SECRET"),
		Environment:   env,
		WebhookID:     os.Getenv("PAYPAL_WEBHOOK_ID"),
		WebhookSecret: os.Getenv("PAYPAL_WEBHOOK_SECRET"),
		SyncInterval:  os.Getenv("PAYPAL_SYNC_INTERVAL"),
	}

	// Parse multi-account configuration from CSV environment variables.
	ids := splitCSV(os.Getenv("PAYPAL_CLIENT_IDS"))
	secrets := splitCSV(os.Getenv("PAYPAL_CLIENT_SECRETS"))
	labels := splitCSV(os.Getenv("PAYPAL_ACCOUNT_LABELS"))

	// Validate that client IDs and secrets have matching counts.
	if len(ids) > 0 && len(ids) != len(secrets) {
		return nil, fmt.Errorf("PayPal config error: PAYPAL_CLIENT_IDS has %d entries but PAYPAL_CLIENT_SECRETS has %d entries — counts must match", len(ids), len(secrets))
	}
	if len(ids) == 0 && len(secrets) > 0 {
		return nil, fmt.Errorf("PayPal config error: PAYPAL_CLIENT_SECRETS has %d entries but PAYPAL_CLIENT_IDS is empty", len(secrets))
	}

	count := len(ids)

	for i := 0; i < count; i++ {
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		cfg.Accounts = append(cfg.Accounts, AccountConfig{
			Label:        label,
			ClientID:     ids[i],
			ClientSecret: secrets[i],
		})
	}

	// Parse webhook IDs and secrets per account.
	webhookIDs := splitCSV(os.Getenv("PAYPAL_WEBHOOK_IDS"))
	webhookSecrets := splitCSV(os.Getenv("PAYPAL_WEBHOOK_SECRETS"))
	if len(webhookIDs) > 0 && len(webhookIDs) == 1 {
		cfg.WebhookID = webhookIDs[0]
	}
	if len(webhookSecrets) > 0 && len(webhookSecrets) == 1 {
		cfg.WebhookSecret = webhookSecrets[0]
	}

	return cfg, nil
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
