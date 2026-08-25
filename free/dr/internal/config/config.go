// Package config reads the project settings this plugin needs.
//
// Purpose: the dr command came out of the nSelf CLI, where it read a
// 6,500-line Config struct. Disaster recovery needs five values from it: the
// project name, the base domain, the backup directory, the standby host, and
// whether Redis is enabled.
//
// Inputs: environment variables, which the CLI resolves from the project's
// .env cascade and passes to this process for every variable declared in the
// plugin's manifest.
//
// Outputs: a Config with the field names and shapes the moved code used.
//
// Constraints: this deliberately does NOT read .env files itself. The cascade
// order is defined once, in the CLI (CLI-R18); a second implementation here is
// the drift that rule exists to prevent.
package config

import (
	"os"
	"strconv"
)

// Config is the subset of project configuration disaster recovery reads.
type Config struct {
	ProjectName string
	BaseDomain  string
	Backup      BackupConfig
	DR          DRConfig
	Redis       RedisConfig
}

// BackupConfig holds where backups are written.
type BackupConfig struct {
	Dir string
}

// DRConfig holds the standby this project fails over to.
type DRConfig struct {
	StandbyHost string
}

// RedisConfig holds whether Redis is part of this stack.
type RedisConfig struct {
	Enabled bool
}

// Load reads the configuration from the environment. Defaults match the CLI's,
// so an absent value behaves as it did in-tree rather than as an empty string.
func Load(string) (*Config, error) {
	return &Config{
		ProjectName: envOr("PROJECT_NAME", "myproject"),
		BaseDomain:  envOr("BASE_DOMAIN", "local.nself.org"),
		Backup:      BackupConfig{Dir: envOr("BACKUP_DIR", "./backups")},
		DR:          DRConfig{StandbyHost: os.Getenv("DR_STANDBY_HOST")},
		Redis:       RedisConfig{Enabled: envBool("REDIS_ENABLED", false)},
	}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
