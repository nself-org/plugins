package internal

import "os"

type Config struct {
	DatabaseURL string
	Port        string

	// Provider selects the sports data backend.
	// Values: "apifootball" (default) | "thesportsdb"
	Provider string

	// SyncCronEnabled enables the background cron worker.
	SyncCronEnabled bool
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3126"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}
	prov := os.Getenv("SPORTS_PROVIDER")
	if prov == "" {
		prov = "apifootball"
	}
	cronEnabled := os.Getenv("SPORTS_SYNC_CRON") == "true"

	return Config{
		DatabaseURL:     dbURL,
		Port:            port,
		Provider:        prov,
		SyncCronEnabled: cronEnabled,
	}
}
