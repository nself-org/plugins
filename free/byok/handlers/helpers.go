package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/nself-byok/kms"
	"github.com/nself-org/nself-byok/tenantdb"
)

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
	id, err := uuid.Parse(val)
	if err != nil {
		return uuid.Nil, err
	}
	if id == uuid.Nil {
		return uuid.Nil, fmt.Errorf("zero tenant_id")
	}
	return id, nil
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
	_ = tenantdb.With(ctx, h.db, tenantID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO np_encryption_key_events
		   (tenant_id, event_type, key_ref, triggered_by, details)
		 VALUES ($1, $2, $3, $4, $5)`,
			tenantID, eventType, keyRef, trig, details,
		)
		return err
	})
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
