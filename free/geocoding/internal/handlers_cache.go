package internal

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func (h *Handlers) ListCache(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, source_account_id, query_type, query_hash, query_text, provider,
		        lat, lng, formatted_address, hit_count, created_at, updated_at, expires_at
		   FROM geo_cache WHERE source_account_id=$1 ORDER BY created_at DESC LIMIT 200`,
		account,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	entries := []GeoCache{}
	for rows.Next() {
		var c GeoCache
		if err := rows.Scan(&c.ID, &c.SourceAccountID, &c.QueryType, &c.QueryHash, &c.QueryText,
			&c.Provider, &c.Lat, &c.Lng, &c.FormattedAddress, &c.HitCount,
			&c.CreatedAt, &c.UpdatedAt, &c.ExpiresAt); err != nil {
			continue
		}
		entries = append(entries, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": entries, "total": len(entries)})
}

func (h *Handlers) DeleteCacheEntry(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	account := sa(r)
	tag, err := h.DB.Pool.Exec(r.Context(),
		`DELETE FROM geo_cache WHERE id=$1 AND source_account_id=$2`, id, account)
	if err != nil || tag.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "cache entry not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ─── Stats ───────────────────────────────────────────────────────────────────

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	ctx := r.Context()

	var stats PluginStats
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_cache WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalCacheEntries)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofences WHERE source_account_id=$1 AND deleted_at IS NULL`, account,
	).Scan(&stats.TotalGeofences)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofences WHERE source_account_id=$1 AND active=TRUE AND deleted_at IS NULL`, account,
	).Scan(&stats.ActiveGeofences)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_geofence_events WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalGeofenceEvents)
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM geo_places WHERE source_account_id=$1`, account,
	).Scan(&stats.TotalPlaces)

	var totalHits, totalEntries int
	_ = h.DB.Pool.QueryRow(ctx,
		`SELECT COALESCE(SUM(hit_count),0), COUNT(*) FROM geo_cache WHERE source_account_id=$1`, account,
	).Scan(&totalHits, &totalEntries)
	if totalEntries > 0 {
		stats.CacheHitRate = float64(totalHits) / float64(totalEntries)
	}

	writeJSON(w, http.StatusOK, stats)
}

// ─── Quota ───────────────────────────────────────────────────────────────────

func (h *Handlers) GetQuota(w http.ResponseWriter, r *http.Request) {
	account := sa(r)
	ctx := r.Context()

	getQuota := func(qtype string) map[string]int {
		var api, geo, hits int
		_ = h.DB.Pool.QueryRow(ctx,
			`SELECT COALESCE(api_calls,0), COALESCE(geocode_calls,0), COALESCE(cache_hits,0)
			   FROM geo_api_quotas
			  WHERE source_account_id=$1 AND quota_type=$2 AND period_start=CURRENT_DATE`,
			account, qtype,
		).Scan(&api, &geo, &hits)
		return map[string]int{"api_calls": api, "geocode_calls": geo, "cache_hits": hits}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"daily":   getQuota("daily"),
		"monthly": getQuota("monthly"),
	})
}
