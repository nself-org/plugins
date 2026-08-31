package internal

import "os"

type Config struct {
	DatabaseURL              string
	Port                     string
	CheckIntervalSeconds     int
	WatchdogTimeoutSeconds   int
	HealthHistoryRetainDays  int
	WatchdogEnabled          bool
}

func LoadConfig() Config {
	port := os.Getenv("OBSERVABILITY_PLUGIN_PORT")
	if port == "" {
		port = os.Getenv("PORT")
	}
	if port == "" {
		port = "3215"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}

	return Config{
		DatabaseURL:             dbURL,
		Port:                    port,
		CheckIntervalSeconds:    envInt("OBSERVABILITY_CHECK_INTERVAL", 30),
		WatchdogTimeoutSeconds:  envInt("OBSERVABILITY_WATCHDOG_TIMEOUT", 120),
		HealthHistoryRetainDays: envInt("OBSERVABILITY_HISTORY_RETAIN_DAYS", 30),
		WatchdogEnabled:         os.Getenv("OBSERVABILITY_WATCHDOG_ENABLED") != "false",
	}
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n := 0
	for _, c := range v {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
