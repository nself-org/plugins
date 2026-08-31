// Package main — auth-enterprise plugin HTTP server.
//
// Purpose: Registers all MFA and SSO HTTP routes and starts the server.
// Inputs: ENV — PLUGIN_AUTH_ENTERPRISE_PORT (default 3826), DATABASE_URL,
//         NSELF_SSO (enables SSO endpoints), SP_ENTITY_ID, SAML_ACS_URL,
//         OIDC_CALLBACK_URL.
// Outputs: HTTP server on configured port exposing all MFA + SSO routes.
// Constraints: MFA always active (Security-Always-Free); SSO requires
//              NSELF_SSO=true (ɳSelf+ license gated in handler).
package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-auth-enterprise/go/handlers"
	"github.com/nself-org/nself-auth-enterprise/go/mfa"
)

func main() {
	port := os.Getenv("PLUGIN_AUTH_ENTERPRISE_PORT")
	if port == "" {
		port = "3826"
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "postgres://postgres:postgres@localhost:5432/nself"
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		log.Fatalf("auth-enterprise: db connect: %v", err)
	}
	defer pool.Close()

	spEntityID := os.Getenv("SP_ENTITY_ID")
	if spEntityID == "" {
		spEntityID = "https://nself.local/auth/enterprise"
	}
	samlACSURL := os.Getenv("SAML_ACS_URL")
	if samlACSURL == "" {
		samlACSURL = "https://nself.local/auth/sso/saml/callback"
	}
	oidcCallbackURL := os.Getenv("OIDC_CALLBACK_URL")
	if oidcCallbackURL == "" {
		oidcCallbackURL = "https://nself.local/auth/sso/oidc/callback"
	}

	// Service construction.
	issuer := os.Getenv("NSELF_ISSUER")
	if issuer == "" {
		issuer = "nSelf"
	}
	totpSvc := mfa.NewTOTPService(pool, issuer)
	policySvc := mfa.NewPolicyService(pool)
	recoverySvc := mfa.NewRecoveryService(pool)

	mfaH := handlers.NewMFAHandlers(totpSvc, policySvc, recoverySvc)
	ssoH := handlers.NewSSOHandlers(pool, spEntityID, samlACSURL, oidcCallbackURL)

	mux := http.NewServeMux()

	// ── Health + readiness ───────────────────────────────────────────────────
	mux.HandleFunc("/health", handlers.HandleHealth)
	mux.HandleFunc("/ready", handlers.HandleHealth)

	// ── MFA endpoints (always active — Security-Always-Free) ────────────────
	mux.HandleFunc("GET /auth/mfa/status", mfaH.Status)
	mux.HandleFunc("POST /auth/mfa/totp/setup", mfaH.TOTPSetup)
	mux.HandleFunc("POST /auth/mfa/totp/verify", mfaH.TOTPVerify)
	mux.HandleFunc("POST /auth/mfa/totp/challenge", mfaH.TOTPChallenge)
	mux.HandleFunc("POST /auth/mfa/recovery", mfaH.UseRecoveryCode)
	mux.HandleFunc("GET /auth/mfa/recovery/codes", mfaH.ListRecoveryCodes)
	mux.HandleFunc("POST /auth/mfa/recovery/regenerate", mfaH.RegenerateCodes)
	mux.HandleFunc("GET /auth/mfa/policy", mfaH.GetPolicy)
	mux.HandleFunc("PUT /auth/mfa/policy", mfaH.SetPolicy)

	// ── SSO endpoints (NSELF_SSO=true required — handler enforces gate) ─────
	mux.HandleFunc("GET /auth/sso/providers", ssoH.ListProviders)
	mux.HandleFunc("POST /auth/sso/providers", ssoH.CreateProvider)
	mux.HandleFunc("DELETE /auth/sso/providers/{id}", ssoH.DeleteProvider)
	mux.HandleFunc("GET /auth/sso/metadata", ssoH.SPMetadata)
	mux.HandleFunc("GET /auth/sso/{provider}/begin", ssoH.OIDCBegin)
	mux.HandleFunc("POST /auth/sso/saml/callback", ssoH.SAMLCallback)
	mux.HandleFunc("GET /auth/sso/oidc/callback", ssoH.OIDCCallback)

	addr := ":" + port
	log.Printf("nself-auth-enterprise listening on %s (SSO=%v)", addr, os.Getenv("NSELF_SSO") == "true")
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
