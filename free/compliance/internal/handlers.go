package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// -------------------------------------------------------------------------
// Helpers
// -------------------------------------------------------------------------

func sa(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		v = r.URL.Query().Get("source_account_id")
	}
	if v == "" {
		v = "primary"
	}
	return v
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, dst any) error {
	return json.NewDecoder(r.Body).Decode(dst)
}

// -------------------------------------------------------------------------
// Health
// -------------------------------------------------------------------------

func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "compliance",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func ReadyHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ready":  false,
				"plugin": "compliance",
				"error":  "database unavailable",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ready":     true,
			"plugin":    "compliance",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// -------------------------------------------------------------------------
// DSARs
// -------------------------------------------------------------------------

func ListDSARsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, request_type, request_number,
			       user_id, requester_email, requester_name, description,
			       data_categories, status, assigned_to, started_at,
			       completed_at, deadline, resolution_notes, rejection_reason,
			       regulation, jurisdiction, legal_basis, ip_address,
			       user_agent, created_at, updated_at
			FROM compliance_dsars
			WHERE source_account_id = $1
			ORDER BY created_at DESC`, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		dsars, err := scanDSARs(rows)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, dsars)
	}
}

func CreateDSARHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		var req CreateDSARRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.RequestType == "" || req.Email == "" {
			errJSON(w, http.StatusBadRequest, "request_type and email are required")
			return
		}

		reg := "GDPR"
		if req.Regulation != nil && *req.Regulation != "" {
			reg = *req.Regulation
		}
		// DSARs default deadline: 30 days
		deadline := time.Now().UTC().Add(30 * 24 * time.Hour)
		reqNum := fmt.Sprintf("DSAR-%d", time.Now().UnixMilli())

		var d DSAR
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO compliance_dsars
			  (source_account_id, request_type, request_number, user_id,
			   requester_email, requester_name, description, data_categories,
			   specific_data_requested, status, deadline, regulation, jurisdiction)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending',$10,$11,$12)
			RETURNING id, source_account_id, request_type, request_number,
			          user_id, requester_email, requester_name, description,
			          data_categories, status, assigned_to, started_at,
			          completed_at, deadline, resolution_notes, rejection_reason,
			          regulation, jurisdiction, legal_basis, ip_address,
			          user_agent, created_at, updated_at`,
			acc, req.RequestType, reqNum, req.UserID,
			req.Email, req.Name, req.Description,
			req.DataCategories, req.SpecificDataRequested,
			deadline, reg, req.Jurisdiction,
		).Scan(
			&d.ID, &d.SourceAccountID, &d.RequestType, &d.RequestNumber,
			&d.UserID, &d.RequesterEmail, &d.RequesterName, &d.Description,
			&d.DataCategories, &d.Status, &d.AssignedTo, &d.StartedAt,
			&d.CompletedAt, &d.Deadline, &d.ResolutionNotes, &d.RejectionReason,
			&d.Regulation, &d.Jurisdiction, &d.LegalBasis, &d.IPAddress,
			&d.UserAgent, &d.CreatedAt, &d.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

func GetDSARHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		var d DSAR
		err := db.Pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, request_type, request_number,
			       user_id, requester_email, requester_name, description,
			       data_categories, status, assigned_to, started_at,
			       completed_at, deadline, resolution_notes, rejection_reason,
			       regulation, jurisdiction, legal_basis, ip_address,
			       user_agent, created_at, updated_at
			FROM compliance_dsars
			WHERE id = $1 AND source_account_id = $2`, id, acc,
		).Scan(
			&d.ID, &d.SourceAccountID, &d.RequestType, &d.RequestNumber,
			&d.UserID, &d.RequesterEmail, &d.RequesterName, &d.Description,
			&d.DataCategories, &d.Status, &d.AssignedTo, &d.StartedAt,
			&d.CompletedAt, &d.Deadline, &d.ResolutionNotes, &d.RejectionReason,
			&d.Regulation, &d.Jurisdiction, &d.LegalBasis, &d.IPAddress,
			&d.UserAgent, &d.CreatedAt, &d.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "dsar not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func UpdateDSARHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		var body map[string]any
		if err := decode(r, &body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}

		// Build dynamic SET clause from provided fields
		allowed := []string{"status", "assigned_to", "resolution_notes", "rejection_reason", "description"}
		setClauses := []string{"updated_at = NOW()"}
		args := []any{}
		argIdx := 1
		for _, f := range allowed {
			if v, ok := body[f]; ok {
				setClauses = append(setClauses, fmt.Sprintf("%s = $%d", f, argIdx))
				args = append(args, v)
				argIdx++
			}
		}
		args = append(args, id, acc)

		_, err := db.Pool.Exec(r.Context(),
			fmt.Sprintf("UPDATE compliance_dsars SET %s WHERE id = $%d AND source_account_id = $%d",
				strings.Join(setClauses, ", "), argIdx, argIdx+1),
			args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		GetDSARHandler(db)(w, r)
	}
}

func CompleteDSARHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		now := time.Now().UTC()
		_, err := db.Pool.Exec(r.Context(), `
			UPDATE compliance_dsars
			SET status = 'completed', completed_at = $1, updated_at = NOW()
			WHERE id = $2 AND source_account_id = $3`, now, id, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		logDSARActivity(r.Context(), db, acc, id, "completed", "DSAR marked as completed", nil)
		GetDSARHandler(db)(w, r)
	}
}

func DenyDSARHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		var body struct {
			Reason string `json:"reason"`
		}
		_ = decode(r, &body)

		_, err := db.Pool.Exec(r.Context(), `
			UPDATE compliance_dsars
			SET status = 'rejected', rejection_reason = $1, updated_at = NOW()
			WHERE id = $2 AND source_account_id = $3`, body.Reason, id, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		logDSARActivity(r.Context(), db, acc, id, "denied", "DSAR denied: "+body.Reason, nil)
		GetDSARHandler(db)(w, r)
	}
}

func ListDSARActivitiesHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, dsar_id, activity_type,
			       description, performed_by, performed_by_name, metadata, created_at
			FROM compliance_dsar_activities
			WHERE dsar_id = $1 AND source_account_id = $2
			ORDER BY created_at DESC`, id, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var activities []DSARActivity
		for rows.Next() {
			var a DSARActivity
			var metadata []byte
			if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.DSARID,
				&a.ActivityType, &a.Description, &a.PerformedBy,
				&a.PerformedByName, &metadata, &a.CreatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			_ = json.Unmarshal(metadata, &a.Metadata)
			activities = append(activities, a)
		}
		if activities == nil {
			activities = []DSARActivity{}
		}
		writeJSON(w, http.StatusOK, activities)
	}
}

func logDSARActivity(ctx context.Context, db *DB, acc, dsarID, actType, desc string, metadata map[string]any) {
	meta := "{}"
	if metadata != nil {
		if b, err := json.Marshal(metadata); err == nil {
			meta = string(b)
		}
	}
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO compliance_dsar_activities
		  (source_account_id, dsar_id, activity_type, description, metadata)
		VALUES ($1, $2, $3, $4, $5)`, acc, dsarID, actType, desc, meta)
}

func scanDSARs(rows pgx.Rows) ([]DSAR, error) {
	var result []DSAR
	for rows.Next() {
		var d DSAR
		if err := rows.Scan(
			&d.ID, &d.SourceAccountID, &d.RequestType, &d.RequestNumber,
			&d.UserID, &d.RequesterEmail, &d.RequesterName, &d.Description,
			&d.DataCategories, &d.Status, &d.AssignedTo, &d.StartedAt,
			&d.CompletedAt, &d.Deadline, &d.ResolutionNotes, &d.RejectionReason,
			&d.Regulation, &d.Jurisdiction, &d.LegalBasis, &d.IPAddress,
			&d.UserAgent, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	if result == nil {
		result = []DSAR{}
	}
	return result, rows.Err()
}

// -------------------------------------------------------------------------
// Consents
// -------------------------------------------------------------------------

func ListConsentsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, user_id, purpose, purpose_description,
			       status, granted_at, denied_at, withdrawn_at, expires_at,
			       consent_method, consent_text, privacy_policy_version,
			       ip_address, user_agent, metadata, created_at, updated_at
			FROM compliance_consents
			WHERE source_account_id = $1
			ORDER BY created_at DESC`, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		consents, err := scanConsents(rows)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, consents)
	}
}

func CreateConsentHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		var req CreateConsentRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.UserID == "" || req.Purpose == "" {
			errJSON(w, http.StatusBadRequest, "user_id and purpose are required")
			return
		}
		if req.Status == "" {
			req.Status = "granted"
		}

		var grantedAt, deniedAt *time.Time
		now := time.Now().UTC()
		if req.Status == "granted" {
			grantedAt = &now
		} else {
			deniedAt = &now
		}

		var c Consent
		var metadata []byte
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO compliance_consents
			  (source_account_id, user_id, purpose, purpose_description, status,
			   granted_at, denied_at, expires_at, consent_method, consent_text,
			   privacy_policy_version)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id, source_account_id, user_id, purpose, purpose_description,
			          status, granted_at, denied_at, withdrawn_at, expires_at,
			          consent_method, consent_text, privacy_policy_version,
			          ip_address, user_agent, metadata, created_at, updated_at`,
			acc, req.UserID, req.Purpose, req.PurposeDescription, req.Status,
			grantedAt, deniedAt, req.ExpiresAt, req.ConsentMethod,
			req.ConsentText, req.PrivacyPolicyVersion,
		).Scan(
			&c.ID, &c.SourceAccountID, &c.UserID, &c.Purpose, &c.PurposeDescription,
			&c.Status, &c.GrantedAt, &c.DeniedAt, &c.WithdrawnAt, &c.ExpiresAt,
			&c.ConsentMethod, &c.ConsentText, &c.PrivacyPolicyVersion,
			&c.IPAddress, &c.UserAgent, &metadata, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		_ = json.Unmarshal(metadata, &c.Metadata)
		writeJSON(w, http.StatusCreated, c)
	}
}

func GetConsentHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		c, err := fetchConsent(r.Context(), db, acc, id)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "consent not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, c)
	}
}

func RevokeConsentHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		var req RevokeConsentRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.UserID == "" || req.ConsentType == "" {
			errJSON(w, http.StatusBadRequest, "user_id and consent_type are required")
			return
		}
		now := time.Now().UTC()
		tag, err := db.Pool.Exec(r.Context(), `
			UPDATE compliance_consents
			SET status = 'withdrawn', withdrawn_at = $1, updated_at = NOW()
			WHERE source_account_id = $2 AND user_id = $3 AND purpose = $4
			  AND status = 'granted'`,
			now, acc, req.UserID, req.ConsentType)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"revoked": tag.RowsAffected(),
		})
	}
}

func ListUserConsentsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		userID := chi.URLParam(r, "user_id")
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, user_id, purpose, purpose_description,
			       status, granted_at, denied_at, withdrawn_at, expires_at,
			       consent_method, consent_text, privacy_policy_version,
			       ip_address, user_agent, metadata, created_at, updated_at
			FROM compliance_consents
			WHERE source_account_id = $1 AND user_id = $2
			ORDER BY created_at DESC`, acc, userID)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		consents, err := scanConsents(rows)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, consents)
	}
}

func fetchConsent(ctx context.Context, db *DB, acc, id string) (Consent, error) {
	var c Consent
	var metadata []byte
	err := db.Pool.QueryRow(ctx, `
		SELECT id, source_account_id, user_id, purpose, purpose_description,
		       status, granted_at, denied_at, withdrawn_at, expires_at,
		       consent_method, consent_text, privacy_policy_version,
		       ip_address, user_agent, metadata, created_at, updated_at
		FROM compliance_consents
		WHERE id = $1 AND source_account_id = $2`, id, acc,
	).Scan(
		&c.ID, &c.SourceAccountID, &c.UserID, &c.Purpose, &c.PurposeDescription,
		&c.Status, &c.GrantedAt, &c.DeniedAt, &c.WithdrawnAt, &c.ExpiresAt,
		&c.ConsentMethod, &c.ConsentText, &c.PrivacyPolicyVersion,
		&c.IPAddress, &c.UserAgent, &metadata, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return c, err
	}
	_ = json.Unmarshal(metadata, &c.Metadata)
	return c, nil
}

func scanConsents(rows pgx.Rows) ([]Consent, error) {
	var result []Consent
	for rows.Next() {
		var c Consent
		var metadata []byte
		if err := rows.Scan(
			&c.ID, &c.SourceAccountID, &c.UserID, &c.Purpose, &c.PurposeDescription,
			&c.Status, &c.GrantedAt, &c.DeniedAt, &c.WithdrawnAt, &c.ExpiresAt,
			&c.ConsentMethod, &c.ConsentText, &c.PrivacyPolicyVersion,
			&c.IPAddress, &c.UserAgent, &metadata, &c.CreatedAt, &c.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(metadata, &c.Metadata)
		result = append(result, c)
	}
	if result == nil {
		result = []Consent{}
	}
	return result, rows.Err()
}

// -------------------------------------------------------------------------
// Privacy Policies
// -------------------------------------------------------------------------

func ListPrivacyPoliciesHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, version, version_number, title, content,
			       summary, changes_summary, is_active, requires_reacceptance,
			       effective_from, effective_until, language, jurisdiction,
			       created_by, created_at
			FROM compliance_privacy_policies
			WHERE source_account_id = $1
			ORDER BY version_number DESC`, acc)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()
		policies, err := scanPolicies(rows)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, policies)
	}
}

func CreatePrivacyPolicyHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		var req CreatePrivacyPolicyRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.Version == "" || req.Title == "" || req.Content == "" {
			errJSON(w, http.StatusBadRequest, "version, title, and content are required")
			return
		}
		lang := "en"
		if req.Language != nil && *req.Language != "" {
			lang = *req.Language
		}

		var p PrivacyPolicy
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO compliance_privacy_policies
			  (source_account_id, version, version_number, title, content, summary,
			   changes_summary, requires_reacceptance, effective_from, language, jurisdiction)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			RETURNING id, source_account_id, version, version_number, title, content,
			          summary, changes_summary, is_active, requires_reacceptance,
			          effective_from, effective_until, language, jurisdiction,
			          created_by, created_at`,
			acc, req.Version, req.VersionNumber, req.Title, req.Content,
			req.Summary, req.ChangesSummary, req.RequiresReacceptance,
			req.EffectiveFrom, lang, req.Jurisdiction,
		).Scan(
			&p.ID, &p.SourceAccountID, &p.Version, &p.VersionNumber,
			&p.Title, &p.Content, &p.Summary, &p.ChangesSummary,
			&p.IsActive, &p.RequiresReacceptance, &p.EffectiveFrom,
			&p.EffectiveUntil, &p.Language, &p.Jurisdiction,
			&p.CreatedBy, &p.CreatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, p)
	}
}

func GetPrivacyPolicyHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		var p PrivacyPolicy
		err := db.Pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, version, version_number, title, content,
			       summary, changes_summary, is_active, requires_reacceptance,
			       effective_from, effective_until, language, jurisdiction,
			       created_by, created_at
			FROM compliance_privacy_policies
			WHERE id = $1 AND source_account_id = $2`, id, acc,
		).Scan(
			&p.ID, &p.SourceAccountID, &p.Version, &p.VersionNumber,
			&p.Title, &p.Content, &p.Summary, &p.ChangesSummary,
			&p.IsActive, &p.RequiresReacceptance, &p.EffectiveFrom,
			&p.EffectiveUntil, &p.Language, &p.Jurisdiction,
			&p.CreatedBy, &p.CreatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "privacy policy not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func scanPolicies(rows pgx.Rows) ([]PrivacyPolicy, error) {
	var result []PrivacyPolicy
	for rows.Next() {
		var p PrivacyPolicy
		if err := rows.Scan(
			&p.ID, &p.SourceAccountID, &p.Version, &p.VersionNumber,
			&p.Title, &p.Content, &p.Summary, &p.ChangesSummary,
			&p.IsActive, &p.RequiresReacceptance, &p.EffectiveFrom,
			&p.EffectiveUntil, &p.Language, &p.Jurisdiction,
			&p.CreatedBy, &p.CreatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if result == nil {
		result = []PrivacyPolicy{}
	}
	return result, rows.Err()
}

// -------------------------------------------------------------------------
// Data Exports
// -------------------------------------------------------------------------

func CreateDataExportHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		var req CreateDataExportRequest
		if err := decode(r, &req); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if req.UserID == "" {
			errJSON(w, http.StatusBadRequest, "user_id is required")
			return
		}
		format := req.Format
		if format == "" {
			format = "json"
		}
		expires := time.Now().UTC().Add(24 * time.Hour)

		var e DataExport
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO compliance_data_exports
			  (source_account_id, user_id, format, status, expires_at)
			VALUES ($1,$2,$3,'pending',$4)
			RETURNING id, source_account_id, user_id, format, status,
			          export_url, expires_at, error_message,
			          requested_at, completed_at, created_at`,
			acc, req.UserID, format, expires,
		).Scan(
			&e.ID, &e.SourceAccountID, &e.UserID, &e.Format, &e.Status,
			&e.ExportURL, &e.ExpiresAt, &e.ErrorMessage,
			&e.RequestedAt, &e.CompletedAt, &e.CreatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, e)
	}
}

func GetDataExportHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		id := chi.URLParam(r, "id")
		var e DataExport
		err := db.Pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, user_id, format, status,
			       export_url, expires_at, error_message,
			       requested_at, completed_at, created_at
			FROM compliance_data_exports
			WHERE id = $1 AND source_account_id = $2`, id, acc,
		).Scan(
			&e.ID, &e.SourceAccountID, &e.UserID, &e.Format, &e.Status,
			&e.ExportURL, &e.ExpiresAt, &e.ErrorMessage,
			&e.RequestedAt, &e.CompletedAt, &e.CreatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "data export not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, e)
	}
}

// -------------------------------------------------------------------------
// Stats
// -------------------------------------------------------------------------

func StatsHandler(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		acc := sa(r)
		ctx := r.Context()

		var stats StatsResponse

		// DSARs
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_dsars WHERE source_account_id=$1`, acc,
		).Scan(&stats.DSARs.Total)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_dsars WHERE source_account_id=$1 AND status='pending'`, acc,
		).Scan(&stats.DSARs.Pending)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_dsars WHERE source_account_id=$1 AND status='completed'`, acc,
		).Scan(&stats.DSARs.Completed)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_dsars WHERE source_account_id=$1 AND status NOT IN ('completed','rejected','cancelled') AND deadline < NOW()`, acc,
		).Scan(&stats.DSARs.Overdue)

		// Consents
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_consents WHERE source_account_id=$1`, acc,
		).Scan(&stats.Consents.Total)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_consents WHERE source_account_id=$1 AND status='granted'`, acc,
		).Scan(&stats.Consents.Granted)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_consents WHERE source_account_id=$1 AND status='withdrawn'`, acc,
		).Scan(&stats.Consents.Withdrawn)

		// Privacy Policies
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_privacy_policies WHERE source_account_id=$1`, acc,
		).Scan(&stats.PrivacyPolicies.Total)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_privacy_policies WHERE source_account_id=$1 AND is_active=true`, acc,
		).Scan(&stats.PrivacyPolicies.Active)

		// Data Exports
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_data_exports WHERE source_account_id=$1`, acc,
		).Scan(&stats.DataExports.Total)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_data_exports WHERE source_account_id=$1 AND status='pending'`, acc,
		).Scan(&stats.DataExports.Pending)
		_ = db.Pool.QueryRow(ctx,
			`SELECT COUNT(*) FROM compliance_data_exports WHERE source_account_id=$1 AND status='completed'`, acc,
		).Scan(&stats.DataExports.Complete)

		writeJSON(w, http.StatusOK, stats)
	}
}
