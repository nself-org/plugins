package internal

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Service provides the business logic for GDPR export and delete operations,
// backed by a Postgres pool.
type Service struct {
	db *sql.DB
}

// NewService constructs a Service from an open *sql.DB.
func NewService(db *sql.DB) *Service {
	return &Service{db: db}
}

// Migrate creates np_gdpr_requests and np_gdpr_plugin_registry if they do not
// already exist. The call is idempotent (IF NOT EXISTS on all DDL).
func (s *Service) Migrate(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, migrationSQL)
	return err
}

// exportRequest is the body accepted by POST /gdpr/export.
type exportRequest struct {
	UserID string `json:"user_id"`
	Format string `json:"format"` // "json" (default) or "csv"
	DryRun bool   `json:"dry_run"`
}

// HandleExport handles POST /gdpr/export.
func (s *Service) HandleExport(w http.ResponseWriter, r *http.Request) {
	var req exportRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	requestID, err := s.createRequest(r.Context(), "export", "user", req.UserID, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"request_id": requestID,
		"status":     "pending",
		"deadline":   time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		"message":    "Export queued. Poll /gdpr/request/" + requestID + " for status.",
	})
}

// deleteRequest is the body accepted by POST /gdpr/delete.
type deleteRequest struct {
	UserID string `json:"user_id"`
	DryRun bool   `json:"dry_run"`
}

// HandleDelete handles POST /gdpr/delete.
func (s *Service) HandleDelete(w http.ResponseWriter, r *http.Request) {
	var req deleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.UserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id is required"})
		return
	}

	if req.DryRun {
		preview, err := s.dryRunDelete(r.Context(), req.UserID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"dry_run": true,
			"preview": preview,
		})
		return
	}

	requestID, err := s.createRequest(r.Context(), "delete", "user", req.UserID, nil)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"request_id": requestID,
		"status":     "pending",
		"deadline":   time.Now().AddDate(0, 0, 30).Format("2006-01-02"),
		"message":    "Deletion queued. Poll /gdpr/request/" + requestID + " for status.",
	})
}

// HandleGetRequest handles GET /gdpr/request/{id}.
func (s *Service) HandleGetRequest(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}

	row := s.db.QueryRowContext(r.Context(), `
SELECT id, request_type, subject_type, subject_id,
       requested_at, deadline, status, completed_at,
       artifact_url, notes
FROM np_gdpr_requests WHERE id = $1`, id)

	var (
		rid, rt, st, sid, status, notes sql.NullString
		reqAt, completedAt              sql.NullTime
		deadline                        sql.NullTime
		artifactURL                     sql.NullString
	)
	if err := row.Scan(&rid, &rt, &st, &sid, &reqAt, &deadline, &status, &completedAt, &artifactURL, &notes); err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "request not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":           rid.String,
		"request_type": rt.String,
		"subject_type": st.String,
		"subject_id":   sid.String,
		"requested_at": reqAt.Time.Format(time.RFC3339),
		"deadline":     deadline.Time.Format("2006-01-02"),
		"status":       status.String,
		"completed_at": nullTimeStr(completedAt),
		"artifact_url": nullStr(artifactURL),
		"notes":        nullStr(notes),
	})
}

// HandleListRequests handles GET /gdpr/requests?status=<status>.
func (s *Service) HandleListRequests(w http.ResponseWriter, r *http.Request) {
	statusFilter := r.URL.Query().Get("status")

	q := `SELECT id, request_type, subject_type, subject_id, requested_at, deadline, status
	      FROM np_gdpr_requests`
	args := []interface{}{}
	if statusFilter != "" {
		q += " WHERE status = $1"
		args = append(args, statusFilter)
	}
	q += " ORDER BY requested_at DESC LIMIT 200"

	rows, err := s.db.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	var list []map[string]interface{}
	for rows.Next() {
		var id, rt, st, sid, status string
		var reqAt, deadline time.Time
		if err := rows.Scan(&id, &rt, &st, &sid, &reqAt, &deadline, &status); err != nil {
			continue
		}
		list = append(list, map[string]interface{}{
			"id":           id,
			"request_type": rt,
			"subject_type": st,
			"subject_id":   sid,
			"requested_at": reqAt.Format(time.RFC3339),
			"deadline":     deadline.Format("2006-01-02"),
			"status":       status,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"requests": list})
}

// registerPluginRequest is the body for POST /gdpr/registry.
type registerPluginRequest struct {
	PluginName   string          `json:"plugin_name"`
	UserTables   json.RawMessage `json:"user_tables"`
	TenantTables json.RawMessage `json:"tenant_tables"`
}

// HandleRegisterPlugin handles POST /gdpr/registry to let plugins
// register their own tables into the cascade registry.
func (s *Service) HandleRegisterPlugin(w http.ResponseWriter, r *http.Request) {
	var req registerPluginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if req.PluginName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plugin_name is required"})
		return
	}
	if req.UserTables == nil {
		req.UserTables = json.RawMessage("[]")
	}
	if req.TenantTables == nil {
		req.TenantTables = json.RawMessage("[]")
	}

	_, err := s.db.ExecContext(r.Context(), `
INSERT INTO np_gdpr_plugin_registry (plugin_name, user_tables, tenant_tables)
VALUES ($1, $2, $3)
ON CONFLICT (plugin_name) DO UPDATE
  SET user_tables   = EXCLUDED.user_tables,
      tenant_tables = EXCLUDED.tenant_tables`,
		req.PluginName, req.UserTables, req.TenantTables)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "registered"})
}

// ---------- helpers ----------

func (s *Service) createRequest(ctx context.Context, requestType, subjectType, subjectID string, tenantID *string) (string, error) {
	const q = `
INSERT INTO np_gdpr_requests (request_type, subject_type, subject_id, tenant_id)
VALUES ($1, $2, $3, $4)
RETURNING id`
	var id string
	err := s.db.QueryRowContext(ctx, q, requestType, subjectType, subjectID, tenantID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	return id, nil
}

func (s *Service) dryRunDelete(ctx context.Context, userID string) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT plugin_name, user_tables FROM np_gdpr_plugin_registry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var preview []map[string]interface{}
	for rows.Next() {
		var pluginName string
		var userTablesRaw json.RawMessage
		if err := rows.Scan(&pluginName, &userTablesRaw); err != nil {
			continue
		}
		var tables []struct {
			Table    string `json:"table"`
			UserCol  string `json:"user_col"`
			Strategy string `json:"strategy"`
		}
		if err := json.Unmarshal(userTablesRaw, &tables); err != nil {
			continue
		}
		for _, tbl := range tables {
			var count int64
			q := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = $1", tbl.Table, tbl.UserCol)
			if err := s.db.QueryRowContext(ctx, q, userID).Scan(&count); err != nil {
				continue
			}
			preview = append(preview, map[string]interface{}{
				"plugin":   pluginName,
				"table":    tbl.Table,
				"strategy": tbl.Strategy,
				"rows":     count,
			})
		}
	}
	return preview, rows.Err()
}

func nullStr(ns sql.NullString) interface{} {
	if ns.Valid {
		return ns.String
	}
	return nil
}

func nullTimeStr(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time.Format(time.RFC3339)
	}
	return nil
}

// migrationSQL is the embedded schema — mirrors migrations/001_gdpr_tables.sql.
const migrationSQL = `
CREATE TABLE IF NOT EXISTS np_gdpr_requests (
    id               UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        UUID,
    request_type     TEXT        NOT NULL,
    subject_type     TEXT        NOT NULL,
    subject_id       TEXT        NOT NULL,
    requested_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deadline         DATE        NOT NULL DEFAULT (NOW() + INTERVAL '30 days'),
    status           TEXT        NOT NULL DEFAULT 'pending',
    completed_at     TIMESTAMPTZ,
    artifact_url     TEXT,
    artifact_expires TIMESTAMPTZ,
    notes            TEXT
);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_requests_subject
    ON np_gdpr_requests (subject_id, subject_type);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_requests_status
    ON np_gdpr_requests (status, deadline);

ALTER TABLE np_gdpr_requests ENABLE ROW LEVEL SECURITY;

DO $$ BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_policies WHERE tablename = 'np_gdpr_requests' AND policyname = 'gdpr_requests_select'
  ) THEN
    CREATE POLICY gdpr_requests_select ON np_gdpr_requests FOR SELECT USING (true);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_policies WHERE tablename = 'np_gdpr_requests' AND policyname = 'gdpr_requests_insert'
  ) THEN
    CREATE POLICY gdpr_requests_insert ON np_gdpr_requests FOR INSERT WITH CHECK (true);
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM pg_policies WHERE tablename = 'np_gdpr_requests' AND policyname = 'gdpr_requests_update'
  ) THEN
    CREATE POLICY gdpr_requests_update ON np_gdpr_requests FOR UPDATE USING (status NOT IN ('complete', 'failed'));
  END IF;
END $$;

CREATE TABLE IF NOT EXISTS np_gdpr_plugin_registry (
    id            UUID  PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_name   TEXT  NOT NULL UNIQUE,
    user_tables   JSONB NOT NULL DEFAULT '[]',
    tenant_tables JSONB NOT NULL DEFAULT '[]',
    registered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_np_gdpr_registry_plugin
    ON np_gdpr_plugin_registry (plugin_name);
`

// Ensure strings package is used (for authMiddleware).
var _ = strings.TrimPrefix
