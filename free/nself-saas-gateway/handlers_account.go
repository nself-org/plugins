package main

// handlers_account.go — GDPR surface: data export + account deletion.
//
// Purpose: the two rights every tenant has over their data.
//   GET    /v1/account/export — every row the platform holds for the tenant,
//     as one JSON document. Tables are discovered dynamically (any np_*
//     table with a tenant_id column), so new plugin tables export
//     automatically. Security artifacts never leave: password hashes, email
//     tokens, and API-key hashes are excluded/curated.
//   DELETE /v1/account — full purge via purgeTenant (same engine as the
//     internal endpoint). STRIPE: deletion does NOT cancel the Stripe
//     subscription itself — the billing owner is ping_api (W4); the response
//     carries the stripe_customer_id so the caller/webhook chain cancels it
//     (ping_api receives the tenant-deleted signal via the normal billing
//     reconciliation; a dangling subscription self-heals on its next webhook
//     when the tenant row is gone).
// Constraints — P0 tenancy: both operate ONLY on the tenant resolved from
//   the verified credential; there is no id parameter to spoof.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/nself-org/plugins-pro/paid/shared/saas"
)

// exportSkipTables never appear in an export (security artifacts).
var exportSkipTables = map[string]bool{
	"np_saas_email_tokens": true, // one-time secrets
}

// exportTableRows dumps a table's tenant rows as raw JSON via row_to_json.
// Best-effort: any failure returns nil (the table is reported as skipped).
func (g *gateway) exportTableRows(ctx context.Context, table, tenantID string) json.RawMessage {
	var raw []byte
	// table names come from information_schema, not user input.
	err := g.db.QueryRowContext(ctx,
		`SELECT COALESCE(json_agg(row_to_json(t)), '[]'::json)::text
		 FROM `+table+` t WHERE tenant_id = $1`, tenantID). //nolint:gosec
		Scan(&raw)
	if err != nil {
		return nil
	}
	return json.RawMessage(raw)
}

// baseTableName strips the schema qualifier from a discovered table name.
func baseTableName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// handleAccountExport — GET /v1/account/export.
func (g *gateway) handleAccountExport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(r.Context())

	// Profile: curated np_saas_tenants columns — never the password hash.
	profile := map[string]any{"tenant_id": tenantID}
	var tier string
	var email, name, stripeID sql.NullString
	var verified bool
	var createdAt time.Time
	err := g.db.QueryRowContext(ctx, `
		SELECT tier, owner_email, owner_name, stripe_customer_id,
		       COALESCE(verified, false), created_at
		FROM np_saas_tenants WHERE tenant_id = $1`, tenantID).
		Scan(&tier, &email, &name, &stripeID, &verified, &createdAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant lookup failed")
		return
	}
	if err == nil {
		profile["tier"] = tier
		profile["email"] = email.String
		profile["name"] = name.String
		profile["stripe_customer_id"] = stripeID.String
		profile["verified"] = verified
		profile["created_at"] = createdAt.UTC().Format(time.RFC3339)
	}

	tables, err := g.tenantTables(ctx)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "table discovery failed")
		return
	}

	data := map[string]json.RawMessage{}
	var skipped []string
	for _, table := range tables {
		base := baseTableName(table)
		if exportSkipTables[base] {
			continue
		}
		if base == "np_saas_api_keys" {
			// Curated: metadata only — never key hashes.
			var raw []byte
			err := g.db.QueryRowContext(ctx, `
				SELECT COALESCE(json_agg(json_build_object(
					'id', id, 'name', name, 'key_prefix', key_prefix,
					'scopes', scopes, 'created_at', created_at,
					'last_used_at', last_used_at, 'revoked_at', revoked_at
				)), '[]'::json)::text
				FROM np_saas_api_keys WHERE tenant_id = $1`, tenantID).Scan(&raw)
			if err == nil {
				data[base] = json.RawMessage(raw)
			} else {
				skipped = append(skipped, base)
			}
			continue
		}
		if rows := g.exportTableRows(ctx, table, tenantID); rows != nil {
			data[base] = rows
		} else {
			skipped = append(skipped, base)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"export": map[string]any{
			"generated_at": time.Now().UTC().Format(time.RFC3339),
			"tenant":       profile,
			"data":         data,
			"skipped":      skipped,
		},
	})
}

// handleAccountDelete — DELETE /v1/account: purge the AUTHENTICATED tenant.
func (g *gateway) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID := saas.TenantFrom(r.Context())

	// Capture the Stripe customer before the row disappears — the billing
	// layer (ping_api, W4) owns the actual subscription cancellation.
	var stripeID sql.NullString
	_ = g.db.QueryRowContext(ctx,
		`SELECT stripe_customer_id FROM np_saas_tenants WHERE tenant_id = $1`,
		tenantID).Scan(&stripeID)

	deleted, failed, err := g.purgeTenant(ctx, tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "account purge failed")
		return
	}
	if len(failed) > 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":   map[string]string{"code": "db_error", "message": "some data could not be purged — retry"},
			"deleted": deleted,
			"failed":  failed,
		})
		return
	}

	resp := map[string]any{
		"deleted":   true,
		"tenant_id": tenantID,
		"tables":    deleted,
	}
	if stripeID.String != "" {
		resp["stripe_customer_id"] = stripeID.String
		resp["billing_note"] = "subscription cancellation is processed by the billing service (ping_api) for this customer id"
	}
	writeJSON(w, http.StatusOK, resp)
}
