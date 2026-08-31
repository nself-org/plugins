package internal

// Purpose: P3 billing-backend additions to the entitlements plugin — subscription
// lifecycle mutations (pause/resume/upgrade), proration preview, batch
// entitlement evaluation, and feature-grant CRUD. These move nchat's in-process
// billing logic server-side. All queries are scoped by source_account_id
// (Convention A — Multi-App Isolation).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// ============================================================
// Subscription lifecycle: pause / resume / upgrade / proration
// ============================================================

// PauseSubscription marks an active subscription as paused. Idempotent: pausing
// an already-paused subscription is a no-op success.
func (h *Handlers) PauseSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions
		   SET status='paused', updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2 AND status IN ('active','trialing')`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("pause: %v", err)})
		return
	}
	if tag.RowsAffected() == 0 {
		// Either not found or already paused/canceled — disambiguate.
		var status string
		qErr := h.pool.QueryRow(ctx,
			`SELECT status FROM entitlement_subscriptions WHERE id=$1 AND source_account_id=$2`, id, sa).Scan(&status)
		if qErr != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "paused"})
}

// ResumeSubscription returns a paused subscription to active.
func (h *Handlers) ResumeSubscription(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	tag, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions
		   SET status='active', updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2 AND status='paused'`,
		id, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("resume: %v", err)})
		return
	}
	if tag.RowsAffected() == 0 {
		var status string
		qErr := h.pool.QueryRow(ctx,
			`SELECT status FROM entitlement_subscriptions WHERE id=$1 AND source_account_id=$2`, id, sa).Scan(&status)
		if qErr != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": status})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "status": "active"})
}

// ProrationPreview computes the prorated credit/charge for switching the given
// subscription to a target plan, based on time remaining in the current period.
// This is a deterministic local estimate (not a Stripe call) suitable for UI
// previews; the authoritative proration is applied by Stripe at upgrade time.
func (h *Handlers) ProrationPreview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	targetPlanID := r.URL.Query().Get("plan_id")
	if targetPlanID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id query param is required"})
		return
	}

	var curPrice int
	var periodStart, periodEnd time.Time
	err := h.pool.QueryRow(ctx,
		`SELECT price_cents, current_period_start, current_period_end
		 FROM entitlement_subscriptions WHERE id=$1 AND source_account_id=$2`, id, sa,
	).Scan(&curPrice, &periodStart, &periodEnd)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
		return
	}

	var newPrice int
	if err := h.pool.QueryRow(ctx,
		`SELECT price_cents FROM entitlement_plans WHERE id=$1 AND source_account_id=$2`, targetPlanID, sa,
	).Scan(&newPrice); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "target plan not found"})
		return
	}

	now := time.Now().UTC()
	totalSecs := periodEnd.Sub(periodStart).Seconds()
	remainingSecs := periodEnd.Sub(now).Seconds()
	fraction := 0.0
	if totalSecs > 0 {
		fraction = remainingSecs / totalSecs
		fraction = math.Max(0, math.Min(1, fraction))
	}

	unusedCredit := int(math.Round(float64(curPrice) * fraction))
	newCharge := int(math.Round(float64(newPrice) * fraction))
	amountDue := newCharge - unusedCredit

	writeJSON(w, http.StatusOK, map[string]any{
		"subscription_id":     id,
		"target_plan_id":      targetPlanID,
		"current_price_cents": curPrice,
		"target_price_cents":  newPrice,
		"unused_credit_cents": unusedCredit,
		"new_charge_cents":    newCharge,
		"amount_due_cents":    amountDue,
		"period_fraction":     fraction,
		"currency":            "USD",
	})
}

// UpgradePlan switches a subscription to a new plan, updating price_cents and
// billing_interval in the local database. This is a LOCAL-RECORD-ONLY update —
// it does NOT call Stripe or apply proration at the payment-processor level.
// Callers that need Stripe-side proration must call the stripe plugin's
// subscription-update endpoint separately.
//
// TODO: wire to stripe plugin subscription update so this is a single atomic
//       operation (local + Stripe). Track at: plugins-pro/paid/entitlements
//       stripe-sync-upgrade residue. Until wired, response includes
//       stripe_sync_pending: true so callers can handle the two-step flow.
func (h *Handlers) UpgradePlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	id := chi.URLParam(r, "id")
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	targetPlanID, _ := body["plan_id"].(string)
	if targetPlanID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "plan_id is required"})
		return
	}

	var newPrice int
	var newInterval string
	if err := h.pool.QueryRow(ctx,
		`SELECT price_cents, billing_interval FROM entitlement_plans WHERE id=$1 AND source_account_id=$2`,
		targetPlanID, sa,
	).Scan(&newPrice, &newInterval); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "target plan not found"})
		return
	}

	tag, err := h.pool.Exec(ctx,
		`UPDATE entitlement_subscriptions
		   SET plan_id=$3, price_cents=$4, billing_interval=$5, updated_at=NOW()
		 WHERE id=$1 AND source_account_id=$2 AND status IN ('active','trialing','paused')`,
		id, sa, targetPlanID, newPrice, newInterval)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("upgrade: %v", err)})
		return
	}
	if tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found or not upgradeable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"success":            true,
		"plan_id":            targetPlanID,
		"price_cents":        newPrice,
		"billing_interval":   newInterval,
		"stripe_sync_pending": true,
	})
}

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
	h.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM entitlement_subscriptions s
		 JOIN entitlement_plans p ON p.id=s.plan_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND s.status='active' AND p.features ? $3`,
		sa, userID, featureKey,
	).Scan(&subCount)

	allowed := subCount > 0

	var used int
	h.pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(usage_amount),0) FROM entitlement_usage
		 WHERE source_account_id=$1 AND ($2='' OR user_id=$2) AND quota_key=$3`,
		sa, userID, featureKey,
	).Scan(&used)

	var limitValue *int
	h.pool.QueryRow(ctx,
		`SELECT q.limit_value FROM entitlement_quotas q
		 JOIN entitlement_subscriptions s ON s.plan_id=q.plan_id
		 JOIN entitlement_features f ON f.id=q.feature_id
		 WHERE s.source_account_id=$1 AND ($2='' OR s.user_id=$2)
		   AND f.key=$3 AND s.status='active'
		 LIMIT 1`,
		sa, userID, featureKey,
	).Scan(&limitValue)

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
