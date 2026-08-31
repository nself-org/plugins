package main

// config.go — environment configuration for the ɳSentry SaaS gateway.
//
// Purpose: single place all env vars are read. The gateway is the public
//   /v1 façade at api.sentry.nself.org: it authenticates tenants (shared
//   saas layer) and maps the unified contract onto the per-plugin internal
//   APIs over loopback.
// Inputs:  environment variables (all optional except DATABASE_URL in
//   cloud mode).
// Outputs: config struct consumed by main.go + handlers.
// Constraints: defaults target a single-box deployment where the nSentry
//   plugins listen on their F10-registry loopback ports.
// SPORT: REGISTRY-SERVICES nself-saas-gateway (port 3848, F10 nself-sentry-api).

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// jwtSecret resolves the HS256 secret: SAAS_JWT_HS256_SECRET (canonical for
// the SaaS auth authority) first, then the shared saas env chain.
func jwtSecret() []byte {
	if v := strings.TrimSpace(os.Getenv("SAAS_JWT_HS256_SECRET")); v != "" {
		return []byte(v)
	}
	return saas.JWTSecretFromEnv()
}

// upstreamJWTSecret resolves the secret for gateway→plugin service JWTs:
// SAAS_GATEWAY_UPSTREAM_JWT_SECRET first, then the shared saas env chain
// (the plugins' own HASURA_GRAPHQL_JWT_SECRET on a single-box deployment).
func upstreamJWTSecret() []byte {
	if v := strings.TrimSpace(os.Getenv("SAAS_GATEWAY_UPSTREAM_JWT_SECRET")); v != "" {
		return []byte(v)
	}
	return saas.JWTSecretFromEnv()
}

// config holds the resolved gateway configuration.
type config struct {
	// Port the gateway listens on (F10-reserved 3848).
	Port string
	// DatabaseURL is the shared ops-Postgres holding np_saas_* tables.
	DatabaseURL string
	// Cloud enables tenant auth (fail-closed). Set SAAS_GATEWAY_MODE=cloud.
	// Default TRUE: the gateway only exists as a SaaS front door; local-first
	// dev uses the seeded dev tenant key (cmd/seed) rather than no-auth.
	Cloud bool

	// Upstream plugin base URLs (loopback in the single-box topology).
	UptimeURL   string // nself-uptime-monitor  (F10: 3831)
	IncidentURL string // nself-incident-mgmt   (F10: 3833)
	AlertURL    string // nself-alert-router    (F10: 3834)

	// InternalAPIKey guards POST /internal/billing/tenant-tier (W4 webhook
	// → tier updates). Empty = endpoint disabled (503, fail-closed).
	InternalAPIKey string

	// InternalAllowIPs is the source-IP allowlist for the externally-reachable
	// billing tenant-tier hook (the prod ping_api box). Set via
	// SAAS_GATEWAY_INTERNAL_ALLOW_IPS (comma-separated). Empty = no IP gate
	// (loopback-only endpoints depend on nginx returning 404 externally).
	// Loopback (127.0.0.1, ::1) is always permitted.
	InternalAllowIPs map[string]struct{}

	// JWTSecret signs /v1/login session tokens and verifies bearer JWTs on
	// the /v1 surface. Resolution: SAAS_JWT_HS256_SECRET first (the SaaS
	// auth-authority secret, shared with the SPA's AUTH_HS256_KEY), then the
	// legacy saas.JWTSecretFromEnv chain (NSELF_JWT_SECRET /
	// HASURA_GRAPHQL_JWT_SECRET). Empty = password login disabled (503).
	JWTSecret []byte

	// UpstreamJWTSecret signs the short-lived SERVICE JWTs the gateway mints
	// for the gateway→plugin hop (proxy.go). The plugins verify with their
	// HASURA_GRAPHQL_JWT_SECRET, which differs from the SPA session secret —
	// the gateway re-mints rather than forwarding the dashboard JWT.
	// Resolution: SAAS_GATEWAY_UPSTREAM_JWT_SECRET, then the shared saas env
	// chain (NSELF_JWT_SECRET / HASURA_GRAPHQL_JWT_SECRET). Empty = no
	// service JWT (plugins resolve via the injected tenant header only).
	UpstreamJWTSecret []byte

	// AlertIngestHMACSecret signs test alerts sent to alert-router's
	// /api/alerts when ALERT_ROUTER_INBOUND_HMAC_SECRET is set on the box.
	AlertIngestHMACSecret string

	// StatusPageBaseURL is the public base for tenant status pages
	// (PRD §4: sentry.nself.org/s/<slug>).
	StatusPageBaseURL string

	// DashboardBaseURL is the SPA base used for deep links in alert emails.
	DashboardBaseURL string

	// DownThreshold is how many CONSECUTIVE failed checks flip a monitor to
	// down (opens an incident + fires monitor.down). SAAS_DOWN_THRESHOLD.
	DownThreshold int
	// DetectorInterval is the down-detector sweep cadence.
	// SAAS_DOWN_DETECTOR_INTERVAL (Go duration).
	DetectorInterval time.Duration
	// SSLExpiryDays is the TLS warning window: certs with fewer remaining
	// days fire ssl.expiring. SAAS_SSL_EXPIRY_ALERT_DAYS.
	SSLExpiryDays int
	// SSLCheckInterval is the SSL-expiry sweep cadence.
	// SAAS_SSL_CHECK_INTERVAL (Go duration).
	SSLCheckInterval time.Duration
}

// envInt reads a positive integer env var with a default.
func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}

// envDuration reads a positive Go-duration env var with a default.
func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// parseAllowIPs builds the internal source-IP allowlist. Loopback is always
// included so on-box callers keep working. An empty/blank env var yields an
// empty set (the IP gate is then skipped by internalIPAllowed).
func parseAllowIPs(raw string) map[string]struct{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	set := map[string]struct{}{"127.0.0.1": {}, "::1": {}}
	for _, part := range strings.Split(raw, ",") {
		if ip := strings.TrimSpace(part); ip != "" {
			set[ip] = struct{}{}
		}
	}
	return set
}

func envDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// loadConfig reads gateway configuration from the environment.
func loadConfig() config {
	return config{
		Port:        envDefault("PORT", "3848"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Cloud:       !strings.EqualFold(envDefault("SAAS_GATEWAY_MODE", "cloud"), "selfhost"),

		UptimeURL:   envDefault("SAAS_GATEWAY_UPTIME_URL", "http://127.0.0.1:3831"),
		IncidentURL: envDefault("SAAS_GATEWAY_INCIDENT_URL", "http://127.0.0.1:3833"),
		AlertURL:    envDefault("SAAS_GATEWAY_ALERT_URL", "http://127.0.0.1:3834"),

		InternalAPIKey:        strings.TrimSpace(os.Getenv("NSENTRY_INTERNAL_API_KEY")),
		InternalAllowIPs:      parseAllowIPs(os.Getenv("SAAS_GATEWAY_INTERNAL_ALLOW_IPS")),
		JWTSecret:             jwtSecret(),
		UpstreamJWTSecret:     upstreamJWTSecret(),
		AlertIngestHMACSecret: strings.TrimSpace(os.Getenv("ALERT_ROUTER_INBOUND_HMAC_SECRET")),

		StatusPageBaseURL: envDefault("SAAS_GATEWAY_STATUS_PAGE_BASE", "https://sentry.nself.org/s"),
		DashboardBaseURL:  strings.TrimRight(envDefault("SAAS_GATEWAY_DASHBOARD_BASE", "https://sentry.nself.org"), "/"),

		DownThreshold:    envInt("SAAS_DOWN_THRESHOLD", 2),
		DetectorInterval: envDuration("SAAS_DOWN_DETECTOR_INTERVAL", time.Minute),
		SSLExpiryDays:    envInt("SAAS_SSL_EXPIRY_ALERT_DAYS", 14),
		SSLCheckInterval: envDuration("SAAS_SSL_CHECK_INTERVAL", 12*time.Hour),
	}
}
