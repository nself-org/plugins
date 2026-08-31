package main

// handlers_internal.go — /internal/tenants/{id}: full tenant purge.
//
// Purpose: operational cleanup of throwaway/e2e tenants. Deletes every row
//   carrying the tenant's id across ALL np_* tables that have a tenant_id
//   column (discovered via information_schema, so new plugin tables are
//   covered automatically), then the np_saas_tenants row itself.
// Auth: NSENTRY_INTERNAL_API_KEY (constant-time compare) — and nginx never
//   routes /internal/* externally, so this is reachable only on-box
//   (127.0.0.1:3848) or over the docker network.
// Outputs: {"tenant_id","deleted":{table:rows}} · 401/503 on auth failure.
// Constraints: multi-pass deletes tolerate FK ordering between plugin
//   tables; individual table failures are reported, never silently dropped.

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// internalAuthOK verifies the shared internal key. Accepts the key as a
// Bearer token, X-Internal-Key, or X-NSentry-Internal-Key (the header the
// ping_api TenantTierClient sends). Constant-time compare; returns false
// after writing the error response.
func (g *gateway) internalAuthOK(w http.ResponseWriter, r *http.Request) bool {
	if g.cfg.InternalAPIKey == "" {
		writeErr(w, http.StatusServiceUnavailable, "internal_disabled",
			"NSENTRY_INTERNAL_API_KEY is not configured")
		return false
	}
	presented := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if presented == "" {
		presented = r.Header.Get("X-Internal-Key")
	}
	if presented == "" {
		presented = r.Header.Get("X-NSentry-Internal-Key")
	}
	if subtle.ConstantTimeCompare([]byte(presented), []byte(g.cfg.InternalAPIKey)) != 1 {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid internal key")
		return false
	}
	return true
}

// internalIPAllowed enforces the optional source-IP allowlist for internal
// endpoints reachable from OUTSIDE the box (currently only the billing
// tenant-tier hook, re-exposed via nginx so the prod ping_api box can reach
// it). When the allowlist is empty the check is skipped (loopback-only
// endpoints rely on nginx returning 404 externally). RemoteAddr is the
// RealIP-resolved client (chi middleware.RealIP reads nginx's X-Forwarded-For,
// which is set to the true peer $remote_addr). Returns false after writing 403.
func (g *gateway) internalIPAllowed(w http.ResponseWriter, r *http.Request) bool {
	if len(g.cfg.InternalAllowIPs) == 0 {
		return true
	}
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if _, ok := g.cfg.InternalAllowIPs[ip]; ok {
		return true
	}
	writeErr(w, http.StatusForbidden, "forbidden", "source address not permitted")
	return false
}

// tenantTables discovers every np_* table carrying a tenant_id column
// (np_saas_tenants excluded — it is handled last/specially by callers).
// New plugin tables are covered automatically.
func (g *gateway) tenantTables(ctx context.Context) ([]string, error) {
	rows, err := g.db.QueryContext(ctx, `
		SELECT table_schema || '.' || table_name
		FROM information_schema.columns
		WHERE column_name = 'tenant_id'
		  AND table_name LIKE 'np\_%'
		  AND table_name <> 'np_saas_tenants'
		ORDER BY table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var tables []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

// purgeTenant deletes every row carrying the tenant's id across all
// discovered np_* tables, then the np_saas_tenants row itself. Multi-pass
// deletes tolerate FK ordering. Returns per-table delete counts and any
// tables that still failed after the passes (the tenants row is only
// removed when nothing failed).
func (g *gateway) purgeTenant(ctx context.Context, tenantID string) (deleted map[string]int64, failed map[string]string, err error) {
	tables, err := g.tenantTables(ctx)
	if err != nil {
		return nil, nil, err
	}

	deleted = map[string]int64{}
	failed = map[string]string{}
	// Up to 3 passes so FK parent/child ordering resolves itself.
	pending := tables
	for pass := 0; pass < 3 && len(pending) > 0; pass++ {
		var next []string
		for _, table := range pending {
			// table names come from information_schema, not user input.
			res, derr := g.db.ExecContext(ctx,
				`DELETE FROM `+table+` WHERE tenant_id = $1`, tenantID) //nolint:gosec
			if derr != nil {
				next = append(next, table)
				failed[table] = derr.Error()
				continue
			}
			delete(failed, table)
			n, _ := res.RowsAffected()
			if n > 0 {
				deleted[table] += n
			}
		}
		pending = next
	}
	if len(failed) > 0 {
		return deleted, failed, nil
	}

	res, err := g.db.ExecContext(ctx,
		`DELETE FROM np_saas_tenants WHERE tenant_id = $1`, tenantID)
	if err != nil {
		return deleted, failed, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		deleted["np_saas_tenants"] = n
	}
	return deleted, failed, nil
}

// handleDeleteTenant — DELETE /internal/tenants/{id}.
func (g *gateway) handleDeleteTenant(w http.ResponseWriter, r *http.Request) {
	if !g.internalAuthOK(w, r) {
		return
	}
	tenantID := chi.URLParam(r, "id")
	if uuid.Validate(tenantID) != nil {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_request", "tenant id must be a UUID")
		return
	}

	deleted, failed, err := g.purgeTenant(r.Context(), tenantID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db_error", "tenant purge failed")
		return
	}
	if len(failed) > 0 {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error":     map[string]string{"code": "db_error", "message": "some tables could not be purged"},
			"tenant_id": tenantID,
			"deleted":   deleted,
			"failed":    failed,
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"tenant_id": tenantID,
		"deleted":   deleted,
	})
}
