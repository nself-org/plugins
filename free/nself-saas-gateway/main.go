// nself-saas-gateway — the unified ɳSentry SaaS API façade (api.sentry.nself.org).
//
// Purpose: one public /v1 surface (the contract in cli/internal/sentryapi)
//
//	over the per-plugin internal APIs. The gateway authenticates tenants via
//	the shared saas layer (Bearer nsk_ API keys / nself-auth JWT), maps the
//	contract onto loopback plugin calls, and owns tenant lifecycle
//	(signup, billing tier updates, status-page registry).
//
// Routes: /health · /v1/signup · /v1/login · /v1/session · /v1/me · /v1/overview
//
//	· /v1/monitors[...] · /v1/incidents[...] · /v1/status-pages · /v1/api-keys[...]
//	· /v1/status/public/{slug} (PUBLIC — the one unauthenticated read)
//	· /v1/billing · /v1/alerts/channels[...] · /v1/ci/events[...] (CI-failure
//	  ingest, the SaaS equivalent of the self-hosted GitHub-Actions bridge)
//	· /internal/billing/tenant-tier
//	(/internal/* is loopback-only — nginx never routes it externally).
//
// Constraints: THIN by design — quota + tenant scoping for proxied resources
//
//	is enforced inside each plugin; the gateway enforces only what it owns
//	(status-page count quota, signup, tier upserts).
//
// Port: 3848 (F10-PORT-REGISTRY: nself-sentry-api, reserved 2026-07-01).
// SPORT: REGISTRY-SERVICES nself-saas-gateway.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/lib/pq"

	"github.com/nself-org/plugins-pro/paid/shared/email"
	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// gateway bundles the dependencies handlers need.
type gateway struct {
	cfg config
	db  *sql.DB
	// mail sends transactional email (invites, verify, password reset) via
	// the shared email pkg. nil = SMTP unconfigured; senders degrade
	// gracefully (flows still work, "sent" flags report false).
	mail email.Sender
}

// stripSpoofableHeaders removes inbound headers that internal hops trust.
// The gateway is the PUBLIC edge: X-Hasura-Tenant-Id is an internal-only
// header (gateway→plugin, injected in proxy.go from the VERIFIED credential).
// A client-supplied value must never survive past this point — before this
// strip existed, any caller could read any tenant's data by sending the
// header with a victim UUID (P0 tenant-spoof).
func stripSpoofableHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del("X-Hasura-Tenant-Id")
		next.ServeHTTP(w, r)
	})
}

// router assembles the chi router for the gateway.
func (g *gateway) router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(stripSpoofableHeaders) // SECURITY: must run before any auth/tenant logic
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", g.handleHealth)

	// Public auth surface — no tenant auth; per-IP rate-limited here AND at
	// the fronting nginx. The gateway is the SaaS auth authority (W3):
	// password signup/login mint HS256 session JWTs (SAAS_JWT_HS256_SECRET).
	signupLimiter := newRateLimiter(10, time.Minute)
	loginLimiter := newRateLimiter(15, time.Minute)
	r.Post("/v1/signup", rateLimited(signupLimiter, g.requireDB(g.handleSignup)))
	r.Post("/v1/login", rateLimited(loginLimiter, g.requireDB(g.handleLogin)))
	r.Get("/v1/session", g.handleSession)

	// Token-by-email account flows — public (no session exists yet), per-IP
	// rate-limited, anti-enumeration (handlers_authflows.go).
	flowLimiter := newRateLimiter(10, time.Minute)
	verifyLimiter := newRateLimiter(30, time.Minute)
	r.Post("/v1/verify-email", rateLimited(verifyLimiter, g.requireDB(g.handleVerifyEmail)))
	r.Post("/v1/verify-email/resend", rateLimited(flowLimiter, g.requireDB(g.handleResendVerifyEmail)))
	r.Post("/v1/password/forgot", rateLimited(flowLimiter, g.requireDB(g.handlePasswordForgot)))
	r.Post("/v1/password/reset", rateLimited(verifyLimiter, g.requireDB(g.handlePasswordReset)))

	// Team-invite accept — public, token-gated (handlers_join.go). The GET
	// renders the SPA accept page; the POST consumes the token.
	joinLimiter := newRateLimiter(20, time.Minute)
	r.Get("/v1/join/{token}", rateLimited(joinLimiter, g.requireDB(g.handleJoinInfo)))
	r.Post("/v1/join", rateLimited(joinLimiter, g.requireDB(g.handleJoinAccept)))

	// Public status pages — the ONE intentionally-public /v1 read. Slug →
	// registry tenant → public components only; fail-closed generic 404
	// (handlers_statuspublic.go). Per-IP rate-limited like the auth surface.
	publicStatusLimiter := newRateLimiter(120, time.Minute)
	r.Get("/v1/status/public/{slug}", rateLimited(publicStatusLimiter, g.requireDB(g.handlePublicStatus)))

	// Internal surface — shared-secret auth, never routed publicly (nginx
	// returns 404 for /internal/*; reach it on-box via 127.0.0.1:3848).
	r.Post("/internal/billing/tenant-tier", g.requireDB(g.handleTenantTier))
	r.Delete("/internal/tenants/{id}", g.requireDB(g.handleDeleteTenant))

	// Authenticated /v1 surface.
	// TrustHeader is HARD-false at the public edge: tenant identity comes
	// ONLY from a verified credential (nsk_ API key or HS256 session JWT).
	// TrustHeaderFromEnv() is for plugins behind this gateway on loopback —
	// never for the internet-facing surface. Belt-and-braces with the
	// stripSpoofableHeaders middleware above.
	saasMW := saas.Middleware(saas.Options{
		Cloud:       g.cfg.Cloud,
		DB:          g.db,
		JWTSecret:   g.cfg.JWTSecret, // SAAS_JWT_HS256_SECRET, falls back to legacy env chain
		TrustHeader: false,
	})
	r.Group(func(r chi.Router) {
		r.Use(saasMW)

		r.Get("/v1/me", g.requireDB(g.handleMe))
		r.Get("/v1/overview", g.handleOverview)
		r.Get("/v1/billing", g.requireDB(g.handleBilling))
		r.Get("/v1/usage", g.requireDB(g.handleUsage))

		// GDPR: the authenticated tenant's own data — no id to spoof.
		r.Get("/v1/account/export", g.requireDB(g.handleAccountExport))
		r.Delete("/v1/account", g.requireDB(g.handleAccountDelete))

		r.Route("/v1/api-keys", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListAPIKeys))
			r.Post("/", g.requireDB(g.handleCreateAPIKey))
			r.Delete("/{id}", g.requireDB(g.handleDeleteAPIKey))
		})

		r.Route("/v1/monitors", func(r chi.Router) {
			r.Get("/", g.handleListMonitors)
			r.Post("/", g.handleCreateMonitor)
			r.Patch("/{id}", g.handleUpdateMonitor)
			r.Delete("/{id}", g.handleDeleteMonitor)
			r.Post("/{id}/pause", g.handlePauseResumeMonitor("pause"))
			r.Post("/{id}/resume", g.handlePauseResumeMonitor("resume"))
		})

		r.Route("/v1/incidents", func(r chi.Router) {
			r.Get("/", g.handleListIncidents)
			r.Post("/", g.handleCreateIncident)
			r.Get("/{id}", g.handleGetIncident)
			r.Patch("/{id}", g.handleUpdateIncident)
			r.Post("/{id}/ack", g.handleIncidentAction("acknowledge"))
			r.Post("/{id}/resolve", g.handleIncidentAction("resolve"))
		})

		r.Route("/v1/errors", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListErrors))
			r.Get("/{id}", g.requireDB(g.handleGetError))
			r.Post("/{id}/resolve", g.requireDB(g.handleResolveError))
		})

		r.Get("/v1/rum", g.requireDB(g.handleRUMSummary))

		r.Route("/v1/cron", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListCron))
			r.Post("/{id}/heartbeat", g.requireDB(g.handleCronHeartbeat))
		})

		r.Route("/v1/ci/events", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListCIEvents))
			r.Post("/", g.requireDB(g.handleIngestCIEvent))
		})

		r.Route("/v1/status-pages", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListStatusPages))
			r.Post("/", g.requireDB(g.handleCreateStatusPage))
			r.Patch("/{id}", g.requireDB(g.handleUpdateStatusPage))
			r.Delete("/{id}", g.requireDB(g.handleDeleteStatusPage))
			r.Get("/{id}/components", g.requireDB(g.handleListComponents))
			r.Post("/{id}/components", g.requireDB(g.handleCreateComponent))
			r.Delete("/{id}/components/{component_id}", g.requireDB(g.handleDeleteComponent))
		})

		r.Route("/v1/team", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListTeam))
			r.Post("/", g.requireDB(g.handleInviteTeamMember))
			r.Delete("/{id}", g.requireDB(g.handleRemoveTeamMember))
		})

		r.Route("/v1/subscribers", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListSubscribers))
			r.Post("/", g.requireDB(g.handleCreateSubscriber))
			r.Delete("/{id}", g.requireDB(g.handleDeleteSubscriber))
		})

		r.Route("/v1/alerts/channels", func(r chi.Router) {
			r.Get("/", g.requireDB(g.handleListAlertChannels))
			r.Post("/", g.requireDB(g.handleCreateAlertChannel))
			r.Delete("/{id}", g.requireDB(g.handleDeleteAlertChannel))
			r.Post("/{id}/test", g.requireDB(g.handleTestAlertChannel))
		})
	})

	return r
}

// handleHealth — GET /health (no auth).
func (g *gateway) handleHealth(w http.ResponseWriter, r *http.Request) {
	dbState := "not_configured"
	if g.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := g.db.PingContext(ctx); err != nil {
			dbState = "error"
		} else {
			dbState = "ok"
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"service": "nself-saas-gateway",
		"db":      dbState,
	})
}

func main() {
	cfg := loadConfig()

	var db *sql.DB
	if cfg.DatabaseURL != "" {
		var err error
		db, err = sql.Open("postgres", cfg.DatabaseURL)
		if err != nil {
			log.Fatalf("saas-gateway: open database: %v", err)
		}
		db.SetMaxOpenConns(10)
		db.SetConnMaxLifetime(30 * time.Minute)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := saas.EnsureSchema(ctx, db); err != nil {
			cancel()
			log.Fatalf("saas-gateway: ensure saas schema: %v", err)
		}
		if err := ensureGatewaySchema(ctx, db); err != nil {
			cancel()
			log.Fatalf("saas-gateway: ensure gateway schema: %v", err)
		}
		cancel()
	} else if cfg.Cloud {
		// Cloud mode without a DB cannot authenticate API keys — refuse to
		// start rather than serve a fail-open surface.
		log.Fatal("saas-gateway: DATABASE_URL is required in cloud mode")
	}

	g := &gateway{cfg: cfg, db: db}

	// Transactional email (shared pkg; ELASTIC_EMAIL_*/SAAS_SMTP_* env).
	if emailCfg := email.ConfigFromEnv(); emailCfg.Enabled() {
		sender, err := email.NewSMTPSender(emailCfg)
		if err != nil {
			log.Printf("saas-gateway: email sender disabled: %v", err)
		} else {
			g.mail = sender
		}
	} else {
		log.Print("saas-gateway: email sender disabled (SMTP env not configured)")
	}

	// Down-detector + SSL-expiry sweeps (cloud boxes with a DB only): probe
	// results → incidents + channel alerts. Stopped via detectorCancel on
	// shutdown.
	detectorCtx, detectorCancel := context.WithCancel(context.Background())
	defer detectorCancel()
	if db != nil && cfg.Cloud {
		go g.runDetector(detectorCtx)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%s", cfg.Port),
		Handler:           g.router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}

	go func() {
		log.Printf("nself-saas-gateway listening on :%s (cloud=%v)", cfg.Port, cfg.Cloud)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("saas-gateway: serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("saas-gateway: shutdown: %v", err)
	}
}
