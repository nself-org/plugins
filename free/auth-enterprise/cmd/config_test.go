package main

import "testing"

func TestConfiguredEnv(t *testing.T) {
	for _, names := range [][2]string{
		{"AUTH_ENTERPRISE_SSO_SP_ENTITY_ID", "SP_ENTITY_ID"},
		{"AUTH_ENTERPRISE_SSO_ACS_URL", "SAML_ACS_URL"},
		{"AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL", "OIDC_CALLBACK_URL"},
		{"AUTH_ENTERPRISE_TOTP_ISSUER", "NSELF_ISSUER"},
	} {
		t.Run(names[0], func(t *testing.T) {
			t.Setenv(names[0], "")
			t.Setenv(names[1], "")
			if got := configuredEnv(names[0], names[1], "default"); got != "default" {
				t.Fatalf("unset: got %q", got)
			}
			t.Setenv(names[1], "legacy")
			if got := configuredEnv(names[0], names[1], "default"); got != "legacy" {
				t.Fatalf("legacy: got %q", got)
			}
			t.Setenv(names[0], "documented")
			if got := configuredEnv(names[0], names[1], "default"); got != "documented" {
				t.Fatalf("manifest name must take precedence: got %q", got)
			}
		})
	}
}
