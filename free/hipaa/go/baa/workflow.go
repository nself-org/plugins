// Package baa manages Business Associate Agreement workflow and status.
package baa

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BAARecord represents one BAA in np_baa_records.
type BAARecord struct {
	ID                string     `json:"id"`
	SourceAccountID   string     `json:"source_account_id"`
	TenantID          *string    `json:"tenant_id,omitempty"`
	SignedBy          string     `json:"signed_by"`
	SignerEmail        *string   `json:"signer_email,omitempty"`
	SignedAt          *time.Time `json:"signed_at,omitempty"`
	EffectiveDate     *string    `json:"effective_date,omitempty"`
	ExpiryDate        *string    `json:"expiry_date,omitempty"`
	DocumentURL       *string    `json:"document_url,omitempty"`
	Status            string     `json:"status"`
	TerminatedAt      *time.Time `json:"terminated_at,omitempty"`
	TerminationReason *string    `json:"termination_reason,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// GetBAAHandler returns the current BAA status for the tenant.
func GetBAAHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sourceAccount(r)
		var rec BAARecord
		err := pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, tenant_id, signed_by, signer_email,
			       signed_at, effective_date::TEXT, expiry_date::TEXT,
			       document_url, status, terminated_at, termination_reason,
			       created_at, updated_at
			FROM np_baa_records
			WHERE source_account_id = $1
			ORDER BY created_at DESC LIMIT 1`, acc,
		).Scan(
			&rec.ID, &rec.SourceAccountID, &rec.TenantID,
			&rec.SignedBy, &rec.SignerEmail,
			&rec.SignedAt, &rec.EffectiveDate, &rec.ExpiryDate,
			&rec.DocumentURL, &rec.Status, &rec.TerminatedAt,
			&rec.TerminationReason, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusOK, map[string]string{"status": "none"})
			return
		}
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)
	}
}

// RequestBAAHandler initiates a BAA signing workflow.
func RequestBAAHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:admin role required to request BAA")
			return
		}
		acc := sourceAccount(r)
		var req struct {
			SignedBy    string  `json:"signed_by"`
			SignerEmail *string `json:"signer_email,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.SignedBy == "" {
			writeError(w, http.StatusBadRequest, "signed_by is required")
			return
		}

		var rec BAARecord
		err := pool.QueryRow(r.Context(), `
			INSERT INTO np_baa_records
			  (source_account_id, signed_by, signer_email, status)
			VALUES ($1,$2,$3,'pending')
			RETURNING id, source_account_id, tenant_id, signed_by, signer_email,
			          signed_at, effective_date::TEXT, expiry_date::TEXT,
			          document_url, status, terminated_at, termination_reason,
			          created_at, updated_at`,
			acc, req.SignedBy, req.SignerEmail,
		).Scan(
			&rec.ID, &rec.SourceAccountID, &rec.TenantID,
			&rec.SignedBy, &rec.SignerEmail,
			&rec.SignedAt, &rec.EffectiveDate, &rec.ExpiryDate,
			&rec.DocumentURL, &rec.Status, &rec.TerminatedAt,
			&rec.TerminationReason, &rec.CreatedAt, &rec.UpdatedAt,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	}
}

// ActivateBAAHandler marks a pending BAA as active (e.g. after DocuSign webhook confirms signing).
func ActivateBAAHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:admin role required")
			return
		}
		acc := sourceAccount(r)
		var req struct {
			BAAID       string  `json:"baa_id"`
			DocumentURL *string `json:"document_url,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		now := time.Now().UTC()
		tag, err := pool.Exec(r.Context(), `
			UPDATE np_baa_records
			SET status = 'active', signed_at = $1,
			    effective_date = $2::DATE,
			    document_url = COALESCE($3, document_url),
			    updated_at = NOW()
			WHERE id = $4 AND source_account_id = $5 AND status = 'pending'`,
			now, now.Format("2006-01-02"), req.DocumentURL, req.BAAID, acc,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "pending BAA not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "active",
			"baa_id": req.BAAID,
		})
	}
}

// TerminateBAAHandler marks an active BAA as terminated.
func TerminateBAAHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:admin role required")
			return
		}
		acc := sourceAccount(r)
		var req struct {
			BAAID  string `json:"baa_id"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		now := time.Now().UTC()
		tag, err := pool.Exec(r.Context(), `
			UPDATE np_baa_records
			SET status = 'terminated', terminated_at = $1,
			    termination_reason = $2, updated_at = NOW()
			WHERE id = $3 AND source_account_id = $4 AND status = 'active'`,
			now, req.Reason, req.BAAID, acc,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "active BAA not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "terminated",
			"baa_id": req.BAAID,
		})
	}
}

// -------------------------------------------------------------------------
// Package-level helpers
// -------------------------------------------------------------------------

func sourceAccount(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		v = r.URL.Query().Get("source_account_id")
	}
	if v == "" {
		v = "primary"
	}
	return v
}

func hasRole(r *http.Request, roles ...string) bool {
	hdr := r.Header.Get("X-HIPAA-Role")
	for _, role := range roles {
		if hdr == role {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
