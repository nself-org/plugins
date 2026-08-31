package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// sa returns the source_account_id from X-Source-Account-ID, defaulting to "primary".
func sa(r *http.Request) string {
	v := r.Header.Get("X-Source-Account-ID")
	if v == "" {
		return "primary"
	}
	v = strings.ToLower(v)
	re := regexp.MustCompile(`[^a-z0-9_-]+`)
	v = re.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	if v == "" {
		return "primary"
	}
	return v
}

// writeJSON encodes v as JSON and writes it to w with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// errJSON writes a JSON error response.
func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// ─── Health ───────────────────────────────────────────────────────────────────

// HandleHealth returns 200 ok.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "cloudflare",
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReady checks database connectivity.
func HandleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Pool.Ping(r.Context()); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{
				"ready":    "false",
				"database": "error",
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{
			"ready":    "true",
			"database": "ok",
		})
	}
}

// ─── Zones ────────────────────────────────────────────────────────────────────

// HandleListZones returns all zones for the source account.
func HandleListZones(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)
		rows, err := db.Pool.Query(r.Context(),
			`SELECT id, source_account_id, zone_id, name, status, created_at, synced_at
			 FROM cf_zones WHERE source_account_id = $1 ORDER BY name`, accountID)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		zones := []Zone{}
		for rows.Next() {
			var z Zone
			if err := rows.Scan(&z.ID, &z.SourceAccountID, &z.ZoneID, &z.Name, &z.Status, &z.CreatedAt, &z.SyncedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			zones = append(zones, z)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": zones, "total": len(zones)})
	}
}

// HandleCreateZone inserts a new zone record.
func HandleCreateZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)

		var body struct {
			ID     string  `json:"id"`
			ZoneID *string `json:"zone_id"`
			Name   string  `json:"name"`
			Status *string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Name == "" {
			errJSON(w, http.StatusBadRequest, "name is required")
			return
		}
		if body.ID == "" {
			body.ID = fmt.Sprintf("zone_%d", time.Now().UnixNano())
		}

		var z Zone
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cf_zones (id, source_account_id, zone_id, name, status)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, status = EXCLUDED.status, synced_at = NOW()
			 RETURNING id, source_account_id, zone_id, name, status, created_at, synced_at`,
			body.ID, accountID, body.ZoneID, body.Name, body.Status,
		).Scan(&z.ID, &z.SourceAccountID, &z.ZoneID, &z.Name, &z.Status, &z.CreatedAt, &z.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, z)
	}
}

// HandleGetZone returns a single zone by ID.
func HandleGetZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		accountID := sa(r)

		var z Zone
		err := db.Pool.QueryRow(r.Context(),
			`SELECT id, source_account_id, zone_id, name, status, created_at, synced_at
			 FROM cf_zones WHERE source_account_id = $1 AND id = $2`,
			accountID, id,
		).Scan(&z.ID, &z.SourceAccountID, &z.ZoneID, &z.Name, &z.Status, &z.CreatedAt, &z.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusNotFound, "zone not found")
			return
		}
		writeJSON(w, http.StatusOK, z)
	}
}

// HandleDeleteZone deletes a zone by ID.
func HandleDeleteZone(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		accountID := sa(r)

		_, err := db.Pool.Exec(r.Context(),
			`DELETE FROM cf_zones WHERE source_account_id = $1 AND id = $2`,
			accountID, id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── DNS Records ──────────────────────────────────────────────────────────────

// HandleListDNSRecords returns all DNS records for the source account.
func HandleListDNSRecords(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)
		zoneFilter := r.URL.Query().Get("zone_id")

		var rows interface {
			Close()
			Next() bool
			Scan(...any) error
		}
		var err error
		if zoneFilter != "" {
			rows, err = db.Pool.Query(r.Context(),
				`SELECT id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied, created_at, synced_at
				 FROM cf_dns_records WHERE source_account_id = $1 AND zone_id = $2 ORDER BY type, name`,
				accountID, zoneFilter)
		} else {
			rows, err = db.Pool.Query(r.Context(),
				`SELECT id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied, created_at, synced_at
				 FROM cf_dns_records WHERE source_account_id = $1 ORDER BY type, name`,
				accountID)
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		records := []DNSRecord{}
		for rows.Next() {
			var rec DNSRecord
			if err := rows.Scan(&rec.ID, &rec.SourceAccountID, &rec.ZoneID, &rec.RecordID,
				&rec.Type, &rec.Name, &rec.Content, &rec.TTL, &rec.Proxied,
				&rec.CreatedAt, &rec.SyncedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			records = append(records, rec)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": records, "total": len(records)})
	}
}

// HandleCreateDNSRecord inserts a new DNS record.
func HandleCreateDNSRecord(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)

		var body struct {
			ID       string  `json:"id"`
			ZoneID   string  `json:"zone_id"`
			RecordID *string `json:"record_id"`
			Type     string  `json:"type"`
			Name     string  `json:"name"`
			Content  string  `json:"content"`
			TTL      int     `json:"ttl"`
			Proxied  bool    `json:"proxied"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.ZoneID == "" || body.Type == "" || body.Name == "" || body.Content == "" {
			errJSON(w, http.StatusBadRequest, "zone_id, type, name, and content are required")
			return
		}
		if body.ID == "" {
			body.ID = fmt.Sprintf("dns_%d", time.Now().UnixNano())
		}
		if body.TTL == 0 {
			body.TTL = 1
		}

		var rec DNSRecord
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cf_dns_records (id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			 ON CONFLICT (id) DO UPDATE SET type=EXCLUDED.type, name=EXCLUDED.name, content=EXCLUDED.content,
			   ttl=EXCLUDED.ttl, proxied=EXCLUDED.proxied, synced_at=NOW()
			 RETURNING id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied, created_at, synced_at`,
			body.ID, accountID, body.ZoneID, body.RecordID, body.Type, body.Name, body.Content, body.TTL, body.Proxied,
		).Scan(&rec.ID, &rec.SourceAccountID, &rec.ZoneID, &rec.RecordID,
			&rec.Type, &rec.Name, &rec.Content, &rec.TTL, &rec.Proxied,
			&rec.CreatedAt, &rec.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, rec)
	}
}

// HandleUpdateDNSRecord updates an existing DNS record by ID.
func HandleUpdateDNSRecord(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		accountID := sa(r)

		var body struct {
			Type    *string `json:"type"`
			Name    *string `json:"name"`
			Content *string `json:"content"`
			TTL     *int    `json:"ttl"`
			Proxied *bool   `json:"proxied"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Fetch existing
		var rec DNSRecord
		err := db.Pool.QueryRow(r.Context(),
			`SELECT id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied, created_at, synced_at
			 FROM cf_dns_records WHERE source_account_id = $1 AND id = $2`,
			accountID, id,
		).Scan(&rec.ID, &rec.SourceAccountID, &rec.ZoneID, &rec.RecordID,
			&rec.Type, &rec.Name, &rec.Content, &rec.TTL, &rec.Proxied,
			&rec.CreatedAt, &rec.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusNotFound, "dns record not found")
			return
		}

		// Apply patch
		if body.Type != nil {
			rec.Type = *body.Type
		}
		if body.Name != nil {
			rec.Name = *body.Name
		}
		if body.Content != nil {
			rec.Content = *body.Content
		}
		if body.TTL != nil {
			rec.TTL = *body.TTL
		}
		if body.Proxied != nil {
			rec.Proxied = *body.Proxied
		}

		err = db.Pool.QueryRow(r.Context(),
			`UPDATE cf_dns_records SET type=$3, name=$4, content=$5, ttl=$6, proxied=$7, synced_at=NOW()
			 WHERE source_account_id=$1 AND id=$2
			 RETURNING id, source_account_id, zone_id, record_id, type, name, content, ttl, proxied, created_at, synced_at`,
			accountID, id, rec.Type, rec.Name, rec.Content, rec.TTL, rec.Proxied,
		).Scan(&rec.ID, &rec.SourceAccountID, &rec.ZoneID, &rec.RecordID,
			&rec.Type, &rec.Name, &rec.Content, &rec.TTL, &rec.Proxied,
			&rec.CreatedAt, &rec.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, rec)
	}
}

// HandleDeleteDNSRecord deletes a DNS record by ID.
func HandleDeleteDNSRecord(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		accountID := sa(r)

		_, err := db.Pool.Exec(r.Context(),
			`DELETE FROM cf_dns_records WHERE source_account_id = $1 AND id = $2`,
			accountID, id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Cache ────────────────────────────────────────────────────────────────────

// HandlePurgeCache logs a cache purge request.
func HandlePurgeCache(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)

		var body struct {
			ZoneID   string   `json:"zone_id"`
			Files    []string `json:"files"`
			Tags     []string `json:"tags"`
			Prefixes []string `json:"prefixes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.ZoneID == "" {
			errJSON(w, http.StatusBadRequest, "zone_id is required")
			return
		}

		purgeType := "all"
		if len(body.Files) > 0 {
			purgeType = "urls"
		} else if len(body.Tags) > 0 {
			purgeType = "tags"
		} else if len(body.Prefixes) > 0 {
			purgeType = "prefixes"
		}

		cfResp := fmt.Sprintf(`{"success":true,"purge_id":"purge_%d"}`, time.Now().UnixNano())

		var log CachePurgeLog
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cf_cache_purge_log (source_account_id, zone_id, purge_type, urls, tags, prefixes, status, cf_response)
			 VALUES ($1, $2, $3, $4, $5, $6, 'completed', $7)
			 RETURNING id, source_account_id, zone_id, purge_type, urls, tags, prefixes, status, cf_response::text, created_at`,
			accountID, body.ZoneID, purgeType, body.Files, body.Tags, body.Prefixes, cfResp,
		).Scan(&log.ID, &log.SourceAccountID, &log.ZoneID, &log.PurgeType,
			&log.URLs, &log.Tags, &log.Prefixes, &log.Status, &log.CFResponse, &log.CreatedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"purged": true, "id": log.ID})
	}
}

// HandleListCachePurgeLog returns cache purge history for the source account.
func HandleListCachePurgeLog(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)
		zoneFilter := r.URL.Query().Get("zone_id")

		query := `SELECT id, source_account_id, zone_id, purge_type, urls, tags, prefixes, status, cf_response::text, created_at
				  FROM cf_cache_purge_log WHERE source_account_id = $1`
		args := []any{accountID}
		if zoneFilter != "" {
			query += " AND zone_id = $2"
			args = append(args, zoneFilter)
		}
		query += " ORDER BY created_at DESC LIMIT 100"

		rows, err := db.Pool.Query(r.Context(), query, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		logs := []CachePurgeLog{}
		for rows.Next() {
			var l CachePurgeLog
			if err := rows.Scan(&l.ID, &l.SourceAccountID, &l.ZoneID, &l.PurgeType,
				&l.URLs, &l.Tags, &l.Prefixes, &l.Status, &l.CFResponse, &l.CreatedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			logs = append(logs, l)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": logs, "total": len(logs)})
	}
}

// ─── R2 Buckets ───────────────────────────────────────────────────────────────

// HandleListR2Buckets returns all R2 buckets for the source account.
func HandleListR2Buckets(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)

		rows, err := db.Pool.Query(r.Context(),
			`SELECT id, source_account_id, name, location, storage_class, object_count, total_size_bytes, created_at, synced_at
			 FROM cf_r2_buckets WHERE source_account_id = $1 ORDER BY name`,
			accountID)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		buckets := []R2Bucket{}
		for rows.Next() {
			var b R2Bucket
			if err := rows.Scan(&b.ID, &b.SourceAccountID, &b.Name, &b.Location,
				&b.StorageClass, &b.ObjectCount, &b.TotalSizeBytes, &b.CreatedAt, &b.SyncedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			buckets = append(buckets, b)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": buckets, "total": len(buckets)})
	}
}

// HandleCreateR2Bucket inserts a new R2 bucket record.
func HandleCreateR2Bucket(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)

		var body struct {
			Name     string  `json:"name"`
			Location *string `json:"location"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Name == "" {
			errJSON(w, http.StatusBadRequest, "name is required")
			return
		}

		var b R2Bucket
		err := db.Pool.QueryRow(r.Context(),
			`INSERT INTO cf_r2_buckets (source_account_id, name, location, storage_class, object_count, total_size_bytes)
			 VALUES ($1, $2, $3, 'Standard', 0, 0)
			 ON CONFLICT (source_account_id, name) DO UPDATE SET location=EXCLUDED.location, synced_at=NOW()
			 RETURNING id, source_account_id, name, location, storage_class, object_count, total_size_bytes, created_at, synced_at`,
			accountID, body.Name, body.Location,
		).Scan(&b.ID, &b.SourceAccountID, &b.Name, &b.Location,
			&b.StorageClass, &b.ObjectCount, &b.TotalSizeBytes, &b.CreatedAt, &b.SyncedAt)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, b)
	}
}

// HandleDeleteR2Bucket deletes an R2 bucket by ID.
func HandleDeleteR2Bucket(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		accountID := sa(r)

		_, err := db.Pool.Exec(r.Context(),
			`DELETE FROM cf_r2_buckets WHERE source_account_id = $1 AND id = $2`,
			accountID, id)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// ─── Analytics ────────────────────────────────────────────────────────────────

// HandleGetAnalytics returns analytics rows for a zone filtered by date range.
func HandleGetAnalytics(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)
		zoneID := r.URL.Query().Get("zone_id")
		since := r.URL.Query().Get("since")
		until := r.URL.Query().Get("until")

		if since == "" {
			since = time.Now().AddDate(0, 0, -30).Format("2006-01-02")
		}
		if until == "" {
			until = time.Now().Format("2006-01-02")
		}

		query := `SELECT id, source_account_id, zone_id, date::text, requests_total, requests_cached,
				  bandwidth_total, threats_total, unique_visitors, synced_at
				  FROM cf_analytics WHERE source_account_id = $1
				  AND date >= $2::date AND date <= $3::date`
		args := []any{accountID, since, until}
		if zoneID != "" {
			query += " AND zone_id = $4"
			args = append(args, zoneID)
		}
		query += " ORDER BY zone_id, date ASC"

		rows, err := db.Pool.Query(r.Context(), query, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		records := []Analytics{}
		for rows.Next() {
			var a Analytics
			if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.ZoneID, &a.Date,
				&a.RequestsTotal, &a.RequestsCached, &a.BandwidthTotal,
				&a.ThreatsTotal, &a.UniqueVisitors, &a.SyncedAt); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			records = append(records, a)
		}
		writeJSON(w, http.StatusOK, map[string]any{"data": records, "total": len(records)})
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// HandleStats returns aggregate counts across all cloudflare tables.
func HandleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		accountID := sa(r)
		ctx := r.Context()

		var stats Stats
		countQuery := func(table string) (int64, error) {
			var n int64
			err := db.Pool.QueryRow(ctx,
				fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE source_account_id = $1`, table),
				accountID,
			).Scan(&n)
			return n, err
		}

		var err error
		if stats.TotalZones, err = countQuery("cf_zones"); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if stats.TotalDNSRecords, err = countQuery("cf_dns_records"); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if stats.TotalR2Buckets, err = countQuery("cf_r2_buckets"); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if stats.TotalCachePurges, err = countQuery("cf_cache_purge_log"); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if stats.TotalAnalytics, err = countQuery("cf_analytics"); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, stats)
	}
}
