// Package config reads the project settings this plugin needs.
//
// Purpose: the tenant and billing commands came out of the nSelf CLI, where
// they read a 6,500-line Config struct. A plugin needs ten values from it. This
// is those ten, read from the environment.
//
// Inputs: environment variables, which the CLI resolves from the project's
// .env cascade and passes to this process for every variable the plugin's
// manifest declares in envVars.
//
// Outputs: a Config with the same field names and shapes the commands already
// used, so the moved code did not have to change.
//
// Constraints: this deliberately does NOT read .env files itself. The cascade
// order — which file wins, and in which environment — is defined once, in the
// CLI, and re-implementing it here is exactly the drift CLI-R18 existed to
// remove. If a value is missing, the CLI did not pass it, which means the
// manifest did not declare it.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
)

// Config is the subset of the CLI's project configuration these commands read.
type Config struct {
	ProjectName string
	Postgres    PostgresConfig
	Minio       MinioConfig
	Monitoring  MonitoringConfig
}

// PostgresConfig holds the database connection settings.
type PostgresConfig struct {
	Host     string
	Port     int
	DB       string
	User     string
	Password string
}

// MinioConfig holds object-storage settings.
type MinioConfig struct {
	Enabled      bool
	Port         int
	RootUser     string
	RootPassword string
}

// MonitoringConfig holds the monitoring settings these commands report on.
type MonitoringConfig struct {
	PrometheusEnabled bool
	PrometheusPort    int
}

// Load reads the configuration from the environment.
//
// Defaults match the CLI's, so a value absent here behaves as it did in-tree
// rather than as an empty string.
func Load(string) (*Config, error) {
	cfg := &Config{
		ProjectName: envOr("PROJECT_NAME", "myproject"),
		Postgres: PostgresConfig{
			Host:     envOr("POSTGRES_HOST", "postgres"),
			Port:     envInt("POSTGRES_PORT", 5432),
			DB:       envOr("POSTGRES_DB", "nself"),
			User:     envOr("POSTGRES_USER", "postgres"),
			Password: os.Getenv("POSTGRES_PASSWORD"),
		},
		Minio: MinioConfig{
			// The CLI accepts either name for this one.
			Enabled:      envBool("MINIO_ENABLED", false) || envBool("STORAGE_ENABLED", false),
			Port:         envInt("MINIO_PORT", 0),
			RootUser:     os.Getenv("MINIO_ROOT_USER"),
			RootPassword: os.Getenv("MINIO_ROOT_PASSWORD"),
		},
		Monitoring: MonitoringConfig{
			PrometheusEnabled: envBool("PROMETHEUS_ENABLED", false),
			PrometheusPort:    envInt("PROMETHEUS_PORT", 0),
		},
	}
	return cfg, nil
}

// DatabaseURL returns the PostgreSQL connection string, using internal
// container networking exactly as the CLI does: always port 5432 and the
// configured host, with the password percent-encoded per RFC 3986.
func (cfg *Config) DatabaseURL() string {
	return fmt.Sprintf("postgresql://%s:%s@%s:5432/%s",
		cfg.Postgres.User,
		url.PathEscape(cfg.Postgres.Password),
		cfg.Postgres.Host,
		cfg.Postgres.DB,
	)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}
