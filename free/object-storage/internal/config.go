package internal

import (
	"os"
)

// Config holds all runtime configuration for the object-storage plugin.
type Config struct {
	Port              string
	DatabaseURL       string
	StorageEndpoint   string
	StorageAccessKey  string
	StorageSecretKey  string
	StorageBasePath   string
	DefaultProvider   string
	APIKey            string
	MaxUploadSize     string
}

// LoadConfig reads configuration from environment variables with sensible defaults.
func LoadConfig() *Config {
	return &Config{
		Port:             getEnv("OS_PLUGIN_PORT", "3301"),
		DatabaseURL:      getEnv("DATABASE_URL", ""),
		StorageEndpoint:  getEnv("OS_S3_ENDPOINT", ""),
		StorageAccessKey: getEnv("OS_S3_ACCESS_KEY", ""),
		StorageSecretKey: getEnv("OS_S3_SECRET_KEY", ""),
		StorageBasePath:  getEnv("OS_STORAGE_BASE_PATH", "/data/object-storage"),
		DefaultProvider:  getEnv("OS_DEFAULT_PROVIDER", "local"),
		APIKey:           getEnv("OS_API_KEY", ""),
		MaxUploadSize:    getEnv("OS_MAX_UPLOAD_SIZE", "1073741824"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
