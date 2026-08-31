// Package config loads and validates plugin configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all payments plugin configuration.
type Config struct {
	Port            int
	DatabaseURL     string
	DefaultProvider string // stripe|lemonsqueezy|paddle

	// Stripe
	StripeSecretKey     string
	StripeWebhookSecret string
	StripePriceIDs      string // JSON map

	// LemonSqueezy
	LemonSqueezyAPIKey       string
	LemonSqueezyStoreID      string
	LemonSqueezyWebhookSecret string

	// Paddle
	PaddleAPIKey       string
	PaddleWebhookSecret string
	PaddlePublicKey    string

	// Dunning
	DunningGraceDays      int
	DunningRetryIntervals []int // days between retries

	// Internal
	InternalSecret string
	LogLevel       string
}

// Load reads configuration from environment variables.
func Load() *Config {
	cfg := &Config{
		Port:            envInt("PAYMENTS_PORT", 3086),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		DefaultProvider: envStr("PAYMENTS_DEFAULT_PROVIDER", "stripe"),

		StripeSecretKey:     os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret: os.Getenv("STRIPE_WEBHOOK_SECRET"),
		StripePriceIDs:      os.Getenv("STRIPE_PRICE_IDS"),

		LemonSqueezyAPIKey:       os.Getenv("LEMONSQUEEZY_API_KEY"),
		LemonSqueezyStoreID:      os.Getenv("LEMONSQUEEZY_STORE_ID"),
		LemonSqueezyWebhookSecret: os.Getenv("LEMONSQUEEZY_WEBHOOK_SECRET"),

		PaddleAPIKey:        os.Getenv("PADDLE_API_KEY"),
		PaddleWebhookSecret: os.Getenv("PADDLE_WEBHOOK_SECRET"),
		PaddlePublicKey:     os.Getenv("PADDLE_PUBLIC_KEY"),

		DunningGraceDays:      envInt("PAYMENTS_DUNNING_GRACE_DAYS", 7),
		DunningRetryIntervals: parseIntList(os.Getenv("PAYMENTS_DUNNING_RETRY_INTERVALS"), []int{1, 3, 7}),

		InternalSecret: os.Getenv("PLUGIN_INTERNAL_SECRET"),
		LogLevel:       envStr("LOG_LEVEL", "info"),
	}
	return cfg
}

// Validate checks required configuration is present.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if c.DefaultProvider == "" {
		return fmt.Errorf("PAYMENTS_DEFAULT_PROVIDER is required")
	}
	switch c.DefaultProvider {
	case "stripe":
		if c.StripeSecretKey == "" {
			return fmt.Errorf("STRIPE_SECRET_KEY is required when PAYMENTS_DEFAULT_PROVIDER=stripe")
		}
	case "lemonsqueezy":
		if c.LemonSqueezyAPIKey == "" {
			return fmt.Errorf("LEMONSQUEEZY_API_KEY is required when PAYMENTS_DEFAULT_PROVIDER=lemonsqueezy")
		}
	case "paddle":
		if c.PaddleAPIKey == "" {
			return fmt.Errorf("PADDLE_API_KEY is required when PAYMENTS_DEFAULT_PROVIDER=paddle")
		}
	default:
		return fmt.Errorf("PAYMENTS_DEFAULT_PROVIDER must be one of: stripe, lemonsqueezy, paddle")
	}
	return nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func parseIntList(s string, def []int) []int {
	if s == "" {
		return def
	}
	parts := strings.Split(s, ",")
	result := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		n, err := strconv.Atoi(p)
		if err != nil {
			continue
		}
		result = append(result, n)
	}
	if len(result) == 0 {
		return def
	}
	return result
}
