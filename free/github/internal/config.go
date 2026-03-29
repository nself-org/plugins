package internal

import (
	"fmt"
	"os"
	"strings"
)

// AccountConfig holds credentials for a single GitHub account.
type AccountConfig struct {
	Label string
	Token string
}

// Config holds all GitHub plugin configuration loaded from environment variables.
type Config struct {
	Token         string
	WebhookSecret string
	Org           string
	Repos         []string
	Accounts      []AccountConfig
}

// LoadConfig reads GitHub plugin configuration from environment variables.
// GITHUB_TOKEN is required. GITHUB_WEBHOOK_SECRET, GITHUB_ORG, GITHUB_REPOS,
// GITHUB_API_KEYS, and GITHUB_ACCOUNT_LABELS are optional.
func LoadConfig() (*Config, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN is required")
	}

	cfg := &Config{
		Token:         token,
		WebhookSecret: os.Getenv("GITHUB_WEBHOOK_SECRET"),
		Org:           os.Getenv("GITHUB_ORG"),
	}

	if repos := os.Getenv("GITHUB_REPOS"); repos != "" {
		for _, r := range strings.Split(repos, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				cfg.Repos = append(cfg.Repos, r)
			}
		}
	}

	apiKeys := os.Getenv("GITHUB_API_KEYS")
	labels := os.Getenv("GITHUB_ACCOUNT_LABELS")

	if apiKeys != "" {
		keys := strings.Split(apiKeys, ",")
		var lbls []string
		if labels != "" {
			lbls = strings.Split(labels, ",")
		}

		for i, key := range keys {
			key = strings.TrimSpace(key)
			if key == "" {
				continue
			}
			label := fmt.Sprintf("account_%d", i+1)
			if i < len(lbls) {
				l := strings.TrimSpace(lbls[i])
				if l != "" {
					label = l
				}
			}
			cfg.Accounts = append(cfg.Accounts, AccountConfig{
				Label: label,
				Token: key,
			})
		}
	}

	return cfg, nil
}
