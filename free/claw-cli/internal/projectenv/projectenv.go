// Package projectenv reads the handful of nSelf project .env values this
// plugin needs, without depending on cli/internal/config.
//
// Purpose: cli/internal/claw.Migrate and cli/cmd/commands/claw_pair.go's
// getServerURL() both took a *config.Config, but only ever read five fields
// off it (ProjectName, BaseDomain, Env, Postgres.User, Postgres.DB) plus one
// more from a sibling struct (PluginSystem.Dir). cli/internal/config is
// 7239 lines — the full .env cascade loader, schema validation, unknown-var
// warnings, and defaulting for 100+ fields — and is unreachable from this
// plugin module (a separate Go module) besides. Duplicating it whole would
// be a far worse outcome than this: a narrow, direct re-read of six
// well-known, publicly-documented env var names, using the same file
// cascade order core's internal/config/loader.go documents (.env then
// .env.{ENV} then .env.secrets then .env.local, later overrides earlier).
//
// Inputs: a project directory (normally the current working directory).
//
// Outputs: a Config with the six fields, each falling back to the same
// default core's internal/config/defaults*.go apply when the var is unset.
//
// Constraints: this intentionally does NOT replicate core's full schema
// validation, unknown-var warnings, or NSELF_LEGACY_ENV_ORDER escape hatch —
// none of that affects the six values this plugin reads. If core's cascade
// order or defaults for these six vars ever change, this file needs a
// matching update; there is no compile-time link forcing that, which is the
// accepted cost of this package boundary.
package projectenv

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Config holds the project settings this plugin's claw commands need.
type Config struct {
	// ProjectName names the postgres container: <ProjectName>_postgres.
	ProjectName string
	// BaseDomain and Env build the public server URL for pairing.
	BaseDomain string
	Env        string
	// PostgresUser and PostgresDB target the claw schema migrations.
	PostgresUser string
	PostgresDB   string
	// PluginDir is where installed CLI-type plugins publish their binaries
	// and extracted assets (e.g. this plugin's own migrations/ directory
	// once installed, at <PluginDir>/claw/migrations — see internal/claw).
	PluginDir string
}

// defaults mirror cli/internal/config/defaults.go and
// defaults_postgres_hasura.go / defaults_plugins_backup.go for these exact
// six keys.
const (
	defaultProjectName = "myproject"
	defaultBaseDomain  = "local.nself.org"
	defaultEnv         = "dev"
	defaultPostgresDB  = "nself"
	defaultPostgresUsr = "postgres"
)

// Load reads the .env cascade from dir and returns the six values this
// plugin needs, applying the same defaults core applies when a var is unset
// anywhere in the cascade (including the real process environment).
func Load(dir string) (*Config, error) {
	env := lookup("ENV", dir, defaultEnv)

	// Cascade order matches internal/config/cascade.go's canonical
	// (non-legacy) order: .env → .env.{ENV} → .env.secrets → .env.local.
	files := []string{
		".env",
		".env." + env,
		".env.secrets",
		".env.local",
	}

	merged := map[string]string{}
	for _, f := range files {
		vars, err := parseEnvFile(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		for k, v := range vars {
			merged[k] = v
		}
	}

	get := func(key, def string) string {
		// Process env wins over the file cascade, matching godotenv.Overload
		// semantics core's loader uses (each file is Setenv'd in order, so a
		// var already exported in the real environment is what a plain
		// os.Getenv would see first regardless of file content).
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v, ok := merged[key]; ok && v != "" {
			return v
		}
		return def
	}

	pluginDir := get("NSELF_PLUGIN_DIR", "")
	if pluginDir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			pluginDir = filepath.Join(home, ".nself", "plugins")
		}
	}

	return &Config{
		ProjectName:  get("PROJECT_NAME", defaultProjectName),
		BaseDomain:   get("BASE_DOMAIN", defaultBaseDomain),
		Env:          env,
		PostgresUser: get("POSTGRES_USER", defaultPostgresUsr),
		PostgresDB:   get("POSTGRES_DB", defaultPostgresDB),
		PluginDir:    pluginDir,
	}, nil
}

// lookup reads one var from the process environment, falling back to a
// single-file peek at dir/.env (ENV itself is read before the cascade order
// can be determined, so only the base file is consulted), then to def.
func lookup(key, dir, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	vars, err := parseEnvFile(filepath.Join(dir, ".env"))
	if err == nil {
		if v, ok := vars[key]; ok && v != "" {
			return v
		}
	}
	return def
}

// parseEnvFile reads simple KEY=VALUE lines from path. Missing files return
// an empty map, not an error — core's cascade treats every file as optional.
// Lines starting with # are comments; surrounding single or double quotes on
// the value are stripped, matching the common .env convention.
func parseEnvFile(path string) (map[string]string, error) {
	out := map[string]string{}

	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.IndexByte(line, '=')
		if idx < 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		val = strings.Trim(val, `"'`)
		if key != "" {
			out[key] = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
