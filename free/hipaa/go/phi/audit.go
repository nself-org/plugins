package phi

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PHIAuditEntry represents one row in np_phi_audit_log.
type PHIAuditEntry struct {
	ID               string    `json:"id"`
	SourceAccountID  string    `json:"source_account_id"`
	TenantID         *string   `json:"tenant_id,omitempty"`
	AccessorID       *string   `json:"accessor_id,omitempty"`
	AccessorEmail    *string   `json:"accessor_email,omitempty"`
	TableName        string    `json:"table_name"`
	ColumnNames      []string  `json:"column_names"`
	RowCount         int       `json:"row_count"`
	Purpose          *string   `json:"purpose,omitempty"`
	QueryHash        *string   `json:"query_hash,omitempty"`
	AccessedAt       time.Time `json:"accessed_at"`
	RetainUntil      string    `json:"retain_until"`
}

// LogPHIAccessRequest is the body for manually appending a PHI access entry.
type LogPHIAccessRequest struct {
	AccessorID    *string  `json:"accessor_id,omitempty"`
	AccessorEmail *string  `json:"accessor_email,omitempty"`
	TableName     string   `json:"table_name"`
	ColumnNames   []string `json:"column_names"`
	RowCount      int      `json:"row_count"`
	Purpose       *string  `json:"purpose,omitempty"`
	QueryHash     *string  `json:"query_hash,omitempty"`
}

// AppendAuditEntry writes one PHI access log entry.
// Called by the Hasura Event Trigger webhook and optionally by application code.
func AppendAuditEntry(ctx context.Context, pool *pgxpool.Pool, acc string, req LogPHIAccessRequest) error {
	if req.TableName == "" || len(req.ColumnNames) == 0 {
		return fmt.Errorf("table_name and column_names are required")
	}
	colJSON, err := json.Marshal(req.ColumnNames)
	if err != nil {
		return fmt.Errorf("marshal column_names: %w", err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO np_phi_audit_log
		  (source_account_id, accessor_id, accessor_email, table_name,
		   column_names, row_count, purpose, query_hash)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		acc, req.AccessorID, req.AccessorEmail, req.TableName,
		colJSON, req.RowCount, req.Purpose, req.QueryHash,
	)
	return err
}

// ListAuditLogHandler returns PHI access log entries with optional filters.
func ListAuditLogHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:read", "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:read role required")
			return
		}
		acc := sourceAccount(r)
		q := r.URL.Query()

		// Optional filters
		from := q.Get("from")
		to := q.Get("to")
		userID := q.Get("accessor_id")
		limit := 100
		if l := q.Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 1000 {
				limit = v
			}
		}

		args := []any{acc}
		where := "WHERE source_account_id = $1"
		argIdx := 2

		if from != "" {
			where += fmt.Sprintf(" AND accessed_at >= $%d", argIdx)
			args = append(args, from)
			argIdx++
		}
		if to != "" {
			where += fmt.Sprintf(" AND accessed_at <= $%d", argIdx)
			args = append(args, to)
			argIdx++
		}
		if userID != "" {
			where += fmt.Sprintf(" AND accessor_id = $%d", argIdx)
			args = append(args, userID)
			argIdx++
		}
		args = append(args, limit)

		rows, err := pool.Query(r.Context(), fmt.Sprintf(`
			SELECT id, source_account_id, tenant_id, accessor_id, accessor_email,
			       table_name, column_names, row_count, purpose, query_hash,
			       accessed_at, retain_until::TEXT
			FROM np_phi_audit_log %s
			ORDER BY accessed_at DESC LIMIT $%d`, where, argIdx), args...)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		entries, err := scanAuditEntries(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, entries)
	}
}

// ExportAuditLogHandler streams the audit log as CSV for auditor review.
func ExportAuditLogHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !hasRole(r, "phi:admin", "hipaa:admin") {
			writeError(w, http.StatusForbidden, "phi:admin role required for export")
			return
		}
		acc := sourceAccount(r)

		rows, err := pool.Query(r.Context(), `
			SELECT id, source_account_id, accessor_id, accessor_email,
			       table_name, column_names, row_count, purpose,
			       accessed_at, retain_until::TEXT
			FROM np_phi_audit_log
			WHERE source_account_id = $1
			ORDER BY accessed_at DESC`, acc)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="phi-audit-log.csv"`)
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{
			"id", "source_account_id", "accessor_id", "accessor_email",
			"table_name", "column_names", "row_count", "purpose",
			"accessed_at", "retain_until",
		})
		for rows.Next() {
			var e PHIAuditEntry
			if err := rows.Scan(
				&e.ID, &e.SourceAccountID, &e.AccessorID, &e.AccessorEmail,
				&e.TableName, &e.ColumnNames, &e.RowCount, &e.Purpose,
				&e.AccessedAt, &e.RetainUntil,
			); err != nil {
				continue
			}
			accID := ""
			if e.AccessorID != nil {
				accID = *e.AccessorID
			}
			accEmail := ""
			if e.AccessorEmail != nil {
				accEmail = *e.AccessorEmail
			}
			purpose := ""
			if e.Purpose != nil {
				purpose = *e.Purpose
			}
			colB, _ := json.Marshal(e.ColumnNames)
			_ = cw.Write([]string{
				e.ID, e.SourceAccountID, accID, accEmail,
				e.TableName, string(colB), strconv.Itoa(e.RowCount), purpose,
				e.AccessedAt.Format(time.RFC3339), e.RetainUntil,
			})
		}
		cw.Flush()
	}
}

func scanAuditEntries(rows pgx.Rows) ([]PHIAuditEntry, error) {
	var result []PHIAuditEntry
	for rows.Next() {
		var e PHIAuditEntry
		var colJSON []byte
		if err := rows.Scan(
			&e.ID, &e.SourceAccountID, &e.TenantID, &e.AccessorID, &e.AccessorEmail,
			&e.TableName, &colJSON, &e.RowCount, &e.Purpose, &e.QueryHash,
			&e.AccessedAt, &e.RetainUntil,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(colJSON, &e.ColumnNames)
		result = append(result, e)
	}
	if result == nil {
		result = []PHIAuditEntry{}
	}
	return result, rows.Err()
}
