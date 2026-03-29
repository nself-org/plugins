package internal

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// StripeAccountConfig holds per-account Stripe credentials.
type StripeAccountConfig struct {
	ID            string
	APIKey        string
	WebhookSecret string
}

// Config holds the full plugin configuration loaded from environment variables.
type Config struct {
	// Stripe
	StripeAPIKey        string
	StripeWebhookSecret string
	StripeAccounts      []StripeAccountConfig

	// Server
	Port int
	Host string

	// Database
	DatabaseURL string
}

var validAPIKeyRe = regexp.MustCompile(`^(sk_|rk_)(test_|live_)`)

// LoadConfig reads configuration from environment variables.
// Required: DATABASE_URL, STRIPE_API_KEY (or STRIPE_API_KEYS).
func LoadConfig() (*Config, error) {
	accounts, err := buildStripeAccountsFromEnv()
	if err != nil {
		return nil, err
	}

	port := 3070
	if v := os.Getenv("PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid PORT value: %s", v)
		}
		port = p
	}

	host := "0.0.0.0"
	if v := os.Getenv("HOST"); v != "" {
		host = v
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	primary := accounts[0]

	cfg := &Config{
		StripeAPIKey:        primary.APIKey,
		StripeWebhookSecret: primary.WebhookSecret,
		StripeAccounts:      accounts,
		Port:                port,
		Host:                host,
		DatabaseURL:         databaseURL,
	}

	return cfg, nil
}

// IsTestMode returns true if the API key is a test mode key.
func IsTestMode(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk_test_") || strings.HasPrefix(apiKey, "rk_test_")
}

func buildStripeAccountsFromEnv() ([]StripeAccountConfig, error) {
	multiAPIKeys := parseCsvList(os.Getenv("STRIPE_API_KEYS"))
	multiLabels := parseCsvList(os.Getenv("STRIPE_ACCOUNT_LABELS"))
	multiSecrets := parseCsvList(os.Getenv("STRIPE_WEBHOOK_SECRETS"))

	if len(multiLabels) > 0 && len(multiLabels) != len(multiAPIKeys) {
		return nil, fmt.Errorf("STRIPE_ACCOUNT_LABELS length must match STRIPE_API_KEYS length")
	}
	if len(multiSecrets) > 0 && len(multiSecrets) != len(multiAPIKeys) {
		return nil, fmt.Errorf("STRIPE_WEBHOOK_SECRETS length must match STRIPE_API_KEYS length")
	}

	if len(multiAPIKeys) > 0 {
		accounts := make([]StripeAccountConfig, len(multiAPIKeys))
		seen := make(map[string]bool)
		for i, key := range multiAPIKeys {
			label := fmt.Sprintf("account-%d", i+1)
			if i < len(multiLabels) {
				label = multiLabels[i]
			}
			id := normalizeAccountID(label, i)
			if seen[id] {
				return nil, fmt.Errorf("duplicate Stripe account id %q in configuration", id)
			}
			seen[id] = true

			if !validAPIKeyRe.MatchString(key) {
				return nil, fmt.Errorf("invalid Stripe API key format for account %q; expected sk_test_*, sk_live_*, rk_test_*, or rk_live_*", id)
			}

			secret := ""
			if i < len(multiSecrets) {
				secret = multiSecrets[i]
			}
			accounts[i] = StripeAccountConfig{ID: id, APIKey: key, WebhookSecret: secret}
		}
		return accounts, nil
	}

	singleKey := os.Getenv("STRIPE_API_KEY")
	if singleKey == "" {
		return nil, fmt.Errorf("either STRIPE_API_KEY or STRIPE_API_KEYS must be set")
	}
	if !validAPIKeyRe.MatchString(singleKey) {
		return nil, fmt.Errorf("invalid Stripe API key format; expected sk_test_*, sk_live_*, rk_test_*, or rk_live_*")
	}

	singleSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	accountID := normalizeAccountID(os.Getenv("STRIPE_ACCOUNT_ID"), 0)
	if accountID == "" {
		accountID = "primary"
	}

	return []StripeAccountConfig{
		{ID: accountID, APIKey: singleKey, WebhookSecret: singleSecret},
	}, nil
}

var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9_-]+`)
var leadTrailDash = regexp.MustCompile(`^-+|-+$`)

func normalizeAccountID(value string, index int) string {
	normalized := strings.ToLower(value)
	normalized = nonAlphanumRe.ReplaceAllString(normalized, "-")
	normalized = leadTrailDash.ReplaceAllString(normalized, "")
	if normalized == "" {
		return fmt.Sprintf("account-%d", index+1)
	}
	return normalized
}

func parseCsvList(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	var result []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
