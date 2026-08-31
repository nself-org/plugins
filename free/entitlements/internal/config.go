package internal

import "os"

type Config struct {
	DatabaseURL string
	Port        string
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3715"
	}
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}
	return Config{DatabaseURL: dbURL, Port: port}
}
