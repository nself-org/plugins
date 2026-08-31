package internal

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// ---- helpers ----------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func sa(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-Id"); v != "" {
		return v
	}
	return "primary"
}

func appID(r *http.Request) string {
	if v := r.Header.Get("X-App-Id"); v != "" {
		return v
	}
	return sa(r)
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func queryInt(r *http.Request, key string, def int) int {
	v := r.URL.Query().Get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// ---- Handlers ---------------------------------------------------------------

type Handlers struct {
	DB  *DB
	Cfg Config
}

// ---- Health -----------------------------------------------------------------

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"plugin":    "devices",
		"timestamp": time.Now().UTC(),
	})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.DB.Pool.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ready":     false,
			"plugin":    "devices",
			"error":     "Database unavailable",
			"timestamp": time.Now().UTC(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":     true,
		"plugin":    "devices",
		"timestamp": time.Now().UTC(),
	})
}

// ---- Devices ----------------------------------------------------------------

func (h *Handlers) ListDevices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	status := r.URL.Query().Get("status")
	dtype := r.URL.Query().Get("type")
	trust := r.URL.Query().Get("trust_level")

	devices, err := listDevices(ctx, h.DB, acc, status, dtype, trust, limit, offset)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": devices, "limit": limit, "offset": offset})
}

func (h *Handlers) CreateDevice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	app := appID(r)

	var body struct {
		DeviceID        string          `json:"device_id"`
		Name            *string         `json:"name"`
		DeviceType      string          `json:"device_type"`
		Model           *string         `json:"model"`
		FirmwareVersion *string         `json:"firmware_version"`
		Capabilities    json.RawMessage `json:"capabilities"`
		Config          json.RawMessage `json:"config"`
		Labels          json.RawMessage `json:"labels"`
		Metadata        json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.DeviceID == "" || body.DeviceType == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id and device_type are required"})
		return
	}
	caps := nullableJSON(body.Capabilities, "[]")
	cfg := nullableJSON(body.Config, "{}")
	lbl := nullableJSON(body.Labels, "{}")
	meta := nullableJSON(body.Metadata, "{}")

	row := h.DB.Pool.QueryRow(ctx, `
		INSERT INTO np_dev_devices
		  (source_account_id, app_id, device_id, name, device_type, model, firmware_version,
		   capabilities, config, labels, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id, source_account_id, app_id, device_id, name, device_type, model,
		  firmware_version, status, trust_level, last_seen_at, capabilities, config,
		  labels, metadata, created_at, updated_at`,
		acc, app, body.DeviceID, body.Name, body.DeviceType, body.Model, body.FirmwareVersion,
		caps, cfg, lbl, meta,
	)
	dev, err := scanDevice(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = insertAudit(ctx, h.DB, acc, app, "device.registered", &dev.ID, nil, `{"device_type":"`+body.DeviceType+`"}`)
	writeJSON(w, http.StatusCreated, dev)
}

func (h *Handlers) GetDevice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	dev, err := getDeviceByID(ctx, h.DB, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (h *Handlers) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	var body struct {
		Name            *string         `json:"name"`
		FirmwareVersion *string         `json:"firmware_version"`
		Capabilities    json.RawMessage `json:"capabilities"`
		Config          json.RawMessage `json:"config"`
		Labels          json.RawMessage `json:"labels"`
		Metadata        json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	row := h.DB.Pool.QueryRow(ctx, `
		UPDATE np_dev_devices SET
		  name = COALESCE($2, name),
		  firmware_version = COALESCE($3, firmware_version),
		  capabilities = CASE WHEN $4::jsonb IS NOT NULL THEN $4::jsonb ELSE capabilities END,
		  config       = CASE WHEN $5::jsonb IS NOT NULL THEN $5::jsonb ELSE config END,
		  labels       = CASE WHEN $6::jsonb IS NOT NULL THEN $6::jsonb ELSE labels END,
		  metadata     = CASE WHEN $7::jsonb IS NOT NULL THEN $7::jsonb ELSE metadata END,
		  updated_at   = NOW()
		WHERE id = $1
		RETURNING id, source_account_id, app_id, device_id, name, device_type, model,
		  firmware_version, status, trust_level, last_seen_at, capabilities, config,
		  labels, metadata, created_at, updated_at`,
		id, body.Name, body.FirmwareVersion,
		jsonOrNil(body.Capabilities), jsonOrNil(body.Config),
		jsonOrNil(body.Labels), jsonOrNil(body.Metadata),
	)
	dev, err := scanDevice(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, dev)
}

func (h *Handlers) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")
	acc := sa(r)
	app := appID(r)

	dev, err := getDeviceByID(ctx, h.DB, id)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	_ = insertAudit(ctx, h.DB, acc, app, "device.deleted", &id, nil, fmt.Sprintf(`{"device_id":%q}`, dev.DeviceID))
	if _, err := h.DB.Pool.Exec(ctx, `DELETE FROM np_dev_devices WHERE id = $1`, id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Device Commands --------------------------------------------------------

func (h *Handlers) SendDeviceCommand(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deviceID := chi.URLParam(r, "id")
	acc := sa(r)
	app := appID(r)

	var body struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.Type == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "type is required"})
		return
	}

	if _, err := getDeviceByID(ctx, h.DB, deviceID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "Device not found"})
		return
	}

	payload := nullableJSON(body.Payload, "{}")
	timeout := h.Cfg.CommandDefaultTimeout
	idkRaw, _ := randomHex(16)

	row := h.DB.Pool.QueryRow(ctx, `
		INSERT INTO np_dev_commands
		  (source_account_id, app_id, device_id, command_type, payload, timeout_seconds,
		   deadline, max_retries, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6, NOW() + ($6 * INTERVAL '1 second'), $7,$8)
		RETURNING id, source_account_id, app_id, device_id, command_type, command_id,
		  payload, status, priority, dispatched_at, acked_at, started_at, completed_at,
		  result, error, timeout_seconds, deadline, retry_count, max_retries,
		  idempotency_key, metadata, created_at`,
		acc, app, deviceID, body.Type, payload, timeout,
		h.Cfg.CommandMaxRetries, idkRaw,
	)
	cmd, err := scanCommand(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = insertAudit(ctx, h.DB, acc, app, "command.dispatched", &deviceID, nil,
		fmt.Sprintf(`{"command_id":%q,"command_type":%q}`, cmd.ID, body.Type))
	writeJSON(w, http.StatusCreated, map[string]string{"commandId": cmd.ID})
}

func (h *Handlers) ListDeviceCommands(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deviceID := chi.URLParam(r, "id")
	limit := queryInt(r, "limit", 50)

	rows, err := h.DB.Pool.Query(ctx, `
		SELECT id, source_account_id, app_id, device_id, command_type, command_id,
		  payload, status, priority, dispatched_at, acked_at, started_at, completed_at,
		  result, error, timeout_seconds, deadline, retry_count, max_retries,
		  idempotency_key, metadata, created_at
		FROM np_dev_commands WHERE device_id = $1
		ORDER BY dispatched_at DESC LIMIT $2`, deviceID, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	cmds := []Command{}
	for rows.Next() {
		cmd, err := scanCommandRows(rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		cmds = append(cmds, cmd)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": cmds})
}

func (h *Handlers) GetCommand(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	row := h.DB.Pool.QueryRow(ctx, `
		SELECT id, source_account_id, app_id, device_id, command_type, command_id,
		  payload, status, priority, dispatched_at, acked_at, started_at, completed_at,
		  result, error, timeout_seconds, deadline, retry_count, max_retries,
		  idempotency_key, metadata, created_at
		FROM np_dev_commands WHERE id = $1`, id)
	cmd, err := scanCommand(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Command not found"})
		} else {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		}
		return
	}
	writeJSON(w, http.StatusOK, cmd)
}

// ---- Telemetry --------------------------------------------------------------

func (h *Handlers) IngestTelemetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	app := appID(r)

	var body struct {
		DeviceID      string          `json:"device_id"`
		TelemetryType string          `json:"telemetry_type"`
		Metric        string          `json:"metric"`
		Value         *float64        `json:"value"`
		Unit          *string         `json:"unit"`
		Data          json.RawMessage `json:"data"`
		RecordedAt    *time.Time      `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	// Support both telemetry_type and metric/value style
	ttype := body.TelemetryType
	if ttype == "" && body.Metric != "" {
		ttype = body.Metric
	}
	if ttype == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "telemetry_type or metric is required"})
		return
	}
	data := nullableJSON(body.Data, "{}")
	if body.Value != nil {
		data = []byte(fmt.Sprintf(`{"value":%v}`, *body.Value))
	}
	recAt := time.Now().UTC()
	if body.RecordedAt != nil {
		recAt = *body.RecordedAt
	}

	row := h.DB.Pool.QueryRow(ctx, `
		INSERT INTO np_dev_telemetry (source_account_id, app_id, device_id, telemetry_type, data, recorded_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		RETURNING id, source_account_id, app_id, device_id, telemetry_type, data, recorded_at, received_at`,
		acc, app, body.DeviceID, ttype, data, recAt,
	)
	tel, err := scanTelemetry(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, tel)
}

func (h *Handlers) BatchIngestTelemetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	app := appID(r)

	var items []struct {
		DeviceID      string          `json:"device_id"`
		TelemetryType string          `json:"telemetry_type"`
		Data          json.RawMessage `json:"data"`
		RecordedAt    *time.Time      `json:"timestamp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&items); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	count := 0
	for _, item := range items {
		if item.TelemetryType == "" || item.DeviceID == "" {
			continue
		}
		data := nullableJSON(item.Data, "{}")
		recAt := time.Now().UTC()
		if item.RecordedAt != nil {
			recAt = *item.RecordedAt
		}
		_, err := h.DB.Pool.Exec(ctx, `
			INSERT INTO np_dev_telemetry (source_account_id, app_id, device_id, telemetry_type, data, recorded_at)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			acc, app, item.DeviceID, item.TelemetryType, data, recAt,
		)
		if err == nil {
			count++
		}
	}
	writeJSON(w, http.StatusCreated, map[string]int{"inserted": count})
}

func (h *Handlers) QueryTelemetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	deviceID := r.URL.Query().Get("device_id")
	metric := r.URL.Query().Get("metric")
	startTime := r.URL.Query().Get("start_time")
	endTime := r.URL.Query().Get("end_time")
	limit := queryInt(r, "limit", 100)

	q := `SELECT id, source_account_id, app_id, device_id, telemetry_type, data, recorded_at, received_at
	      FROM np_dev_telemetry WHERE source_account_id = $1`
	args := []any{acc}
	idx := 2
	if deviceID != "" {
		q += fmt.Sprintf(" AND device_id = $%d", idx)
		args = append(args, deviceID)
		idx++
	}
	if metric != "" {
		q += fmt.Sprintf(" AND telemetry_type = $%d", idx)
		args = append(args, metric)
		idx++
	}
	if startTime != "" {
		q += fmt.Sprintf(" AND recorded_at >= $%d", idx)
		args = append(args, startTime)
		idx++
	}
	if endTime != "" {
		q += fmt.Sprintf(" AND recorded_at <= $%d", idx)
		args = append(args, endTime)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY recorded_at DESC LIMIT $%d", idx)
	args = append(args, limit)

	rows, err := h.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	result := []Telemetry{}
	for rows.Next() {
		t, err := scanTelemetryRows(rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

func (h *Handlers) GetDeviceTelemetry(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	deviceID := chi.URLParam(r, "id")
	metric := r.URL.Query().Get("metric")
	limit := queryInt(r, "limit", 100)

	q := `SELECT id, source_account_id, app_id, device_id, telemetry_type, data, recorded_at, received_at
	      FROM np_dev_telemetry WHERE device_id = $1`
	args := []any{deviceID}
	if metric != "" {
		q += " AND telemetry_type = $2 ORDER BY recorded_at DESC LIMIT $3"
		args = append(args, metric, limit)
	} else {
		q += " ORDER BY recorded_at DESC LIMIT $2"
		args = append(args, limit)
	}

	rows, err := h.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	result := []Telemetry{}
	for rows.Next() {
		t, err := scanTelemetryRows(rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		result = append(result, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": result})
}

// ---- Ingest Sessions --------------------------------------------------------

func (h *Handlers) ListIngestSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	status := r.URL.Query().Get("status")
	deviceID := r.URL.Query().Get("device_id")

	q := `SELECT id, source_account_id, app_id, device_id, stream_id, status, ingest_url,
		  protocol, channel, quality, bitrate_kbps, fps, resolution, started_at,
		  last_heartbeat_at, ended_at, bytes_ingested, frames_dropped, error_count,
		  last_error, metadata, created_at, updated_at
		  FROM np_dev_ingest_sessions WHERE source_account_id = $1`
	args := []any{acc}
	idx := 2
	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if deviceID != "" {
		q += fmt.Sprintf(" AND device_id = $%d", idx)
		args = append(args, deviceID)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := h.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	sessions := []IngestSession{}
	for rows.Next() {
		s, err := scanSessionRows(rows)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		sessions = append(sessions, s)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": sessions, "limit": limit, "offset": offset})
}

func (h *Handlers) CreateIngestSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	app := appID(r)

	var body struct {
		DeviceID string          `json:"device_id"`
		StreamID string          `json:"stream_id"`
		Protocol *string         `json:"protocol"`
		Channel  *string         `json:"channel"`
		Quality  *string         `json:"quality"`
		Metadata json.RawMessage `json:"metadata"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if body.DeviceID == "" || body.StreamID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id and stream_id are required"})
		return
	}
	proto := "rtmp"
	if body.Protocol != nil {
		proto = *body.Protocol
	}
	meta := nullableJSON(body.Metadata, "{}")
	now := time.Now().UTC()

	row := h.DB.Pool.QueryRow(ctx, `
		INSERT INTO np_dev_ingest_sessions
		  (source_account_id, app_id, device_id, stream_id, protocol, channel, quality, metadata,
		   status, started_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'active',$9)
		RETURNING id, source_account_id, app_id, device_id, stream_id, status, ingest_url,
		  protocol, channel, quality, bitrate_kbps, fps, resolution, started_at,
		  last_heartbeat_at, ended_at, bytes_ingested, frames_dropped, error_count,
		  last_error, metadata, created_at, updated_at`,
		acc, app, body.DeviceID, body.StreamID, proto, body.Channel, body.Quality, meta, now,
	)
	session, err := scanSession(row)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_ = insertAudit(ctx, h.DB, acc, app, "ingest.started", &body.DeviceID, nil,
		fmt.Sprintf(`{"session_id":%q,"stream_id":%q}`, session.ID, body.StreamID))
	writeJSON(w, http.StatusCreated, session)
}

// ---- Audit ------------------------------------------------------------------

func (h *Handlers) GetAuditLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)
	limit := queryInt(r, "limit", 100)
	deviceID := r.URL.Query().Get("device_id")

	q := `SELECT id, source_account_id, app_id, device_id, action, actor_id, details, created_at
	      FROM np_dev_audit_log WHERE source_account_id = $1`
	args := []any{acc}
	if deviceID != "" {
		q += " AND device_id = $2 ORDER BY created_at DESC LIMIT $3"
		args = append(args, deviceID, limit)
	} else {
		q += " ORDER BY created_at DESC LIMIT $2"
		args = append(args, limit)
	}

	rows, err := h.DB.Pool.Query(ctx, q, args...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()
	logs := []AuditLog{}
	for rows.Next() {
		var a AuditLog
		var details []byte
		if err := rows.Scan(&a.ID, &a.SourceAccountID, &a.AppID, &a.DeviceID,
			&a.Action, &a.ActorID, &details, &a.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		a.Details = JSONField(details)
		logs = append(logs, a)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": logs})
}

// ---- Stats ------------------------------------------------------------------

func (h *Handlers) GetStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	acc := sa(r)

	var stats FleetStats
	err := h.DB.Pool.QueryRow(ctx, `
		SELECT
		  COUNT(*)                                                       AS total_devices,
		  COUNT(*) FILTER (WHERE status = 'enrolled')                   AS enrolled_devices,
		  COUNT(*) FILTER (WHERE last_seen_at > NOW() - INTERVAL '90 seconds') AS online_devices,
		  COUNT(*) FILTER (WHERE status = 'suspended')                  AS suspended_devices,
		  COUNT(*) FILTER (WHERE status = 'revoked')                    AS revoked_devices,
		  MAX(last_seen_at)
		FROM np_dev_devices WHERE source_account_id = $1`, acc).
		Scan(&stats.TotalDevices, &stats.EnrolledDevices, &stats.OnlineDevices,
			&stats.SuspendedDevices, &stats.RevokedDevices, &stats.LastActivity)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	_ = h.DB.Pool.QueryRow(ctx, `
		SELECT
		  COUNT(*)                                              AS total,
		  COUNT(*) FILTER (WHERE status = 'dispatched')        AS pending,
		  COUNT(*) FILTER (WHERE status = 'succeeded')         AS succeeded,
		  COUNT(*) FILTER (WHERE status IN ('failed','timeout')) AS failed
		FROM np_dev_commands WHERE source_account_id = $1`, acc).
		Scan(&stats.TotalCommands, &stats.PendingCommands,
			&stats.SucceededCommands, &stats.FailedCommands)

	_ = h.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_dev_ingest_sessions WHERE source_account_id = $1 AND status = 'active'`, acc).
		Scan(&stats.ActiveIngestSessions)

	_ = h.DB.Pool.QueryRow(ctx, `SELECT COUNT(*) FROM np_dev_telemetry WHERE source_account_id = $1`, acc).
		Scan(&stats.TotalTelemetryRecords)

	writeJSON(w, http.StatusOK, map[string]any{
		"plugin":  "devices",
		"version": "1.0.0",
		"stats":   stats,
		"config": map[string]any{
			"heartbeat_interval":        h.Cfg.HeartbeatInterval,
			"heartbeat_timeout":         h.Cfg.HeartbeatTimeout,
			"command_default_timeout":   h.Cfg.CommandDefaultTimeout,
			"command_max_retries":       h.Cfg.CommandMaxRetries,
			"ingest_heartbeat_interval": h.Cfg.IngestHeartbeatInterval,
		},
		"timestamp": time.Now().UTC(),
	})
}

// ---- Helpers: scan ----------------------------------------------------------

func scanDevice(row pgx.Row) (Device, error) {
	var d Device
	var caps, cfg, lbl, meta []byte
	err := row.Scan(&d.ID, &d.SourceAccountID, &d.AppID, &d.DeviceID, &d.Name,
		&d.DeviceType, &d.Model, &d.FirmwareVersion, &d.Status, &d.TrustLevel,
		&d.LastSeenAt, &caps, &cfg, &lbl, &meta, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return d, err
	}
	d.Capabilities = JSONField(caps)
	d.Config = JSONField(cfg)
	d.Labels = JSONField(lbl)
	d.Metadata = JSONField(meta)
	return d, nil
}

func getDeviceByID(ctx context.Context, db *DB, id string) (Device, error) {
	row := db.Pool.QueryRow(ctx, `
		SELECT id, source_account_id, app_id, device_id, name, device_type, model,
		  firmware_version, status, trust_level, last_seen_at, capabilities, config,
		  labels, metadata, created_at, updated_at
		FROM np_dev_devices WHERE id = $1`, id)
	return scanDevice(row)
}

func listDevices(ctx context.Context, db *DB, acc, status, dtype, trust string, limit, offset int) ([]Device, error) {
	q := `SELECT id, source_account_id, app_id, device_id, name, device_type, model,
		  firmware_version, status, trust_level, last_seen_at, capabilities, config,
		  labels, metadata, created_at, updated_at
		  FROM np_dev_devices WHERE source_account_id = $1`
	args := []any{acc}
	idx := 2
	if status != "" {
		q += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if dtype != "" {
		q += fmt.Sprintf(" AND device_type = $%d", idx)
		args = append(args, dtype)
		idx++
	}
	if trust != "" {
		q += fmt.Sprintf(" AND trust_level = $%d", idx)
		args = append(args, trust)
		idx++
	}
	q += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, limit, offset)

	rows, err := db.Pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devs []Device
	for rows.Next() {
		var d Device
		var caps, cfg, lbl, meta []byte
		if err := rows.Scan(&d.ID, &d.SourceAccountID, &d.AppID, &d.DeviceID, &d.Name,
			&d.DeviceType, &d.Model, &d.FirmwareVersion, &d.Status, &d.TrustLevel,
			&d.LastSeenAt, &caps, &cfg, &lbl, &meta, &d.CreatedAt, &d.UpdatedAt); err != nil {
			return nil, err
		}
		d.Capabilities = JSONField(caps)
		d.Config = JSONField(cfg)
		d.Labels = JSONField(lbl)
		d.Metadata = JSONField(meta)
		devs = append(devs, d)
	}
	return devs, nil
}

func scanCommand(row pgx.Row) (Command, error) {
	var c Command
	var payload, result []byte
	err := row.Scan(&c.ID, &c.SourceAccountID, &c.AppID, &c.DeviceID, &c.CommandType,
		&c.CommandIDField, &payload, &c.Status, &c.Priority, &c.DispatchedAt,
		&c.AckedAt, &c.StartedAt, &c.CompletedAt, &result, &c.Error,
		&c.TimeoutSeconds, &c.Deadline, &c.RetryCount, &c.MaxRetries,
		&c.IdempotencyKey, &c.Metadata, &c.CreatedAt)
	if err != nil {
		return c, err
	}
	c.Payload = JSONField(payload)
	c.Result = JSONField(result)
	return c, nil
}

type pgxRows interface {
	Scan(dest ...any) error
}

func scanCommandRows(rows pgxRows) (Command, error) {
	var c Command
	var payload, result, meta []byte
	err := rows.Scan(&c.ID, &c.SourceAccountID, &c.AppID, &c.DeviceID, &c.CommandType,
		&c.CommandIDField, &payload, &c.Status, &c.Priority, &c.DispatchedAt,
		&c.AckedAt, &c.StartedAt, &c.CompletedAt, &result, &c.Error,
		&c.TimeoutSeconds, &c.Deadline, &c.RetryCount, &c.MaxRetries,
		&c.IdempotencyKey, &meta, &c.CreatedAt)
	if err != nil {
		return c, err
	}
	c.Payload = JSONField(payload)
	c.Result = JSONField(result)
	c.Metadata = JSONField(meta)
	return c, nil
}

func scanTelemetry(row pgx.Row) (Telemetry, error) {
	var t Telemetry
	var data []byte
	err := row.Scan(&t.ID, &t.SourceAccountID, &t.AppID, &t.DeviceID,
		&t.TelemetryType, &data, &t.RecordedAt, &t.ReceivedAt)
	if err != nil {
		return t, err
	}
	t.Data = JSONField(data)
	return t, nil
}

func scanTelemetryRows(rows pgxRows) (Telemetry, error) {
	var t Telemetry
	var data []byte
	err := rows.Scan(&t.ID, &t.SourceAccountID, &t.AppID, &t.DeviceID,
		&t.TelemetryType, &data, &t.RecordedAt, &t.ReceivedAt)
	if err != nil {
		return t, err
	}
	t.Data = JSONField(data)
	return t, nil
}

func scanSession(row pgx.Row) (IngestSession, error) {
	var s IngestSession
	var meta []byte
	err := row.Scan(&s.ID, &s.SourceAccountID, &s.AppID, &s.DeviceID, &s.StreamID,
		&s.Status, &s.IngestURL, &s.Protocol, &s.Channel, &s.Quality,
		&s.BitrateKbps, &s.FPS, &s.Resolution, &s.StartedAt, &s.LastHeartbeatAt,
		&s.EndedAt, &s.BytesIngested, &s.FramesDropped, &s.ErrorCount,
		&s.LastError, &meta, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	s.Metadata = JSONField(meta)
	return s, nil
}

func scanSessionRows(rows pgxRows) (IngestSession, error) {
	var s IngestSession
	var meta []byte
	err := rows.Scan(&s.ID, &s.SourceAccountID, &s.AppID, &s.DeviceID, &s.StreamID,
		&s.Status, &s.IngestURL, &s.Protocol, &s.Channel, &s.Quality,
		&s.BitrateKbps, &s.FPS, &s.Resolution, &s.StartedAt, &s.LastHeartbeatAt,
		&s.EndedAt, &s.BytesIngested, &s.FramesDropped, &s.ErrorCount,
		&s.LastError, &meta, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return s, err
	}
	s.Metadata = JSONField(meta)
	return s, nil
}

func insertAudit(ctx context.Context, db *DB, acc, app, action string, deviceID *string, actorID *string, details string) error {
	_, err := db.Pool.Exec(ctx, `
		INSERT INTO np_dev_audit_log (source_account_id, app_id, device_id, action, actor_id, details)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)`,
		acc, app, deviceID, action, actorID, details)
	return err
}

func nullableJSON(raw json.RawMessage, def string) []byte {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte(def)
	}
	return raw
}

func jsonOrNil(raw json.RawMessage) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return string(raw)
}
