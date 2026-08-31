// Package phi implements the PHI column registry and access logging.
package phi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PHIColumn represents a registered PHI-containing column.
type PHIColumn struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	TenantID         *string   `json:"tenant_id,omitempty"`
	TableName        string    `json:"table_name"`
	ColumnName       string    `json:"column_name"`
	PHICategory      string    `json:"phi_category"`
	DeIDMethod       string    `json:"de_id_method"`
	CreatedBy        *string   `json:"created_by,omitempty"`
	AuditNote        *string   `json:"audit_note,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// RegisterPHIColumnRequest is the request body for POST /hipaa/phi-columns.
type RegisterPHIColumnRequest struct {
	TableName   string  `json:"table_name"`
	ColumnName  string  `json:"column_name"`
	PHICategory string  `json:"phi_category"`
	DeIDMethod  string  `json:"de_id_method"`
	CreatedBy   *string `json:"created_by,omitempty"`
	AuditNote   *string `json:"audit_note,omitempty"`
}

// ListPHIColumnsHandler returns all registered PHI columns for the tenant.
func ListPHIColumnsHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sourceAccount(r)
		rows, err := pool.Query(r.Context(), `
			SELECT id, source_account_id, tenant_id, table_name, column_name,
			       phi_category, de_id_method, created_by, audit_note, created_at
			FROM np_phi_columns
			WHERE source_account_id = $1
			ORDER BY table_name, column_name`, acc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		cols, err := scanPHIColumns(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, cols)
	}
}

// RegisterPHIColumnHandler adds a column to the PHI registry.
func RegisterPHIColumnHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sourceAccount(r)
		var req RegisterPHIColumnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.TableName == "" || req.ColumnName == "" || req.PHICategory == "" {
			writeError(w, http.StatusBadRequest, "table_name, column_name, and phi_category are required")
			return
		}
		deID := req.DeIDMethod
		if deID == "" {
			deID = "mask"
		}

		var col PHIColumn
		err := pool.QueryRow(r.Context(), `
			INSERT INTO np_phi_columns
			  (source_account_id, table_name, column_name, phi_category,
			   de_id_method, created_by, audit_note)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (source_account_id, table_name, column_name) DO UPDATE
			  SET phi_category = EXCLUDED.phi_category,
			      de_id_method = EXCLUDED.de_id_method,
			      created_by   = EXCLUDED.created_by,
			      audit_note   = EXCLUDED.audit_note
			RETURNING id, source_account_id, tenant_id, table_name, column_name,
			          phi_category, de_id_method, created_by, audit_note, created_at`,
			acc, req.TableName, req.ColumnName, req.PHICategory,
			deID, req.CreatedBy, req.AuditNote,
		).Scan(
			&col.ID, &col.SourceAccountID, &col.TenantID,
			&col.TableName, &col.ColumnName, &col.PHICategory,
			&col.DeIDMethod, &col.CreatedBy, &col.AuditNote, &col.CreatedAt,
		)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, col)
	}
}

// UnregisterPHIColumnHandler removes a column from the PHI registry.
// Requires phi:admin role (validated via X-HIPAA-Role header).
func UnregisterPHIColumnHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:admin role required to unregister PHI columns")
			return
		}
		acc := sourceAccount(r)
		id := chi.URLParam(r, "id")

		var req struct {
			AuditNote string `json:"audit_note"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.AuditNote == "" {
			writeError(w, http.StatusBadRequest, "audit_note is required when unregistering a PHI column")
			return
		}

		tag, err := pool.Exec(r.Context(), `
			DELETE FROM np_phi_columns
			WHERE id = $1 AND source_account_id = $2`, id, acc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if tag.RowsAffected() == 0 {
			writeError(w, http.StatusNotFound, "phi column not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"status": "unregistered",
			"id":     id,
		})
	}
}

// LookupPHIColumns returns the PHI column entries for a given table.
// Used internally by the access logger.
func LookupPHIColumns(ctx context.Context, pool *pgxpool.Pool, acc, tableName string) ([]PHIColumn, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, source_account_id, tenant_id, table_name, column_name,
		       phi_category, de_id_method, created_by, audit_note, created_at
		FROM np_phi_columns
		WHERE source_account_id = $1 AND table_name = $2`, acc, tableName)
	if err != nil {
		return nil, fmt.Errorf("lookup phi columns: %w", err)
	}
	defer rows.Close()
	return scanPHIColumns(rows)
}

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func scanPHIColumns(rows pgx.Rows) ([]PHIColumn, error) {
	var result []PHIColumn
	for rows.Next() {
		var c PHIColumn
		if err := rows.Scan(
			&c.ID, &c.SourceAccountID, &c.TenantID,
			&c.TableName, &c.ColumnName, &c.PHICategory,
			&c.DeIDMethod, &c.CreatedBy, &c.AuditNote, &c.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	if result == nil {
		result = []PHIColumn{}
	}
	return result, rows.Err()
}
