package internal

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds all runtime configuration for the push plugin.
// Loaded once at startup from environment variables; never re-read from env
// after init so that credential rotation requires a container restart
// (documented in push.md — operators rotate by updating env + nself restart push).
type Config struct {
	Port    int
	RedisURL string

	// APNs (Apple Push Notification service)
	APNsTeamID   string
	APNsKeyID    string
	APNsKeyPEM   string // raw PEM content (EC private key, .p8 format)
	APNsBundleID string
	APNsSandbox  bool   // set PUSH_APNS_SANDBOX=1 for development

	// FCM (Firebase Cloud Messaging v1 API)
	FCMProjectID         string
	FCMServiceAccountJSON string // raw JSON content of the service account key

	// Retry policy
	RetryMaxAttempts   int
	RetryBackoffBaseMs int
}

// LoadConfig reads push plugin configuration from environment variables.
// Exits with a clear error message if a required credential is missing and
// at least one platform is expected to be active.
func LoadConfig() *Config {
	cfg := &Config{
		Port:               envInt("PORT", 3053),
		RedisURL:           os.Getenv("REDIS_URL"),
		APNsTeamID:         os.Getenv("PUSH_APNS_TEAM_ID"),
		APNsKeyID:          os.Getenv("PUSH_APNS_KEY_ID"),
		APNsBundleID:       os.Getenv("PUSH_APNS_BUNDLE_ID"),
		APNsSandbox:        os.Getenv("PUSH_APNS_SANDBOX") == "1",
		FCMProjectID:       os.Getenv("PUSH_FCM_PROJECT_ID"),
		RetryMaxAttempts:   envInt("PUSH_RETRY_MAX_ATTEMPTS", 3),
		RetryBackoffBaseMs: envInt("PUSH_RETRY_BACKOFF_BASE_MS", 500),
	}

	// APNs key: accept raw PEM content or a file path.
	// Loading from env is preferred over file path for container deployments.
	cfg.APNsKeyPEM = loadSecretEnv("PUSH_APNS_KEY_PEM")

	// FCM service account: accept raw JSON content or a file path.
	cfg.FCMServiceAccountJSON = loadSecretEnv("PUSH_FCM_SERVICE_ACCOUNT_JSON")

	return cfg
}

// APNsEnabled reports whether APNs credentials are fully configured.
func (c *Config) APNsEnabled() bool {
	return c.APNsTeamID != "" && c.APNsKeyID != "" && c.APNsKeyPEM != "" && c.APNsBundleID != ""
}

// FCMEnabled reports whether FCM credentials are fully configured.
func (c *Config) FCMEnabled() bool {
	return c.FCMProjectID != "" && c.FCMServiceAccountJSON != ""
}

// Validate returns an error if the configuration is not usable.
// Neither APNs nor FCM being configured is valid (plugin starts in degraded
// mode; dispatch handler returns provider-not-configured for appropriate platforms).
func (c *Config) Validate() error {
	if c.RetryMaxAttempts < 1 {
		return fmt.Errorf("PUSH_RETRY_MAX_ATTEMPTS must be >= 1 (got %d)", c.RetryMaxAttempts)
	}
	if c.RetryBackoffBaseMs < 100 {
		return fmt.Errorf("PUSH_RETRY_BACKOFF_BASE_MS must be >= 100ms (got %d)", c.RetryBackoffBaseMs)
	}
	return nil
}

// loadSecretEnv reads a secret from an env var that may contain either:
//   (a) raw content (preferred for containers — set directly in env)
//   (b) a file path — content is read from disk if the value looks like a path
//
// The value is never logged. Returns empty string if unset.
func loadSecretEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		return ""
	}
	// Heuristic: if the value starts with "/" or "./" it's a file path.
	// Otherwise treat as raw content.
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "./") {
		data, err := os.ReadFile(v)
		if err != nil {
			// Bubble a clear error; the caller (main) will exit.
			fmt.Fprintf(os.Stderr, "[push] ERROR: %s points to file %q but read failed: %v\n", key, v, err)
			os.Exit(1)
		}
		return string(data)
	}
	return v
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return def
	}
	return n
}
