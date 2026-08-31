package internal

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// sa returns the source_account_id from the request header, defaulting to "primary".
func sa(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-Id")
	if v == "" {
		return "primary"
	}
	return v
}

// writeJSON writes v as JSON with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a {"error":"..."} response.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ---- Verifications ----

// CreateVerification handles POST /api/v1/verifications.
func CreateVerification(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			UserID      string  `json:"user_id"`
			GroupID     *string `json:"group_id"`
			Status      string  `json:"status"`
			RedirectURL *string `json:"redirect_url"`
			IDmeUUID    *string `json:"idme_uuid"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.UserID == "" {
			errJSON(w, http.StatusBadRequest, "user_id is required")
			return
		}
		status := body.Status
		if status == "" {
			status = "pending"
		}

		ctx := context.Background()
		var v Verification
		err := db.Pool.QueryRow(ctx, `
			INSERT INTO idme_verifications
				(source_account_id, user_id, idme_uuid, email, verified, verification_level, created_at, updated_at, last_synced_at)
			VALUES ($1, $2, $3, '', false, $4, now(), now(), now())
			RETURNING id, source_account_id, user_id, idme_uuid, email, verified, verification_level,
			          first_name, last_name, birth_date, zip, phone, access_token, refresh_token,
			          token_expires_at, verified_at, last_synced_at, created_at, updated_at`,
			sa(r), body.UserID, body.IDmeUUID, status,
		).Scan(
			&v.ID, &v.SourceAccountID, &v.UserID, &v.IDmeUUID, &v.Email, &v.Verified,
			&v.VerificationLevel, &v.FirstName, &v.LastName, &v.BirthDate, &v.Zip, &v.Phone,
			&v.AccessToken, &v.RefreshToken, &v.TokenExpiresAt, &v.VerifiedAt,
			&v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to create verification")
			return
		}
		writeJSON(w, http.StatusCreated, v)
	}
}

// ListVerifications handles GET /api/v1/verifications.
func ListVerifications(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		rows, err := db.Pool.Query(ctx, `
			SELECT id, source_account_id, user_id, idme_uuid, email, verified, verification_level,
			       first_name, last_name, birth_date, zip, phone, access_token, refresh_token,
			       token_expires_at, verified_at, last_synced_at, created_at, updated_at
			FROM idme_verifications
			WHERE source_account_id = $1
			ORDER BY created_at DESC`, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list verifications")
			return
		}
		defer rows.Close()

		result := make([]Verification, 0)
		for rows.Next() {
			var v Verification
			if err := rows.Scan(
				&v.ID, &v.SourceAccountID, &v.UserID, &v.IDmeUUID, &v.Email, &v.Verified,
				&v.VerificationLevel, &v.FirstName, &v.LastName, &v.BirthDate, &v.Zip, &v.Phone,
				&v.AccessToken, &v.RefreshToken, &v.TokenExpiresAt, &v.VerifiedAt,
				&v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to scan verification")
				return
			}
			result = append(result, v)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// GetVerification handles GET /api/v1/verifications/{id}.
func GetVerification(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ctx := context.Background()
		var v Verification
		err := db.Pool.QueryRow(ctx, `
			SELECT id, source_account_id, user_id, idme_uuid, email, verified, verification_level,
			       first_name, last_name, birth_date, zip, phone, access_token, refresh_token,
			       token_expires_at, verified_at, last_synced_at, created_at, updated_at
			FROM idme_verifications
			WHERE id = $1 AND source_account_id = $2`, id, sa(r)).Scan(
			&v.ID, &v.SourceAccountID, &v.UserID, &v.IDmeUUID, &v.Email, &v.Verified,
			&v.VerificationLevel, &v.FirstName, &v.LastName, &v.BirthDate, &v.Zip, &v.Phone,
			&v.AccessToken, &v.RefreshToken, &v.TokenExpiresAt, &v.VerifiedAt,
			&v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "verification not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to get verification")
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

// VerificationCallback handles POST /api/v1/verifications/callback.
func VerificationCallback(db *DB, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code            string `json:"code"`
			State           string `json:"state"`
			UserID          string `json:"user_id"`
			SourceAccountID string `json:"source_account_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.Code == "" {
			errJSON(w, http.StatusBadRequest, "code is required")
			return
		}

		// Record that a callback was received; token exchange happens upstream.
		ctx := context.Background()
		sourceAccountID := body.SourceAccountID
		if sourceAccountID == "" {
			sourceAccountID = sa(r)
		}
		now := time.Now().UTC()
		var v Verification
		err := db.Pool.QueryRow(ctx, `
			INSERT INTO idme_verifications
				(source_account_id, user_id, email, verified, verification_level, last_synced_at, created_at, updated_at)
			VALUES ($1, $2, '', false, 'callback_received', $3, $3, $3)
			RETURNING id, source_account_id, user_id, idme_uuid, email, verified, verification_level,
			          first_name, last_name, birth_date, zip, phone, access_token, refresh_token,
			          token_expires_at, verified_at, last_synced_at, created_at, updated_at`,
			sourceAccountID, body.UserID, now,
		).Scan(
			&v.ID, &v.SourceAccountID, &v.UserID, &v.IDmeUUID, &v.Email, &v.Verified,
			&v.VerificationLevel, &v.FirstName, &v.LastName, &v.BirthDate, &v.Zip, &v.Phone,
			&v.AccessToken, &v.RefreshToken, &v.TokenExpiresAt, &v.VerifiedAt,
			&v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to record callback")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"success":          true,
			"verification_id":  v.ID,
			"source_account_id": sourceAccountID,
		})
	}
}

// ---- Groups ----

// ListGroups handles GET /api/v1/groups.
func ListGroups(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		rows, err := db.Pool.Query(ctx, `
			SELECT id, source_account_id, verification_id, user_id, group_type, group_name,
			       verified, verified_at, expires_at, affiliation, rank, status, created_at, updated_at
			FROM idme_groups
			WHERE source_account_id = $1
			ORDER BY created_at DESC`, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list groups")
			return
		}
		defer rows.Close()

		result := make([]Group, 0)
		for rows.Next() {
			var g Group
			if err := rows.Scan(
				&g.ID, &g.SourceAccountID, &g.VerificationID, &g.UserID, &g.GroupType, &g.GroupName,
				&g.Verified, &g.VerifiedAt, &g.ExpiresAt, &g.Affiliation, &g.Rank, &g.Status,
				&g.CreatedAt, &g.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to scan group")
				return
			}
			result = append(result, g)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// CreateGroup handles POST /api/v1/groups.
func CreateGroup(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			VerificationID string  `json:"verification_id"`
			UserID         string  `json:"user_id"`
			GroupType      string  `json:"group_type"`
			GroupName      string  `json:"group_name"`
			Affiliation    *string `json:"affiliation"`
			Rank           *string `json:"rank"`
			Status         *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.GroupType == "" || body.UserID == "" {
			errJSON(w, http.StatusBadRequest, "group_type and user_id are required")
			return
		}
		if body.GroupName == "" {
			body.GroupName = body.GroupType
		}

		ctx := context.Background()
		var g Group
		err := db.Pool.QueryRow(ctx, `
			INSERT INTO idme_groups
				(source_account_id, verification_id, user_id, group_type, group_name, verified,
				 affiliation, rank, status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, false, $6, $7, $8, now(), now())
			RETURNING id, source_account_id, verification_id, user_id, group_type, group_name,
			          verified, verified_at, expires_at, affiliation, rank, status, created_at, updated_at`,
			sa(r), body.VerificationID, body.UserID, body.GroupType, body.GroupName,
			body.Affiliation, body.Rank, body.Status,
		).Scan(
			&g.ID, &g.SourceAccountID, &g.VerificationID, &g.UserID, &g.GroupType, &g.GroupName,
			&g.Verified, &g.VerifiedAt, &g.ExpiresAt, &g.Affiliation, &g.Rank, &g.Status,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to create group")
			return
		}
		writeJSON(w, http.StatusCreated, g)
	}
}

// UpdateGroup handles PUT /api/v1/groups/{id}.
func UpdateGroup(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		var body struct {
			GroupType   *string `json:"group_type"`
			GroupName   *string `json:"group_name"`
			Verified    *bool   `json:"verified"`
			Affiliation *string `json:"affiliation"`
			Rank        *string `json:"rank"`
			Status      *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		ctx := context.Background()
		var g Group
		err := db.Pool.QueryRow(ctx, `
			UPDATE idme_groups SET
				group_type   = COALESCE($3, group_type),
				group_name   = COALESCE($4, group_name),
				verified     = COALESCE($5, verified),
				affiliation  = COALESCE($6, affiliation),
				rank         = COALESCE($7, rank),
				status       = COALESCE($8, status),
				updated_at   = now()
			WHERE id = $1 AND source_account_id = $2
			RETURNING id, source_account_id, verification_id, user_id, group_type, group_name,
			          verified, verified_at, expires_at, affiliation, rank, status, created_at, updated_at`,
			id, sa(r), body.GroupType, body.GroupName, body.Verified,
			body.Affiliation, body.Rank, body.Status,
		).Scan(
			&g.ID, &g.SourceAccountID, &g.VerificationID, &g.UserID, &g.GroupType, &g.GroupName,
			&g.Verified, &g.VerifiedAt, &g.ExpiresAt, &g.Affiliation, &g.Rank, &g.Status,
			&g.CreatedAt, &g.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "group not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to update group")
			return
		}
		writeJSON(w, http.StatusOK, g)
	}
}

// DeleteGroup handles DELETE /api/v1/groups/{id}.
func DeleteGroup(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		ctx := context.Background()
		tag, err := db.Pool.Exec(ctx,
			`DELETE FROM idme_groups WHERE id = $1 AND source_account_id = $2`,
			id, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to delete group")
			return
		}
		if tag.RowsAffected() == 0 {
			errJSON(w, http.StatusNotFound, "group not found")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ---- User sub-resources ----

// GetUserVerifications handles GET /api/v1/users/{user_id}/verifications.
func GetUserVerifications(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "user_id")
		ctx := context.Background()
		rows, err := db.Pool.Query(ctx, `
			SELECT id, source_account_id, user_id, idme_uuid, email, verified, verification_level,
			       first_name, last_name, birth_date, zip, phone, access_token, refresh_token,
			       token_expires_at, verified_at, last_synced_at, created_at, updated_at
			FROM idme_verifications
			WHERE user_id = $1 AND source_account_id = $2
			ORDER BY created_at DESC`, userID, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list user verifications")
			return
		}
		defer rows.Close()

		result := make([]Verification, 0)
		for rows.Next() {
			var v Verification
			if err := rows.Scan(
				&v.ID, &v.SourceAccountID, &v.UserID, &v.IDmeUUID, &v.Email, &v.Verified,
				&v.VerificationLevel, &v.FirstName, &v.LastName, &v.BirthDate, &v.Zip, &v.Phone,
				&v.AccessToken, &v.RefreshToken, &v.TokenExpiresAt, &v.VerifiedAt,
				&v.LastSyncedAt, &v.CreatedAt, &v.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to scan verification")
				return
			}
			result = append(result, v)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// GetUserBadges handles GET /api/v1/users/{user_id}/badges.
func GetUserBadges(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := chi.URLParam(r, "user_id")
		ctx := context.Background()
		rows, err := db.Pool.Query(ctx, `
			SELECT id, source_account_id, verification_id, user_id, badge_type, badge_name,
			       badge_icon, badge_color, verified_at, expires_at, active, display_order,
			       created_at, updated_at
			FROM idme_badges
			WHERE user_id = $1 AND source_account_id = $2
			ORDER BY display_order ASC, created_at DESC`, userID, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list user badges")
			return
		}
		defer rows.Close()

		result := make([]Badge, 0)
		for rows.Next() {
			var b Badge
			if err := rows.Scan(
				&b.ID, &b.SourceAccountID, &b.VerificationID, &b.UserID, &b.BadgeType, &b.BadgeName,
				&b.BadgeIcon, &b.BadgeColor, &b.VerifiedAt, &b.ExpiresAt, &b.Active, &b.DisplayOrder,
				&b.CreatedAt, &b.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to scan badge")
				return
			}
			result = append(result, b)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// ---- Badges ----

// ListBadges handles GET /api/v1/badges.
func ListBadges(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		rows, err := db.Pool.Query(ctx, `
			SELECT id, source_account_id, verification_id, user_id, badge_type, badge_name,
			       badge_icon, badge_color, verified_at, expires_at, active, display_order,
			       created_at, updated_at
			FROM idme_badges
			WHERE source_account_id = $1
			ORDER BY display_order ASC, created_at DESC`, sa(r))
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to list badges")
			return
		}
		defer rows.Close()

		result := make([]Badge, 0)
		for rows.Next() {
			var b Badge
			if err := rows.Scan(
				&b.ID, &b.SourceAccountID, &b.VerificationID, &b.UserID, &b.BadgeType, &b.BadgeName,
				&b.BadgeIcon, &b.BadgeColor, &b.VerifiedAt, &b.ExpiresAt, &b.Active, &b.DisplayOrder,
				&b.CreatedAt, &b.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to scan badge")
				return
			}
			result = append(result, b)
		}
		writeJSON(w, http.StatusOK, result)
	}
}

// CreateBadge handles POST /api/v1/badges.
func CreateBadge(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			VerificationID string  `json:"verification_id"`
			UserID         string  `json:"user_id"`
			BadgeType      string  `json:"badge_type"`
			BadgeName      string  `json:"badge_name"`
			BadgeIcon      *string `json:"badge_icon"`
			BadgeColor     *string `json:"badge_color"`
			DisplayOrder   int     `json:"display_order"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid request body")
			return
		}
		if body.BadgeType == "" || body.UserID == "" {
			errJSON(w, http.StatusBadRequest, "badge_type and user_id are required")
			return
		}
		if body.BadgeName == "" {
			body.BadgeName = body.BadgeType
		}

		ctx := context.Background()
		var b Badge
		err := db.Pool.QueryRow(ctx, `
			INSERT INTO idme_badges
				(source_account_id, verification_id, user_id, badge_type, badge_name,
				 badge_icon, badge_color, active, display_order, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, true, $8, now(), now())
			RETURNING id, source_account_id, verification_id, user_id, badge_type, badge_name,
			          badge_icon, badge_color, verified_at, expires_at, active, display_order,
			          created_at, updated_at`,
			sa(r), body.VerificationID, body.UserID, body.BadgeType, body.BadgeName,
			body.BadgeIcon, body.BadgeColor, body.DisplayOrder,
		).Scan(
			&b.ID, &b.SourceAccountID, &b.VerificationID, &b.UserID, &b.BadgeType, &b.BadgeName,
			&b.BadgeIcon, &b.BadgeColor, &b.VerifiedAt, &b.ExpiresAt, &b.Active, &b.DisplayOrder,
			&b.CreatedAt, &b.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, "failed to create badge")
			return
		}
		writeJSON(w, http.StatusCreated, b)
	}
}

// ---- Stats ----

// GetStats handles GET /api/v1/stats.
func GetStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()
		sourceAccountID := sa(r)
		var s Stats

		queries := []struct {
			dest  *int
			query string
		}{
			{&s.TotalVerifications, `SELECT COUNT(*) FROM idme_verifications WHERE source_account_id = $1`},
			{&s.VerifiedCount, `SELECT COUNT(*) FROM idme_verifications WHERE source_account_id = $1 AND verified = true`},
			{&s.TotalGroups, `SELECT COUNT(*) FROM idme_groups WHERE source_account_id = $1`},
			{&s.TotalBadges, `SELECT COUNT(*) FROM idme_badges WHERE source_account_id = $1`},
			{&s.TotalWebhookEvents, `SELECT COUNT(*) FROM idme_webhook_events WHERE source_account_id = $1`},
		}
		for _, q := range queries {
			if err := db.Pool.QueryRow(ctx, q.query, sourceAccountID).Scan(q.dest); err != nil {
				errJSON(w, http.StatusInternalServerError, "failed to fetch stats")
				return
			}
		}
		writeJSON(w, http.StatusOK, s)
	}
}
