package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nself-org/nself-byok/kms"
)

// HealthHandler returns 200 OK with status information.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"plugin":  "byok",
		"version": "0.1.0",
	})
}

// ReadyHandler checks the DB connection before returning 200.
func ReadyHandler(db *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready", "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}

// ──────────────────────────────────────────────────────────────────────────
// License gate — all endpoints require Enterprise
// ──────────────────────────────────────────────────────────────────────────

// enterpriseGate returns a 403 if NSELF_BYOK is not "true".
// This middleware must wrap every route in the BYOK plugin.
func enterpriseGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !byokEnabled() {
			errJSON(w, http.StatusForbidden, "BYOK requires an Enterprise license. Set NSELF_BYOK=true after purchasing.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func byokEnabled() bool {
	return strings.ToLower(os.Getenv("NSELF_BYOK")) == "true"
}

// ──────────────────────────────────────────────────────────────────────────
// Router
// ──────────────────────────────────────────────────────────────────────────

// RegisterRoutes registers all BYOK HTTP routes on r.
func RegisterRoutes(r chi.Router, db *pgxpool.Pool) {
	h := &byokHandler{db: db}

	r.Get("/health", HealthHandler)
	r.Get("/ready", ReadyHandler(db))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(enterpriseGate)

		// KMS configuration
		r.Get("/encryption/kms", h.GetKMSConfig)
		r.Post("/encryption/kms", h.ConfigureKMS)
		r.Put("/encryption/kms", h.UpdateKMSConfig)
		r.Post("/encryption/kms/verify", h.VerifyKMS)

		// Key rotation
		r.Post("/encryption/rotate", h.TriggerRotation)
		r.Get("/encryption/rotate/{job_id}", h.RotationStatus)
		r.Get("/encryption/key-events", h.ListKeyEvents)
	})
}

// ──────────────────────────────────────────────────────────────────────────
// Handler struct
// ──────────────────────────────────────────────────────────────────────────

type byokHandler struct {
	db *pgxpool.Pool
}

// ──────────────────────────────────────────────────────────────────────────
// KMS configuration handlers
// ──────────────────────────────────────────────────────────────────────────

type kmsConfigPayload struct {
	Provider       string `json:"provider"`       // "aws" | "gcp" | "vault"
	KeyRef         string `json:"key_ref"`
	Region         string `json:"region,omitempty"`
	EndpointURL    string `json:"endpoint_url,omitempty"`
	CredentialsRef string `json:"credentials_ref,omitempty"`
}

type kmsConfigResponse struct {
	ID             string  `json:"id"`
	TenantID       string  `json:"tenant_id"`
	Provider       string  `json:"provider"`
	KeyRef         string  `json:"key_ref"`
	Region         *string `json:"region"`
	EndpointURL    *string `json:"endpoint_url"`
	CredentialsRef *string `json:"credentials_ref"`
	Enabled        bool    `json:"enabled"`
	CreatedAt      string  `json:"created_at"`
	LastVerifiedAt *string `json:"last_verified_at"`
}

// GetKMSConfig returns the KMS configuration for the requesting tenant.
func (h *byokHandler) GetKMSConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	row := h.db.QueryRow(r.Context(),
		`SELECT id, tenant_id, provider, key_ref, region, endpoint_url,
		        credentials_ref, enabled, created_at, last_verified_at
		   FROM np_kms_configs WHERE tenant_id = $1`, tenantID)

	cfg, err := scanKMSConfig(row)
	if err != nil {
		if err.Error() == "no rows in result set" {
			errJSON(w, http.StatusNotFound, "KMS not configured for this tenant")
			return
		}
		errJSON(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

// ConfigureKMS creates a new KMS configuration for the tenant.
func (h *byokHandler) ConfigureKMS(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	var p kmsConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := validateProvider(p.Provider); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if p.KeyRef == "" {
		errJSON(w, http.StatusBadRequest, "key_ref is required")
		return
	}

	var id uuid.UUID
	err = h.db.QueryRow(r.Context(),
		`INSERT INTO np_kms_configs
		   (tenant_id, provider, key_ref, region, endpoint_url, credentials_ref)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
		tenantID, p.Provider, p.KeyRef,
		nullableString(p.Region), nullableString(p.EndpointURL), nullableString(p.CredentialsRef),
	).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, fmt.Sprintf("insert kms config: %v", err))
		return
	}

	// Audit event.
	h.logKeyEvent(r.Context(), tenantID, uuid.Nil, "key_configured", p.KeyRef, map[string]any{
		"provider": p.Provider,
	})

	writeJSON(w, http.StatusCreated, map[string]string{"id": id.String(), "status": "configured"})
}

// UpdateKMSConfig updates an existing KMS configuration.
func (h *byokHandler) UpdateKMSConfig(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	var p kmsConfigPayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	_, err = h.db.Exec(r.Context(),
		`UPDATE np_kms_configs
		    SET provider        = COALESCE(NULLIF($1,''), provider),
		        key_ref         = COALESCE(NULLIF($2,''), key_ref),
		        region          = COALESCE(NULLIF($3,''), region),
		        endpoint_url    = COALESCE(NULLIF($4,''), endpoint_url),
		        credentials_ref = COALESCE(NULLIF($5,''), credentials_ref)
		  WHERE tenant_id = $6`,
		p.Provider, p.KeyRef, p.Region, p.EndpointURL, p.CredentialsRef, tenantID,
	)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "update kms config")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// VerifyKMS performs a wrap+unwrap round-trip against the tenant's KMS and
// updates last_verified_at on success.
func (h *byokHandler) VerifyKMS(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	// Load config.
	row := h.db.QueryRow(r.Context(),
		`SELECT provider, key_ref, region, endpoint_url FROM np_kms_configs WHERE tenant_id = $1`, tenantID)
	var provider, keyRef string
	var region, endpointURL *string
	if err := row.Scan(&provider, &keyRef, &region, &endpointURL); err != nil {
		errJSON(w, http.StatusNotFound, "KMS not configured")
		return
	}

	// Build provider and run verification.
	verifyErr := verifyProvider(r.Context(), provider, keyRef, region, endpointURL)
	eventType := "verify_ok"
	if verifyErr != nil {
		eventType = "verify_fail"
	}

	h.logKeyEvent(r.Context(), tenantID, uuid.Nil, eventType, keyRef, map[string]any{
		"provider": provider,
		"error":    errString(verifyErr),
	})

	if verifyErr != nil {
		errJSON(w, http.StatusBadGateway, fmt.Sprintf("KMS verify failed: %v", verifyErr))
		return
	}

	// Update last_verified_at.
	_, _ = h.db.Exec(r.Context(),
		`UPDATE np_kms_configs SET last_verified_at = NOW() WHERE tenant_id = $1`, tenantID)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "KMS wrap+unwrap round-trip succeeded"})
}

// ──────────────────────────────────────────────────────────────────────────
// Key rotation handlers
// ──────────────────────────────────────────────────────────────────────────

// TriggerRotation starts a key rotation job (simplified — real impl uses async worker).
func (h *byokHandler) TriggerRotation(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	row := h.db.QueryRow(r.Context(),
		`SELECT key_ref FROM np_kms_configs WHERE tenant_id = $1 AND enabled = TRUE`, tenantID)
	var keyRef string
	if err := row.Scan(&keyRef); err != nil {
		errJSON(w, http.StatusNotFound, "KMS not configured or disabled")
		return
	}

	jobID := uuid.New()
	h.logKeyEvent(r.Context(), tenantID, uuid.Nil, "rotation_started", keyRef, map[string]any{
		"job_id": jobID.String(),
	})

	writeJSON(w, http.StatusAccepted, map[string]string{
		"job_id": jobID.String(),
		"status": "started",
	})
}

// RotationStatus returns the status of a rotation job by ID.
func (h *byokHandler) RotationStatus(w http.ResponseWriter, r *http.Request) {
	jobIDStr := chi.URLParam(r, "job_id")
	if jobIDStr == "" {
		errJSON(w, http.StatusBadRequest, "job_id required")
		return
	}

	row := h.db.QueryRow(r.Context(),
		`SELECT event_type, details, occurred_at
		   FROM np_encryption_key_events
		  WHERE details->>'job_id' = $1
		  ORDER BY occurred_at DESC LIMIT 1`, jobIDStr)

	var eventType string
	var details map[string]any
	var occurredAt time.Time
	if err := row.Scan(&eventType, &details, &occurredAt); err != nil {
		errJSON(w, http.StatusNotFound, "rotation job not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"job_id":     jobIDStr,
		"status":     eventType,
		"details":    details,
		"updated_at": occurredAt.Format(time.RFC3339),
	})
}

// ListKeyEvents returns the key event audit trail for the tenant.
func (h *byokHandler) ListKeyEvents(w http.ResponseWriter, r *http.Request) {
	tenantID, err := tenantIDFromRequest(r)
	if err != nil {
		errJSON(w, http.StatusUnauthorized, "tenant_id required")
		return
	}

	rows, err := h.db.Query(r.Context(),
		`SELECT id, event_type, key_ref, triggered_by, occurred_at, details
		   FROM np_encryption_key_events
		  WHERE tenant_id = $1
		  ORDER BY occurred_at DESC LIMIT 100`, tenantID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "db error")
		return
	}
	defer rows.Close()

	type eventRow struct {
		ID          string         `json:"id"`
		EventType   string         `json:"event_type"`
		KeyRef      string         `json:"key_ref"`
		TriggeredBy *string        `json:"triggered_by"`
		OccurredAt  string         `json:"occurred_at"`
		Details     map[string]any `json:"details"`
	}
	var events []eventRow
	for rows.Next() {
		var e eventRow
		var triggeredBy *uuid.UUID
		var occurredAt time.Time
		var id uuid.UUID
		if err := rows.Scan(&id, &e.EventType, &e.KeyRef, &triggeredBy, &occurredAt, &e.Details); err != nil {
			continue
		}
		e.ID = id.String()
		e.OccurredAt = occurredAt.Format(time.RFC3339)
		if triggeredBy != nil {
			s := triggeredBy.String()
			e.TriggeredBy = &s
		}
		events = append(events, e)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events, "count": len(events)})
}

// ──────────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func tenantIDFromRequest(r *http.Request) (uuid.UUID, error) {
	val := r.Header.Get("X-Tenant-ID")
	if val == "" {
		val = r.URL.Query().Get("tenant_id")
	}
	if val == "" {
		return uuid.Nil, fmt.Errorf("missing tenant_id")
	}
	return uuid.Parse(val)
}

func validateProvider(p string) error {
	switch p {
	case kms.ProviderAWS, kms.ProviderGCP, kms.ProviderVault:
		return nil
	}
	return fmt.Errorf("unsupported provider %q (must be aws, gcp, or vault)", p)
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// verifyProvider constructs a KMS provider from config and calls Verify.
func verifyProvider(ctx context.Context, provider, keyRef string, region, endpointURL *string) error {
	switch provider {
	case kms.ProviderAWS:
		reg := ""
		if region != nil {
			reg = *region
		}
		p, err := kms.NewAWSKMSProvider(ctx, reg)
		if err != nil {
			return err
		}
		return p.Verify(ctx, keyRef)
	case kms.ProviderGCP:
		p, err := kms.NewGCPKMSProvider(ctx, nil)
		if err != nil {
			return err
		}
		return p.Verify(ctx, keyRef)
	case kms.ProviderVault:
		ep := ""
		if endpointURL != nil {
			ep = *endpointURL
		}
		token := os.Getenv("VAULT_TOKEN")
		p := kms.NewVaultKMSProvider(ep, token)
		return p.Verify(ctx, keyRef)
	}
	return fmt.Errorf("unknown provider: %s", provider)
}

func (h *byokHandler) logKeyEvent(ctx context.Context, tenantID, triggeredBy uuid.UUID, eventType, keyRef string, details map[string]any) {
	var trig *uuid.UUID
	if triggeredBy != uuid.Nil {
		t := triggeredBy
		trig = &t
	}
	_, _ = h.db.Exec(ctx,
		`INSERT INTO np_encryption_key_events
		   (tenant_id, event_type, key_ref, triggered_by, details)
		 VALUES ($1, $2, $3, $4, $5)`,
		tenantID, eventType, keyRef, trig, details,
	)
}

func scanKMSConfig(row pgx.Row) (*kmsConfigResponse, error) {
	var cfg kmsConfigResponse
	var id, tenantID uuid.UUID
	var createdAt time.Time
	var lastVerifiedAt *time.Time
	err := row.Scan(
		&id, &tenantID,
		&cfg.Provider, &cfg.KeyRef,
		&cfg.Region, &cfg.EndpointURL, &cfg.CredentialsRef,
		&cfg.Enabled, &createdAt, &lastVerifiedAt,
	)
	if err != nil {
		return nil, err
	}
	cfg.ID = id.String()
	cfg.TenantID = tenantID.String()
	cfg.CreatedAt = createdAt.Format(time.RFC3339)
	if lastVerifiedAt != nil {
		s := lastVerifiedAt.Format(time.RFC3339)
		cfg.LastVerifiedAt = &s
	}
	return &cfg, nil
}
