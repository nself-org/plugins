package internal

import (
	"os"
	"strconv"
	"strings"
)

// Config holds all configuration for the tokens plugin.
type Config struct {
	EncryptionKey            string
	DefaultTTLSeconds        int
	MaxTTLSeconds            int
	SigningAlgorithm         string
	HLSEncryptionEnabled     bool
	HLSKeyRotationHours      int
	DefaultEntitlementCheck  bool
	AllowAllIfNoEntitlements bool
	ExpiredRetentionDays     int
	AppIDs                   []string
	Host                     string
	Port                     int
}

// LoadConfig reads tokens-specific environment variables and returns a Config.
func LoadConfig() *Config {
	appIDsRaw := envStr("TOKENS_APP_IDS", "primary")
	appIDs := parseCsvList(appIDsRaw)

	return &Config{
		EncryptionKey:            os.Getenv("TOKENS_ENCRYPTION_KEY"),
		DefaultTTLSeconds:        envInt("TOKENS_DEFAULT_TTL_SECONDS", 3600),
		MaxTTLSeconds:            envInt("TOKENS_MAX_TTL_SECONDS", 86400),
		SigningAlgorithm:         envStr("TOKENS_SIGNING_ALGORITHM", "hmac-sha256"),
		HLSEncryptionEnabled:     envBool("TOKENS_HLS_ENCRYPTION_ENABLED", false),
		HLSKeyRotationHours:      envInt("TOKENS_HLS_KEY_ROTATION_HOURS", 168),
		DefaultEntitlementCheck:  envBool("TOKENS_DEFAULT_ENTITLEMENT_CHECK", true),
		AllowAllIfNoEntitlements: envBool("TOKENS_ALLOW_ALL_IF_NO_ENTITLEMENTS", true),
		ExpiredRetentionDays:     envInt("TOKENS_EXPIRED_RETENTION_DAYS", 7),
		AppIDs:                   appIDs,
		Host:                     envStr("TOKENS_PLUGIN_HOST", "0.0.0.0"),
		Port:                     envInt("TOKENS_PLUGIN_PORT", 3107),
	}
}

// envStr reads a string env var with a default fallback.
func envStr(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

// envInt reads an integer env var with a default fallback.
func envInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return n
}

// envBool reads a boolean env var with a default fallback.
func envBool(key string, defaultVal bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	return strings.EqualFold(v, "true") || v == "1"
}

// parseCsvList splits a comma-separated string into trimmed, non-empty parts.
func parseCsvList(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
