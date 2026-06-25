package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	sdk "github.com/nself-org/plugin-sdk"
)

// Size-cap exception: single-responsibility HTTP route handler — 97L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func handleIngest(pool *pgxpool.Pool, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Always authenticate internal callers.
		provided := r.Header.Get("X-Plugin-Secret")
		if provided == "" || provided != secret {
			sdk.Respond(w, http.StatusUnauthorized, map[string]string{"error": "missing or invalid X-Plugin-Secret"})
			return
		}

		var req IngestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}

		// Validate required fields.
		if req.EventType == "" {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{"error": "event_type is required"})
			return
		}
		if !validEventTypes[req.EventType] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid event_type; accepted values: auth.login, auth.logout, auth.login_failed, auth.mfa_enabled, privilege.change, secret.accessed, plugin.installed, plugin.uninstalled",
			})
			return
		}
		if req.ActorType != "" && !validActorTypes[req.ActorType] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid actor_type; accepted values: user, system, plugin",
			})
			return
		}
		if req.Severity != "" && !validSeverities[req.Severity] {
			sdk.Respond(w, http.StatusBadRequest, map[string]string{
				"error": "invalid severity; accepted values: info, warning, critical",
			})
			return
		}

		// Apply defaults.
		if req.ActorType == "" {
			req.ActorType = "system"
		}
		if req.Severity == "" {
			req.Severity = "info"
		}
		if req.SourceAccountID == "" {
			req.SourceAccountID = "primary"
		}
		if req.Metadata == nil {
			req.Metadata = map[string]any{}
		}

		// Auto-populate source_plugin from X-Source-Plugin header when the
		// ingest request doesn't set it explicitly. S43-T18.
		sourcePlugin := req.SourcePlugin
		if sourcePlugin == "" {
			sourcePlugin = strings.ToLower(strings.TrimSpace(r.Header.Get("X-Source-Plugin")))
		}

		event := &AuditEvent{
			ID:              uuid.New().String(),
			SourceAccountID: req.SourceAccountID,
			ActorUserID:     req.ActorUserID,
			ActorType:       req.ActorType,
			EventType:       req.EventType,
			ResourceType:    req.ResourceType,
			ResourceID:      req.ResourceID,
			IPAddress:       req.IPAddress,
			UserAgent:       req.UserAgent,
			Metadata:        req.Metadata,
			Severity:        req.Severity,
			SourcePlugin:    sourcePlugin,
			TargetPlugin:    req.TargetPlugin,
			CreatedAt:       time.Now().UTC(),
		}

		// Prefer real client IP when X-Plugin-Secret is set (trusted call from
		// within the stack) or fall back to the remote address.
		if event.IPAddress == "" {
			event.IPAddress = realIP(r)
		}
		if event.UserAgent == "" {
			event.UserAgent = r.Header.Get("User-Agent")
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		if err := InsertEvent(ctx, pool, event); err != nil {
			sdk.Respond(w, http.StatusInternalServerError, map[string]string{"error": "failed to store event: " + err.Error()})
			return
		}

		sdk.Respond(w, http.StatusCreated, event)
	}
}

// handleList handles GET /events.
// All requests must include a matching X-Plugin-Secret header.
//
// Supported query parameters:
//
//	event_type        — exact match
//	actor_user_id     — exact match
//	severity          — exact match (info | warning | critical)
//	source_account_id — exact match; omit to return all accounts
//	from              — RFC 3339 lower bound on created_at
//	to                — RFC 3339 upper bound on created_at
//	limit             — default 50, max 1000
//	offset            — default 0
