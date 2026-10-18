// Package config resolves the auth-enterprise environment variables.
//
// Purpose: one place that maps the names documented in plugin.json and the
// README (AUTH_ENTERPRISE_*) to the values the server uses.
// Inputs: a getenv function (os.Getenv in production).
// Outputs: Config with defaults applied.
// Constraints: the documented name wins; the older short names (SP_ENTITY_ID,
// SAML_ACS_URL, OIDC_CALLBACK_URL, NSELF_ISSUER) still work as a fallback so
// existing stacks keep running.
package config

// Config holds the SSO and TOTP settings read from the environment.
type Config struct {
	SPEntityID      string
	SAMLACSURL      string
	OIDCCallbackURL string
	TOTPIssuer      string
}

// first returns the value of the first non-empty variable, else def.
func first(get func(string) string, def string, names ...string) string {
	for _, n := range names {
		if v := get(n); v != "" {
			return v
		}
	}
	return def
}

// Load reads Config using get to look up variables.
func Load(get func(string) string) Config {
	return Config{
		SPEntityID: first(get, "https://nself.local/auth/enterprise",
			"AUTH_ENTERPRISE_SSO_SP_ENTITY_ID", "SP_ENTITY_ID"),
		SAMLACSURL: first(get, "https://nself.local/auth/sso/saml/callback",
			"AUTH_ENTERPRISE_SSO_ACS_URL", "SAML_ACS_URL"),
		OIDCCallbackURL: first(get, "https://nself.local/auth/sso/oidc/callback",
			"AUTH_ENTERPRISE_SSO_OIDC_CALLBACK_URL", "OIDC_CALLBACK_URL"),
		TOTPIssuer: first(get, "nSelf",
			"AUTH_ENTERPRISE_TOTP_ISSUER", "NSELF_ISSUER"),
	}
}
