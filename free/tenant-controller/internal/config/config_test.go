package config

import (
	"strings"
	"testing"
)

func setRequired(t *testing.T) {
	t.Helper()
	for k, v := range map[string]string{
		"NSELF_CONTROLLER_ADMIN_TOKEN":       "tok",
		"DATABASE_URL":                       "postgres://x",
		"HASURA_GRAPHQL_ADMIN_SECRET":        "s",
		"HASURA_GRAPHQL_ENDPOINT":            "http://h",
		"NSELF_CONTROLLER_BASE_DOMAIN":       "example.test",
		"NSELF_CONTROLLER_ENABLED":           "true",
		"NSELF_FLAG_MULTI_TENANT_CONTROLLER": "",
	} {
		t.Setenv(k, v)
	}
}

func TestDisabledNeedsNoRequiredVars(t *testing.T) {
	for _, k := range []string{"NSELF_CONTROLLER_ADMIN_TOKEN", "DATABASE_URL", "HASURA_GRAPHQL_ADMIN_SECRET",
		"HASURA_GRAPHQL_ENDPOINT", "NSELF_CONTROLLER_BASE_DOMAIN", "NSELF_FLAG_MULTI_TENANT_CONTROLLER"} {
		t.Setenv(k, "")
	}
	t.Setenv("NSELF_CONTROLLER_ENABLED", "false")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("a disabled controller must load without its required vars: %v", err)
	}
	if cfg.Enabled {
		t.Fatal("Enabled must be false")
	}
}

func TestEnabledStillRequiresVars(t *testing.T) {
	setRequired(t)
	t.Setenv("NSELF_CONTROLLER_ADMIN_TOKEN", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "NSELF_CONTROLLER_ADMIN_TOKEN") {
		t.Fatalf("want missing-var error, got %v", err)
	}
}

func TestNginxReloadMode(t *testing.T) {
	setRequired(t)
	t.Setenv("NGINX_RELOAD_MODE", "")
	cfg, err := Load()
	if err != nil || cfg.NginxReloadMode != "exec" {
		t.Fatalf("default mode: %v %q", err, cfg.NginxReloadMode)
	}
	t.Setenv("NGINX_RELOAD_MODE", "watch")
	if cfg, err = Load(); err != nil || cfg.NginxReloadMode != "watch" {
		t.Fatalf("watch mode: %v %+v", err, cfg)
	}
	t.Setenv("NGINX_RELOAD_MODE", "docker")
	if _, err = Load(); err == nil {
		t.Fatal("an unknown reload mode must be a config error, not a silent default")
	}
}
