package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/nself-byok/tenantdb"
)

// ──────────────────────────────────────────────────────────────────────────
// KMS configuration handlers
// ──────────────────────────────────────────────────────────────────────────

type kmsConfigPayload struct {
	Provider       string `json:"provider"` // "aws" | "gcp" | "vault"
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

	var cfg *kmsConfigResponse
	err = tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		var scanErr error
		cfg, scanErr = scanKMSConfig(tx.QueryRow(r.Context(),
			`SELECT id, tenant_id, provider, key_ref, region, endpoint_url,
		        credentials_ref, enabled, created_at, last_verified_at
		   FROM np_kms_configs WHERE tenant_id = $1`, tenantID))
		return scanErr
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	err = tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(),
			`INSERT INTO np_kms_configs
		   (tenant_id, provider, key_ref, region, endpoint_url, credentials_ref)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id`,
			tenantID, p.Provider, p.KeyRef,
			nullableString(p.Region), nullableString(p.EndpointURL), nullableString(p.CredentialsRef),
		).Scan(&id)
	})
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

	err = tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(r.Context(),
			`UPDATE np_kms_configs
		    SET provider        = COALESCE(NULLIF($1,''), provider),
		        key_ref         = COALESCE(NULLIF($2,''), key_ref),
		        region          = COALESCE(NULLIF($3,''), region),
		        endpoint_url    = COALESCE(NULLIF($4,''), endpoint_url),
		        credentials_ref = COALESCE(NULLIF($5,''), credentials_ref)
		  WHERE tenant_id = $6`,
			p.Provider, p.KeyRef, p.Region, p.EndpointURL, p.CredentialsRef, tenantID,
		)
		return execErr
	})
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
	var provider, keyRef string
	var region, endpointURL *string
	if err := tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		return tx.QueryRow(r.Context(),
			`SELECT provider, key_ref, region, endpoint_url FROM np_kms_configs WHERE tenant_id = $1`, tenantID,
		).Scan(&provider, &keyRef, &region, &endpointURL)
	}); err != nil {
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
	_ = tenantdb.With(r.Context(), h.db, tenantID, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(r.Context(),
			`UPDATE np_kms_configs SET last_verified_at = NOW() WHERE tenant_id = $1`, tenantID)
		return execErr
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "message": "KMS wrap+unwrap round-trip succeeded"})
}
