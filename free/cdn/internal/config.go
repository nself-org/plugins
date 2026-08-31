package internal

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for the CDN plugin.
type Config struct {
	Port        int
	Host        string
	DatabaseURL string
	APIKey      string

	// CDN provider tokens
	CloudflareAPIToken string
	BunnyCDNAPIKey     string
	FastlyAPIToken     string
	AkamaiClientToken  string
	AkamaiAccessToken  string
	AkamaiClientSecret string
	AkamaiHost         string

	// Signing
	SigningKey    string
	SignedURLTTL  int // seconds, default 3600

	// Purge
	PurgeBatchSize int
}

// DefaultPort is the reserved port for this plugin.
const DefaultPort = 3036

// LoadConfig reads environment variables and returns a Config.
func LoadConfig() Config {
	port := DefaultPort
	if v := os.Getenv("CDN_PLUGIN_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	} else if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	host := "0.0.0.0"
	if v := os.Getenv("CDN_PLUGIN_HOST"); v != "" {
		host = v
	}

	signedURLTTL := 3600
	if v := os.Getenv("CDN_SIGNED_URL_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			signedURLTTL = n
		}
	}

	purgeBatchSize := 30
	if v := os.Getenv("CDN_PURGE_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			purgeBatchSize = n
		}
	}

	return Config{
		Port:               port,
		Host:               host,
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		APIKey:             os.Getenv("CDN_API_KEY"),
		CloudflareAPIToken: os.Getenv("CDN_CLOUDFLARE_API_TOKEN"),
		BunnyCDNAPIKey:     os.Getenv("CDN_BUNNYCDN_API_KEY"),
		FastlyAPIToken:     os.Getenv("CDN_FASTLY_API_TOKEN"),
		AkamaiClientToken:  os.Getenv("CDN_AKAMAI_CLIENT_TOKEN"),
		AkamaiAccessToken:  os.Getenv("CDN_AKAMAI_ACCESS_TOKEN"),
		AkamaiClientSecret: os.Getenv("CDN_AKAMAI_CLIENT_SECRET"),
		AkamaiHost:         os.Getenv("CDN_AKAMAI_HOST"),
		SigningKey:          os.Getenv("CDN_SIGNING_KEY"),
		SignedURLTTL:        signedURLTTL,
		PurgeBatchSize:     purgeBatchSize,
	}
}
