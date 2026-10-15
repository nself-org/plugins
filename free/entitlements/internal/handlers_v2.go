// Purpose: entitlements lifecycle, batch evaluation and grant handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: queries are scoped by source_account_id.
package internal

import (
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"time"
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
//
//	operation (local + Stripe). Track at: plugins-pro/paid/entitlements
//	stripe-sync-upgrade residue. Until wired, response includes
//	stripe_sync_pending: true so callers can handle the two-step flow.
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
		"success":             true,
		"plan_id":             targetPlanID,
		"price_cents":         newPrice,
		"billing_interval":    newInterval,
		"stripe_sync_pending": true,
	})
}
