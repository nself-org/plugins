package internal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds the Shopify plugin configuration loaded from environment variables.
type Config struct {
	AccessToken   string
	ShopDomain    string
	APIVersion    string
	WebhookSecret string
	SyncInterval  int
	Accounts      []AccountConfig
}

// AccountConfig holds configuration for a single Shopify account in multi-account mode.
type AccountConfig struct {
	Label         string
	AccessToken   string
	ShopDomain    string
	WebhookSecret string
}

// BaseURL returns the Shopify Admin API base URL for the primary account.
func (c *Config) BaseURL() string {
	return fmt.Sprintf("https://%s/admin/api/%s", c.ShopDomain, c.APIVersion)
}

// BaseURLFor returns the Shopify Admin API base URL for a specific shop domain.
func (c *Config) BaseURLFor(shopDomain string) string {
	return fmt.Sprintf("https://%s/admin/api/%s", shopDomain, c.APIVersion)
}

// LoadConfig reads Shopify configuration from environment variables.
func LoadConfig() *Config {
	apiVersion := os.Getenv("SHOPIFY_API_VERSION")
	if apiVersion == "" {
		apiVersion = "2024-01"
	}

	syncInterval := 3600
	if v := os.Getenv("SHOPIFY_SYNC_INTERVAL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			syncInterval = n
		}
	}

	cfg := &Config{
		AccessToken:   os.Getenv("SHOPIFY_ACCESS_TOKEN"),
		ShopDomain:    os.Getenv("SHOPIFY_SHOP_DOMAIN"),
		APIVersion:    apiVersion,
		WebhookSecret: os.Getenv("SHOPIFY_WEBHOOK_SECRET"),
		SyncInterval:  syncInterval,
	}

	// Parse multi-account configuration from CSV env vars.
	tokens := splitCSV(os.Getenv("SHOPIFY_ACCESS_TOKENS"))
	domains := splitCSV(os.Getenv("SHOPIFY_SHOP_DOMAINS"))
	labels := splitCSV(os.Getenv("SHOPIFY_ACCOUNT_LABELS"))
	secrets := splitCSV(os.Getenv("SHOPIFY_WEBHOOK_SECRETS"))

	if len(tokens) > 0 && len(tokens) == len(domains) {
		for i := range tokens {
			acc := AccountConfig{
				AccessToken: tokens[i],
				ShopDomain:  domains[i],
			}
			if i < len(labels) {
				acc.Label = labels[i]
			} else {
				acc.Label = fmt.Sprintf("account-%d", i+1)
			}
			if i < len(secrets) {
				acc.WebhookSecret = secrets[i]
			}
			cfg.Accounts = append(cfg.Accounts, acc)
		}
	}

	return cfg
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
