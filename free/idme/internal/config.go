package internal

import (
	"os"
)

const DefaultPort = "3010"

type Config struct {
	Port            string
	DatabaseURL     string
	ClientID        string
	ClientSecret    string
	RedirectURI     string
	Scopes          string
	Sandbox         bool
	WebhookSecret   string
}

func LoadConfig() Config {
	port := os.Getenv("PORT")
	if port == "" {
		port = DefaultPort
	}

	scopes := os.Getenv("IDME_SCOPES")
	if scopes == "" {
		scopes = "openid,email,profile"
	}

	return Config{
		Port:          port,
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		ClientID:      os.Getenv("IDME_CLIENT_ID"),
		ClientSecret:  os.Getenv("IDME_CLIENT_SECRET"),
		RedirectURI:   os.Getenv("IDME_REDIRECT_URI"),
		Scopes:        scopes,
		Sandbox:       os.Getenv("IDME_SANDBOX") == "true",
		WebhookSecret: os.Getenv("IDME_WEBHOOK_SECRET"),
	}
}
