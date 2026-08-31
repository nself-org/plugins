package internal

import (
	"os"
	"strconv"
)

// Config holds all runtime configuration for the devices plugin.
type Config struct {
	Port                    int
	Host                    string
	DatabaseURL             string
	EnrollmentTokenTTL      int // seconds
	ChallengeTTL            int // seconds
	HeartbeatInterval       int // seconds
	HeartbeatTimeout        int // seconds
	CommandDefaultTimeout   int // seconds
	CommandMaxRetries       int
	TelemetryRetentionDays  int
	IngestHeartbeatInterval int // seconds
	IngestHeartbeatTimeout  int // seconds
	IngestRetryMax          int
}

func LoadConfig() Config {
	return Config{
		Port:                    envInt("PORT", 3603),
		Host:                   envStr("HOST", "0.0.0.0"),
		DatabaseURL:             envStr("DATABASE_URL", ""),
		EnrollmentTokenTTL:      envInt("DEV_ENROLLMENT_TOKEN_TTL", 3600),
		ChallengeTTL:            envInt("DEV_CHALLENGE_TTL", 300),
		HeartbeatInterval:       envInt("DEV_HEARTBEAT_INTERVAL", 30),
		HeartbeatTimeout:        envInt("DEV_HEARTBEAT_TIMEOUT", 90),
		CommandDefaultTimeout:   envInt("DEV_COMMAND_DEFAULT_TIMEOUT", 60),
		CommandMaxRetries:       envInt("DEV_COMMAND_MAX_RETRIES", 3),
		TelemetryRetentionDays:  envInt("DEV_TELEMETRY_RETENTION_DAYS", 30),
		IngestHeartbeatInterval: envInt("DEV_INGEST_HEARTBEAT_INTERVAL", 15),
		IngestHeartbeatTimeout:  envInt("DEV_INGEST_HEARTBEAT_TIMEOUT", 45),
		IngestRetryMax:          envInt("DEV_INGEST_RETRY_MAX", 5),
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
