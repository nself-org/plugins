// Purpose: entitlements HTTP handlers.
// Inputs: HTTP requests and the plugin database pool.
// Outputs: JSON responses.
// Constraints: requests are scoped by source_account_id.
package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// ============================================================
// Addons
// ============================================================

func (h *Handlers) ListAddons(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	rows, err := h.pool.Query(ctx,
		`SELECT id, source_account_id, subscription_id, addon_plan_id, quantity, price_cents, currency,
		        status, created_at, updated_at
		 FROM entitlement_addons WHERE source_account_id=$1 ORDER BY created_at DESC`, sa)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("query: %v", err)})
		return
	}
	defer rows.Close()
	var addons []Addon
	for rows.Next() {
		var a Addon
		if err := rows.Scan(
			&a.ID, &a.SourceAccountID, &a.SubscriptionID, &a.AddonPlanID,
			&a.Quantity, &a.PriceCents, &a.Currency, &a.Status, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("scan: %v", err)})
			return
		}
		addons = append(addons, a)
	}
	if addons == nil {
		addons = []Addon{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": addons})
}

func (h *Handlers) CreateAddon(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sa := h.sa(r)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	subscriptionID, _ := body["subscription_id"].(string)
	addonPlanID, _ := body["addon_plan_id"].(string)
	if subscriptionID == "" || addonPlanID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subscription_id and addon_plan_id are required"})
		return
	}
	quantity := 1
	if v, ok := body["quantity"].(float64); ok {
		quantity = int(v)
	}
	var id string
	err := h.pool.QueryRow(ctx,
		`INSERT INTO entitlement_addons (source_account_id, subscription_id, addon_plan_id, quantity, price_cents, currency, status)
		 VALUES ($1,$2,$3,$4,0,'USD','active') RETURNING id`,
		sa, subscriptionID, addonPlanID, quantity,
	).Scan(&id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("insert: %v", err)})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"success": true, "id": id})
}
