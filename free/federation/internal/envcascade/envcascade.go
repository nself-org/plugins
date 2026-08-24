// Package envcascade reimplements the narrow slice of the CLI's
// internal/config package that the federation plugin needs: resolving three
// env vars (NSELF_FEDERATION, NSELF_PLUGIN_DIR, HASURA_PORT) through the
// same .env cascade core uses, with the same defaults.
//
// Purpose: the plugin is its own Go module and cannot import
// github.com/nself-org/cli/internal/config — that package is ~7,200 lines
// spanning dozens of config sections, entirely out of proportion to the
// three scalar values federation.go actually reads off the resulting
// *config.Config (FederationEnabled, PluginSystem.Dir, Hasura.Port). Rather
// than duplicate config.Load() wholesale (dishonest — it would silently
// drift from the real thing) or guess at env resolution (unsafe — it would
// not honor the documented cascade), this package copies the two genuinely
// small, reusable pieces byte-for-byte: the CLI-R18 cascade file order
// (internal/config/cascade.go's EnvCascadeOrder) and the three helper
// functions used to read a value once the cascade is applied
// (internal/config/helpers.go's getEnvBool/getEnvInt/normalizeEnv). The
// actual file parsing and precedence application reuses
// github.com/joho/godotenv — the same third-party library core depends on,
// not a reimplementation of its own — so cascade semantics cannot drift
// even though the struct-population code around it is not copied.
//
// Inputs: a project directory (found by internal/projectroot.FindNSelfRoot).
//
// Outputs: FederationEnabled, PluginDir, HasuraPort — mirroring
// config.Config.FederationEnabled, config.Config.PluginSystem.Dir, and
// config.Config.Hasura.Port respectively, including their exact defaults
// (false, "~/.nself/plugins" unexpanded — matching core's own behavior of
// not expanding the tilde before use, and 8080).
//
// Constraints: read-only — never writes to the cascade files. Applies the
// same one-process-wide side effect core's godotenv.Overload has (mutates
// os.Environ for the loaded keys), which is safe here because the plugin
// process is short-lived and single-purpose.
package envcascade

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// legacyEnvOrderVar mirrors config.LegacyEnvOrderVar.
const legacyEnvOrderVar = "NSELF_LEGACY_ENV_ORDER"

// Values holds the three federation-relevant config fields, named to match
// their config.Config counterparts.
type Values struct {
	FederationEnabled bool
	PluginDir         string
	HasuraPort        int
}

// Load resolves the .env cascade under projectDir and returns the three
// values federation.go needs, with the same defaults core applies.
func Load(projectDir string) (Values, error) {
	env := getEnvOr("ENV", "dev")
	env = normalizeEnv(env)

	legacy := getEnvBool(legacyEnvOrderVar, false)
	names := envCascadeOrder(env, legacy)

	for _, n := range names {
		f := filepath.Join(projectDir, n)
		if _, err := os.Stat(f); err != nil {
			continue // optional layer, skip if absent
		}
		if err := godotenv.Overload(f); err != nil {
			return Values{}, err
		}
	}

	v := Values{
		FederationEnabled: getEnvBool("NSELF_FEDERATION", false),
		PluginDir:         getEnvOr("NSELF_PLUGIN_DIR", "~/.nself/plugins"),
		HasuraPort:        getEnvInt("HASURA_PORT", 8080),
	}
	return v, nil
}

// envCascadeOrder is a byte-for-byte copy of the canonical branch of
// config.EnvCascadeOrder (internal/config/cascade.go, CLI-R18 GATE B). The
// legacy branch is included too so a project still running with
// NSELF_LEGACY_ENV_ORDER=1 resolves identically whether the command is
// `nself federation` in-core or the extracted plugin.
func envCascadeOrder(envName string, legacy bool) []string {
	if legacy {
		order := []string{".env.dev"}
		switch envName {
		case "staging":
			order = append(order, ".env.staging")
		case "prod":
			order = append(order, ".env.prod")
		}
		return append(order, ".env.secrets", ".env.local", ".env", ".env.ai")
	}

	order := []string{".env"}
	switch envName {
	case "dev":
		order = append(order, ".env.dev")
	case "staging":
		order = append(order, ".env.staging")
	case "prod":
		order = append(order, ".env.prod")
	}
	return append(order, ".env.secrets", ".env.local")
}

// getEnvOr, getEnvInt, getEnvBool, and normalizeEnv are byte-for-byte copies
// of the unexported helpers in internal/config/helpers.go.

func getEnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	s := os.Getenv(key)
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func getEnvBool(key string, fallback bool) bool {
	s := strings.ToLower(os.Getenv(key))
	if s == "" {
		return fallback
	}
	return s == "true" || s == "1" || s == "yes" || s == "on" || s == "enabled"
}

func normalizeEnv(env string) string {
	switch strings.ToLower(env) {
	case "development", "develop", "devel":
		return "dev"
	case "production":
		return "prod"
	case "stage":
		return "staging"
	default:
		return strings.ToLower(env)
	}
}
