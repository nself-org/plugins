package sdk

import (
	"os"
	"strconv"
	"strings"
)

// Config holds standard environment configuration for an nSelf plugin.
type Config struct {
	DatabaseURL string
	Port        int
	Secret      string // PLUGIN_INTERNAL_SECRET

	// AllowedCallers is the set of plugin names permitted to send requests to
	// this plugin via the X-Source-Plugin header. Populated from the CSV env
	// var PLUGIN_<NAME>_ALLOWED_CALLERS set by `nself build`. An empty set
	// means strict mode: no cross-plugin calls are permitted (unless
	// STRICT_PLUGIN_AUTH=false, which disables the check entirely for dev).
	AllowedCallers map[string]bool

	// StrictPluginAuth controls whether the X-Source-Plugin identity check is
	// enforced. Defaults to true (strict). Set STRICT_PLUGIN_AUTH=false in
	// dev environments to skip the check. Production stacks always leave this
	// at the default. S43-T02.
	StrictPluginAuth bool
}

// LoadConfig reads DATABASE_URL, PORT (default 3000), PLUGIN_INTERNAL_SECRET,
// PLUGIN_<NAME>_ALLOWED_CALLERS, and STRICT_PLUGIN_AUTH from the environment.
// name must be the plugin's own name (e.g. "ai", "claw") so the correct
// ALLOWED_CALLERS env var is resolved.
func LoadConfig() *Config {
	port := 3000
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			port = p
		}
	}

	// STRICT_PLUGIN_AUTH defaults true; explicitly set to "false" to disable.
	strict := true
	if strings.ToLower(os.Getenv("STRICT_PLUGIN_AUTH")) == "false" {
		strict = false
	}

	// Build the allowed-callers set from the generic env var.
	// Individual plugins may also call LoadConfigForPlugin("ai") etc. which
	// uses the per-plugin PLUGIN_AI_ALLOWED_CALLERS form.
	callers := parseAllowedCallers(os.Getenv("PLUGIN_ALLOWED_CALLERS"))

	return &Config{
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		Port:             port,
		Secret:           os.Getenv("PLUGIN_INTERNAL_SECRET"),
		AllowedCallers:   callers,
		StrictPluginAuth: strict,
	}
}

// LoadConfigForPlugin is like LoadConfig but also reads the per-plugin
// PLUGIN_<UPPER_NAME>_ALLOWED_CALLERS env var, merging it with any value
// already in PLUGIN_ALLOWED_CALLERS.
func LoadConfigForPlugin(name string) *Config {
	cfg := LoadConfig()

	envKey := "PLUGIN_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_ALLOWED_CALLERS"
	if v := os.Getenv(envKey); v != "" {
		for k := range parseAllowedCallers(v) {
			cfg.AllowedCallers[k] = true
		}
	}
	return cfg
}

// parseAllowedCallers splits a CSV string of plugin names into a lookup set.
// Empty strings and whitespace are ignored. Plugin names are lowercased.
func parseAllowedCallers(csv string) map[string]bool {
	set := make(map[string]bool)
	for _, s := range strings.Split(csv, ",") {
		s = strings.TrimSpace(strings.ToLower(s))
		if s != "" {
			set[s] = true
		}
	}
	return set
}
