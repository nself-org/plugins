package main

// handlers_billing.go — GET /v1/billing: tier, quota usage, portal link.
//
// Purpose: the dashboard's Billing screen — current tier, per-dimension
//   used/limit (same map as /v1/me), and the Stripe customer-portal link.
//   The portal link is a STUB until W4's Stripe integration mints real
//   per-customer portal sessions; it points at the public billing page so
//   the upgrade funnel works today.
// Outputs: {"billing":{"tier","quotas":{dim:{"used","limit"}},"portal_url"}}.

import (
	"net/http"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// handleBilling — GET /v1/billing.
func (g *gateway) handleBilling(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(ctx)

	tenant, err := saas.GetTenant(ctx, g.db, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant lookup failed")
		return
	}
	limits := tenant.LimitsFor()

	writeJSON(w, http.StatusOK, map[string]any{
		"billing": map[string]any{
			"tier":   tenant.Tier,
			"quotas": g.quotaUsage(ctx, tenantID, limits),
			// Stub until W4 mints per-customer Stripe portal sessions.
			"portal_url": saas.UpgradeURL,
		},
	})
}
