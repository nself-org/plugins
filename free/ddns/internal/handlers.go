package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

// sa returns the source_account_id from X-Source-Account-ID header, defaulting to "primary".
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

// getExternalIP attempts to detect the public IP via several services.
func getExternalIP(ctx context.Context) (string, error) {
	services := []string{
		"https://api.ipify.org?format=json",
		"https://api.my-ip.io/v2/ip.json",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range services {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal(body, &data); err != nil {
			continue
		}
		for _, key := range []string{"ip", "origin"} {
			if v, ok := data[key].(string); ok && v != "" {
				return strings.TrimSpace(v), nil
			}
		}
	}

	return "", fmt.Errorf("unable to detect public IP from any service")
}

// updateCFRecord calls the Cloudflare API to update a DNS A record.
func updateCFRecord(ctx context.Context, apiKey, zoneID, recordID, ip string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", zoneID, recordID)
	payload := fmt.Sprintf(`{"type":"A","content":%q,"ttl":1}`, ip)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, strings.NewReader(payload))
	if err != nil {
		return fmt.Errorf("cloudflare request build failed: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("cloudflare returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

// ─── Health ───────────────────────────────────────────────────────────────────

// HandleHealth returns 200 ok.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "ddns",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// HandleReady checks database connectivity.
func HandleReady(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if _, err := db.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"ready":     false,
				"plugin":    "ddns",
				"error":     "Database unavailable",
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ready":     true,
			"plugin":    "ddns",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// ─── IP ───────────────────────────────────────────────────────────────────────

// HandleGetIP returns the current detected public IP.
func HandleGetIP(w http.ResponseWriter, r *http.Request) {
	ip, err := getExternalIP(r.Context())
	if err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"ip":        ip,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ─── Configs ──────────────────────────────────────────────────────────────────

// HandleListConfigs lists DDNS configurations for the scoped account.
func HandleListConfigs(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := sa(r)
		q := r.URL.Query()

		args := []any{account}
		conds := []string{"source_account_id = $1"}
		idx := 2

		if p := q.Get("provider"); p != "" {
			conds = append(conds, fmt.Sprintf("provider = $%d", idx))
			args = append(args, p)
			idx++
		}
		if e := q.Get("enabled"); e != "" {
			conds = append(conds, fmt.Sprintf("enabled = $%d", idx))
			args = append(args, e == "true")
			idx++
		}

		sql := fmt.Sprintf(
			"SELECT id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at FROM np_ddns_config WHERE %s ORDER BY domain ASC",
			strings.Join(conds, " AND "),
		)

		if lim := q.Get("limit"); lim != "" {
			if n, err := strconv.Atoi(lim); err == nil {
				sql += fmt.Sprintf(" LIMIT $%d", idx)
				args = append(args, n)
				idx++
			}
		} else {
			sql += fmt.Sprintf(" LIMIT $%d", idx)
			args = append(args, 200)
			idx++
		}

		if off := q.Get("offset"); off != "" {
			if n, err := strconv.Atoi(off); err == nil {
				sql += fmt.Sprintf(" OFFSET $%d", idx)
				args = append(args, n)
			}
		}

		rows, err := db.Pool.Query(r.Context(), sql, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		configs := []DDNSConfig{}
		for rows.Next() {
			var c DDNSConfig
			if err := rows.Scan(
				&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
				&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
				&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
				&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			configs = append(configs, c)
		}

		writeJSON(w, http.StatusOK, map[string]any{"configs": configs, "count": len(configs)})
	}
}

// HandleCreateConfig creates a new DDNS configuration.
func HandleCreateConfig(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := sa(r)

		var body struct {
			Provider      string  `json:"provider"`
			Domain        string  `json:"domain"`
			Hostname      string  `json:"hostname"`
			Token         string  `json:"token"`
			APIKey        *string `json:"api_key"`
			ZoneID        *string `json:"zone_id"`
			RecordID      *string `json:"record_id"`
			RecordType    string  `json:"record_type"`
			TTL           int     `json:"ttl"`
			CheckInterval int     `json:"check_interval"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		if body.Provider == "" || body.Domain == "" || body.Token == "" {
			errJSON(w, http.StatusBadRequest, "provider, domain, and token are required")
			return
		}
		if body.Hostname == "" {
			body.Hostname = "@"
		}
		if body.RecordType == "" {
			body.RecordType = "A"
		}
		if body.TTL == 0 {
			body.TTL = 300
		}
		if body.CheckInterval == 0 {
			body.CheckInterval = 300
		}

		var c DDNSConfig
		err := db.Pool.QueryRow(r.Context(), `
			INSERT INTO np_ddns_config
				(source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, check_interval, enabled)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,true)
			RETURNING id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at`,
			account, body.Provider, body.Domain, body.Hostname, body.Token,
			body.APIKey, body.ZoneID, body.RecordID, body.RecordType, body.TTL, body.CheckInterval,
		).Scan(
			&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
			&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
			&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusCreated, c)
	}
}

// HandleGetConfig returns a single DDNS configuration by ID.
func HandleGetConfig(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		account := sa(r)

		var c DDNSConfig
		err := db.Pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at
			FROM np_ddns_config WHERE id = $1 AND source_account_id = $2`,
			id, account,
		).Scan(
			&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
			&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
			&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "config not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, c)
	}
}

// HandleUpdateConfig partially updates a DDNS configuration.
func HandleUpdateConfig(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		account := sa(r)

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			errJSON(w, http.StatusBadRequest, "invalid JSON body")
			return
		}

		allowed := map[string]bool{
			"provider": true, "domain": true, "hostname": true, "token": true,
			"api_key": true, "zone_id": true, "record_id": true, "record_type": true,
			"ttl": true, "check_interval": true, "enabled": true,
		}

		sets := []string{}
		args := []any{}
		idx := 1
		for k, v := range body {
			if !allowed[k] {
				continue
			}
			sets = append(sets, fmt.Sprintf("%s = $%d", k, idx))
			args = append(args, v)
			idx++
		}
		if len(sets) == 0 {
			errJSON(w, http.StatusBadRequest, "no updatable fields provided")
			return
		}
		sets = append(sets, "updated_at = NOW()")
		args = append(args, id, account)

		sql := fmt.Sprintf(`
			UPDATE np_ddns_config SET %s
			WHERE id = $%d AND source_account_id = $%d
			RETURNING id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at`,
			strings.Join(sets, ", "), idx, idx+1,
		)

		var c DDNSConfig
		err := db.Pool.QueryRow(r.Context(), sql, args...).Scan(
			&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
			&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
			&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "config not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, c)
	}
}

// HandleDeleteConfig deletes a DDNS configuration.
func HandleDeleteConfig(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		account := sa(r)

		ct, err := db.Pool.Exec(r.Context(),
			"DELETE FROM np_ddns_config WHERE id = $1 AND source_account_id = $2",
			id, account,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ct.RowsAffected() == 0 {
			errJSON(w, http.StatusNotFound, "config not found")
			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"success": true})
	}
}

// HandleManualUpdate triggers a manual IP update for a single config.
func HandleManualUpdate(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		account := sa(r)

		var c DDNSConfig
		err := db.Pool.QueryRow(r.Context(), `
			SELECT id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at
			FROM np_ddns_config WHERE id = $1 AND source_account_id = $2`,
			id, account,
		).Scan(
			&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
			&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
			&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
			&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			errJSON(w, http.StatusNotFound, "config not found")
			return
		}
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		result, logEntry := performUpdate(r.Context(), db, account, c)
		insertUpdateLog(r.Context(), db, logEntry)

		writeJSON(w, http.StatusOK, result)
	}
}

// HandleAutoUpdate detects the current IP and updates all enabled configs.
func HandleAutoUpdate(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := sa(r)

		ip, err := getExternalIP(r.Context())
		if err != nil {
			errJSON(w, http.StatusBadGateway, err.Error())
			return
		}

		rows, err := db.Pool.Query(r.Context(), `
			SELECT id, source_account_id, provider, domain, hostname, token, api_key, zone_id, record_id, record_type, ttl, current_ip, last_updated, check_interval, enabled, created_at, updated_at
			FROM np_ddns_config WHERE source_account_id = $1 AND enabled = true ORDER BY domain ASC`,
			account,
		)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		var configs []DDNSConfig
		for rows.Next() {
			var c DDNSConfig
			if err := rows.Scan(
				&c.ID, &c.SourceAccountID, &c.Provider, &c.Domain, &c.Hostname,
				&c.Token, &c.APIKey, &c.ZoneID, &c.RecordID, &c.RecordType,
				&c.TTL, &c.CurrentIP, &c.LastUpdated, &c.CheckInterval,
				&c.Enabled, &c.CreatedAt, &c.UpdatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			configs = append(configs, c)
		}

		results := []UpdateResult{}
		for _, c := range configs {
			result, logEntry := performUpdateWithIP(r.Context(), db, account, c, ip)
			insertUpdateLog(r.Context(), db, logEntry)
			results = append(results, result)
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"results":     results,
			"count":       len(results),
			"external_ip": ip,
			"timestamp":   time.Now().UTC().Format(time.RFC3339),
		})
	}
}

// performUpdate detects current IP and updates the given config.
func performUpdate(ctx context.Context, db *DB, account string, c DDNSConfig) (UpdateResult, UpdateLog) {
	ip, err := getExternalIP(ctx)
	if err != nil {
		msg := err.Error()
		return UpdateResult{
				ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
				OldIP: strOrEmpty(c.CurrentIP), NewIP: "", Status: "failed", Message: msg,
			}, UpdateLog{
				SourceAccountID: account, ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
				OldIP: c.CurrentIP, NewIP: "", Status: "failed", Error: &msg,
			}
	}
	return performUpdateWithIP(ctx, db, account, c, ip)
}

// performUpdateWithIP updates a config with a known IP.
func performUpdateWithIP(ctx context.Context, db *DB, account string, c DDNSConfig, ip string) (UpdateResult, UpdateLog) {
	start := time.Now()
	oldIP := strOrEmpty(c.CurrentIP)

	// IP unchanged — skip
	if c.CurrentIP != nil && *c.CurrentIP == ip {
		_, _ = db.Pool.Exec(ctx,
			"UPDATE np_ddns_config SET last_updated = NOW(), updated_at = NOW() WHERE id = $1",
			c.ID,
		)
		msg := "IP unchanged"
		return UpdateResult{
				ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
				OldIP: oldIP, NewIP: ip, Status: "skipped", Message: msg,
			}, UpdateLog{
				SourceAccountID: account, ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
				OldIP: c.CurrentIP, NewIP: ip, Status: "skipped",
				ResponseMessage: &msg, DurationMS: int(time.Since(start).Milliseconds()),
			}
	}

	// Attempt DNS update via Cloudflare if credentials present
	if c.APIKey != nil && c.ZoneID != nil && c.RecordID != nil {
		if err := updateCFRecord(ctx, *c.APIKey, *c.ZoneID, *c.RecordID, ip); err != nil {
			msg := err.Error()
			dur := int(time.Since(start).Milliseconds())
			return UpdateResult{
					ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
					OldIP: oldIP, NewIP: ip, Status: "failed", Message: msg,
				}, UpdateLog{
					SourceAccountID: account, ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
					OldIP: c.CurrentIP, NewIP: ip, Status: "failed", Error: &msg, DurationMS: dur,
				}
		}
	}

	// Update DB record
	_, _ = db.Pool.Exec(ctx,
		"UPDATE np_ddns_config SET current_ip = $1, last_updated = NOW(), updated_at = NOW() WHERE id = $2",
		ip, c.ID,
	)

	code := 200
	msg := "DNS record updated"
	dur := int(time.Since(start).Milliseconds())
	return UpdateResult{
			ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
			OldIP: oldIP, NewIP: ip, Status: "success", Message: msg,
		}, UpdateLog{
			SourceAccountID: account, ConfigID: c.ID, Provider: c.Provider, Domain: c.Domain,
			OldIP: c.CurrentIP, NewIP: ip, Status: "success",
			ResponseCode: &code, ResponseMessage: &msg, DurationMS: dur,
		}
}

// insertUpdateLog persists an UpdateLog row.
func insertUpdateLog(ctx context.Context, db *DB, l UpdateLog) {
	_, _ = db.Pool.Exec(ctx, `
		INSERT INTO np_ddns_update_log
			(source_account_id, config_id, provider, domain, old_ip, new_ip, status, response_code, response_message, error, duration_ms)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		l.SourceAccountID, l.ConfigID, l.Provider, l.Domain,
		l.OldIP, l.NewIP, l.Status, l.ResponseCode, l.ResponseMessage, l.Error, l.DurationMS,
	)
}

func strOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ─── Update Log ───────────────────────────────────────────────────────────────

// HandleListUpdateLog lists update log entries.
func HandleListUpdateLog(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := sa(r)
		q := r.URL.Query()

		args := []any{account}
		conds := []string{"source_account_id = $1"}
		idx := 2

		if s := q.Get("status"); s != "" {
			conds = append(conds, fmt.Sprintf("status = $%d", idx))
			args = append(args, s)
			idx++
		}

		sql := fmt.Sprintf(
			"SELECT id, source_account_id, config_id, provider, domain, old_ip, new_ip, status, response_code, response_message, error, duration_ms, created_at FROM np_ddns_update_log WHERE %s ORDER BY created_at DESC",
			strings.Join(conds, " AND "),
		)

		if lim := q.Get("limit"); lim != "" {
			if n, err := strconv.Atoi(lim); err == nil {
				sql += fmt.Sprintf(" LIMIT $%d", idx)
				args = append(args, n)
				idx++
			}
		} else {
			sql += fmt.Sprintf(" LIMIT $%d", idx)
			args = append(args, 50)
			idx++
		}

		if off := q.Get("offset"); off != "" {
			if n, err := strconv.Atoi(off); err == nil {
				sql += fmt.Sprintf(" OFFSET $%d", idx)
				args = append(args, n)
			}
		}

		rows, err := db.Pool.Query(r.Context(), sql, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		logs := []UpdateLog{}
		for rows.Next() {
			var l UpdateLog
			if err := rows.Scan(
				&l.ID, &l.SourceAccountID, &l.ConfigID, &l.Provider, &l.Domain,
				&l.OldIP, &l.NewIP, &l.Status, &l.ResponseCode, &l.ResponseMessage,
				&l.Error, &l.DurationMS, &l.CreatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			logs = append(logs, l)
		}

		writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "count": len(logs)})
	}
}

// HandleListUpdateLogForConfig lists update log entries for a specific config.
func HandleListUpdateLogForConfig(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		configID := chi.URLParam(r, "config_id")
		account := sa(r)
		q := r.URL.Query()

		args := []any{account, configID}
		sql := "SELECT id, source_account_id, config_id, provider, domain, old_ip, new_ip, status, response_code, response_message, error, duration_ms, created_at FROM np_ddns_update_log WHERE source_account_id = $1 AND config_id = $2 ORDER BY created_at DESC"

		idx := 3
		if lim := q.Get("limit"); lim != "" {
			if n, err := strconv.Atoi(lim); err == nil {
				sql += fmt.Sprintf(" LIMIT $%d", idx)
				args = append(args, n)
				idx++
			}
		} else {
			sql += fmt.Sprintf(" LIMIT $%d", idx)
			args = append(args, 50)
			idx++
		}

		if off := q.Get("offset"); off != "" {
			if n, err := strconv.Atoi(off); err == nil {
				sql += fmt.Sprintf(" OFFSET $%d", idx)
				args = append(args, n)
			}
		}

		rows, err := db.Pool.Query(r.Context(), sql, args...)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer rows.Close()

		logs := []UpdateLog{}
		for rows.Next() {
			var l UpdateLog
			if err := rows.Scan(
				&l.ID, &l.SourceAccountID, &l.ConfigID, &l.Provider, &l.Domain,
				&l.OldIP, &l.NewIP, &l.Status, &l.ResponseCode, &l.ResponseMessage,
				&l.Error, &l.DurationMS, &l.CreatedAt,
			); err != nil {
				errJSON(w, http.StatusInternalServerError, err.Error())
				return
			}
			logs = append(logs, l)
		}

		writeJSON(w, http.StatusOK, map[string]any{"logs": logs, "count": len(logs)})
	}
}

// ─── Stats ────────────────────────────────────────────────────────────────────

// HandleStats returns aggregate DDNS statistics.
func HandleStats(db *DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		account := sa(r)

		var (
			totalConfigs   int
			enabledConfigs int
			totalUpdates   int
			successUpdates int
			failedUpdates  int
			skippedUpdates int
			lastUpdated    *time.Time
		)

		err := db.Pool.QueryRow(r.Context(), `
			SELECT
				(SELECT COUNT(*) FROM np_ddns_config WHERE source_account_id = $1) AS total_configs,
				(SELECT COUNT(*) FROM np_ddns_config WHERE source_account_id = $1 AND enabled = true) AS enabled_configs,
				(SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1) AS total_updates,
				(SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'success') AS successful_updates,
				(SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'failed') AS failed_updates,
				(SELECT COUNT(*) FROM np_ddns_update_log WHERE source_account_id = $1 AND status = 'skipped') AS skipped_updates,
				(SELECT MAX(last_updated) FROM np_ddns_config WHERE source_account_id = $1) AS last_updated`,
			account,
		).Scan(&totalConfigs, &enabledConfigs, &totalUpdates, &successUpdates, &failedUpdates, &skippedUpdates, &lastUpdated)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"plugin":  "ddns",
			"version": "1.0.0",
			"stats": map[string]any{
				"total_configs":      totalConfigs,
				"enabled_configs":    enabledConfigs,
				"total_updates":      totalUpdates,
				"successful_updates": successUpdates,
				"failed_updates":     failedUpdates,
				"skipped_updates":    skippedUpdates,
				"last_updated":       lastUpdated,
			},
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	}
}
