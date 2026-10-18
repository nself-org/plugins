package config

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDocumentedNames(t *testing.T) {
	c := Load(env(map[string]string{
		"AUTH_ENTERPRISE_SSO_SP_ENTITY_ID":      "https://sp.example/entity",
		"AUTH_ENTERPRISE_SSO_ACS_URL":           "https://sp.example/acs",
		"AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL": "https://sp.example/oidc",
		"AUTH_ENTERPRISE_TOTP_ISSUER":           "Acme",
	}))
	want := Config{
		SPEntityID:      "https://sp.example/entity",
		SAMLACSURL:      "https://sp.example/acs",
		OIDCCallbackURL: "https://sp.example/oidc",
		TOTPIssuer:      "Acme",
	}
	if c != want {
		t.Fatalf("got %+v, want %+v", c, want)
	}
}

func TestLoadLegacyNamesStillWork(t *testing.T) {
	c := Load(env(map[string]string{
		"SP_ENTITY_ID":      "legacy-entity",
		"SAML_ACS_URL":      "legacy-acs",
		"OIDC_CALLBACK_URL": "legacy-oidc",
		"NSELF_ISSUER":      "Legacy",
	}))
	if c.SPEntityID != "legacy-entity" || c.SAMLACSURL != "legacy-acs" ||
		c.OIDCCallbackURL != "legacy-oidc" || c.TOTPIssuer != "Legacy" {
		t.Fatalf("legacy names ignored: %+v", c)
	}
}

func TestLoadDocumentedWinsOverLegacy(t *testing.T) {
	c := Load(env(map[string]string{
		"AUTH_ENTERPRISE_SSO_SP_ENTITY_ID": "new",
		"SP_ENTITY_ID":                     "old",
	}))
	if c.SPEntityID != "new" {
		t.Fatalf("SPEntityID = %q, want new", c.SPEntityID)
	}
}

func TestLoadDefaults(t *testing.T) {
	c := Load(env(nil))
	want := Config{
		SPEntityID:      "https://nself.local/auth/enterprise",
		SAMLACSURL:      "https://nself.local/auth/sso/saml/callback",
		OIDCCallbackURL: "https://nself.local/auth/sso/oidc/callback",
		TOTPIssuer:      "nSelf",
	}
	if c != want {
		t.Fatalf("got %+v, want %+v", c, want)
	}
}
