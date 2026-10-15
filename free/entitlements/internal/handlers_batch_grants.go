// Purpose: entitlements lifecycle, batch evaluation and grant handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: queries are scoped by source_account_id.
package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"time"
)

// ============================================================
// Batch entitlement evaluation
// ============================================================

// BatchCheckEntitlement evaluates up to 100 feature keys in a single request,
// returning an allowed/remaining map keyed by feature_key. Mirrors the single
// CheckEntitlement semantics per key.
func (h *Handlers) BatchCheckEntitlement(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body struct {
		UserID      string   `json:"user_id"`
		FeatureKeys []string `json:"feature_keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if len(body.FeatureKeys) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_keys is required"})
		return
	}
	if len(body.FeatureKeys) > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_keys exceeds maximum of 100"})
		return
	}

	results := make(map[string]map[string]any, len(body.FeatureKeys))
	for _, key := range body.FeatureKeys {
		if key == "" {
			continue
		}
		results[key] = h.evaluateOne(ctx, sa, body.UserID, key)
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// evaluateOne returns the allowed/remaining decision for a single feature key.
// Shares the logic of CheckEntitlement so single and batch paths cannot drift.
func (h *Handlers) evaluateOne(ctx context.Context, sa, userID, featureKey string) map[string]any {
	var subCount int
	if err := h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM entitlement_subscriptions s
		 JOIN entitlement_plans p ON p.id=s.plan_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND s.status='active' AND p.features ? $3`,
		sa, userID, featureKey,
	).Scan(&subCount); err != nil {
		log.Printf("entitlement subCount lookup: %v", err)
	}

	allowed := subCount > 0

	var used int
	if err := h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(usage_amount),0) FROM entitlement_usage
		 WHERE source_account_id=$1 AND ($2='' OR user_id=$2) AND quota_key=$3`,
		sa, userID, featureKey,
	).Scan(&used); err != nil {
		log.Printf("entitlement used lookup: %v", err)
	}

	var limitValue *int
	if err := h.pool.QueryRow(ctx,
		`SELECT q.limit_value FROM entitlement_quotas q
		 JOIN entitlement_subscriptions s ON s.plan_id=q.plan_id
		 JOIN entitlement_features f ON f.id=q.feature_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND f.key=$3 AND s.status='active'
		 LIMIT 1`,
		sa, userID, featureKey,
	).Scan(&limitValue); err != nil {
		log.Printf("entitlement limitValue lookup: %v", err)
	}

	remaining := -1
	if limitValue != nil {
		remaining = *limitValue - used
		if remaining < 0 {
			remaining = 0
		}
	}
	return map[string]any{"allowed": allowed, "remaining": remaining}
}

// ============================================================
// Feature grants CRUD
// ============================================================

// Grant is an ad-hoc feature override independent of plan membership.
type Grant struct {
	ID              string     `json:"id"`
	SourceAccountID string     `json:"source_account_id"`
	UserID          *string    `json:"user_id,omitempty"`
	WorkspaceID     *string    `json:"workspace_id,omitempty"`
	FeatureKey      string     `json:"feature_key"`
	Reason          *string    `json:"reason,omitempty"`
	GrantedBy       *string    `json:"granted_by,omitempty"`
	ExpiresAt       *time.Time `json:"expires_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	Metadata        any        `json:"metadata,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (h *Handlers) ListGrants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, user_id, workspace_id, feature_key, reason,
		        granted_by, expires_at, revoked_at, metadata, created_at, updated_at
		 FROM entitlement_grants WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var grants []Grant
	for rows.Next() {
		var g Grant
		if err := rows.Scan(
			&g.ID, &g.SourceAccountID, &g.UserID, &g.WorkspaceID, &g.FeatureKey, &g.Reason,
			&g.GrantedBy, &g.ExpiresAt, &g.RevokedAt, &g.Metadata, &g.CreatedAt, &g.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		grants = append(grants, g)
	}
	if grants == nil {
		grants = []Grant{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": grants})
}

func (h *Handlers) CreateGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	featureKey, _ := body["feature_key"].(string)
	if featureKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key is required"})
		return
	}

	// Validate feature_key exists in this account's entitlement_features table.
	// Prevents garbage/DoS grant rows for unknown feature keys.
	var featureExists bool
	if err := h.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM entitlement_features WHERE key=$1 AND source_account_id=$2)`,
		featureKey, sa,
	).Scan(&featureExists); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("feature_key check: %v", err)})
		return
	}
	if !featureExists {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feature_key does not exist"})
		return
	}

	userID, _ := body["user_id"].(string)
	workspaceID, _ := body["workspace_id"].(string)
	if userID == "" && workspaceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id or workspace_id is required"})
		return
	}
	var expiresAt *time.Time
	if v, ok := body["expires_at"].(string); ok && v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			expiresAt = &t
		}
	}
	metadata := body["metadata"]
	if metadata == nil {
		metadata = map[string]any{}
	}
	metaJSON, _ := json.Marshal(metadata)
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_grants
		 (source_account_id, user_id, workspace_id, feature_key, reason, granted_by, expires_at, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`,
		sa, nullStr(userID), nullStr(workspaceID), featureKey,
		nullableString(body, "reason"), nullableString(body, "granted_by"), expiresAt, metaJSON,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}

// RevokeGrant soft-revokes a grant by stamping revoked_at. Idempotent.
func (h *Handlers) RevokeGrant(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	_, err := h.pool.Exec(ctx,
		`UPDATE entitlement_grants SET revoked_at=NOW(), updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2 AND revoked_at IS NULL`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("revoke: %v", err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
