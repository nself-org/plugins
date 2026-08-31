package internal

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the access-controls plugin.
type Config struct {
	Port            int
	DatabaseURL     string
	CacheTTLSeconds int
	MaxRoleDepth    int
	DefaultDeny     bool
	APIKey          string
}

// LoadConfig reads configuration from environment variables with sensible defaults.
func LoadConfig() Config {
	return Config{
		Port:            envInt("ACL_PLUGIN_PORT", 3027),
		DatabaseURL:     envStr("DATABASE_URL", ""),
		CacheTTLSeconds: envInt("ACL_CACHE_TTL_SECONDS", 300),
		MaxRoleDepth:    envInt("ACL_MAX_ROLE_DEPTH", 10),
		DefaultDeny:     envBool("ACL_DEFAULT_DENY", true),
		APIKey:          envStr("ACL_API_KEY", ""),
	}
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v := os.Getenv(key); v != "" {
		switch v {
		case "true", "1", "yes":
			return true
		case "false", "0", "no":
			return false
		}
	}
	return def
}
