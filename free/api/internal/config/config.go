// Package config reads the project settings this plugin needs.
//
// Purpose: the api command came out of the nSelf CLI, where it read a
// 6,500-line Config struct. It needs one value: where plugins are installed.
//
// Inputs: NSELF_PLUGIN_DIR, which the CLI resolves from the project's .env
// cascade and passes to this process because the plugin's manifest declares it.
//
// Outputs: a Config with the field name the moved code used.
//
// Constraints: this deliberately does NOT read .env files itself. The cascade
// order is defined once, in the CLI (CLI-R18).
package config

import "os"

// Config is the subset of project configuration this command reads.
type Config struct {
	PluginSystem PluginSystemConfig
}

// PluginSystemConfig holds where plugins live.
type PluginSystemConfig struct {
	Dir string
}

// Load reads the configuration from the environment.
func Load(string) (*Config, error) {
	return &Config{PluginSystem: PluginSystemConfig{Dir: os.Getenv("NSELF_PLUGIN_DIR")}}, nil
}
