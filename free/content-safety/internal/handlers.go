package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Handlers holds DB and Config for request handling.
type Handlers struct {
	DB  *DB
	Cfg Config
}

// sourceAccountID extracts X-Source-Account-ID header, defaulting to "primary".
func sourceAccountID(r *http.Request) string {
	if v := r.Header.Get("X-Source-Account-ID"); v != "" {
		return v
	}
	return "primary"
}

func sa(r *http.Request) string { return sourceAccountID(r) }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func errJSON(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		errJSON(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func (h *Handlers) HealthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":    "ok",
		"plugin":    "content-safety",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *Handlers) ReadyHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := h.DB.Pool.Exec(r.Context(), "SELECT 1"); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ready":     false,
			"plugin":    "content-safety",
			"error":     "database unavailable",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ready":     true,
		"plugin":    "content-safety",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// ============================================================
// Trust-Safety: Evidence
// ============================================================

func (h *Handlers) EvidenceCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type          string         `json:"type"`
		Content       map[string]any `json:"content"`
		Reason        string         `json:"reason"`
		Source        string         `json:"source"`
		WorkspaceID   string         `json:"workspaceId"`
		CollectorID   *string        `json:"collectorId"`
		CollectorRole *string        `json:"collectorRole"`
		Priority      string         `json:"priority"`
		Encrypt       bool           `json:"encrypt"`
		Metadata      map[string]any `json:"metadata"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Type == "" || body.Content == nil || body.Reason == "" || body.Source == "" || body.WorkspaceID == "" {
		errJSON(w, http.StatusBadRequest, "type, content, reason, source, and workspaceId are required")
		return
	}
	if body.Priority == "" {
		body.Priority = "normal"
	}
	contentJSON, _ := json.Marshal(body.Content)
	metaJSON, _ := json.Marshal(body.Metadata)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_evidence (source_account_id, type, content, reason, source, workspace_id, collector_id, collector_role, priority, is_encrypted, metadata)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`,
		sa(r), body.Type, contentJSON, body.Reason, body.Source, body.WorkspaceID,
		body.CollectorID, body.CollectorRole, body.Priority, body.Encrypt, metaJSON).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"evidence_id": id})
}

func (h *Handlers) EvidenceList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	workspaceID := q.Get("workspaceId")
	status := q.Get("status")

	where := "source_account_id = $1"
	args := []any{sa(r)}
	n := 2
	if workspaceID != "" {
		where += fmt.Sprintf(" AND workspace_id = $%d", n)
		args = append(args, workspaceID)
		n++
	}
	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", n)
		args = append(args, status)
		n++
	}
	_ = n

	rows, err := h.DB.Pool.Query(r.Context(),
		fmt.Sprintf(`SELECT id, type, reason, source, workspace_id, collector_id, priority, is_encrypted, status, legal_hold_id, created_at
		 FROM np_cs_evidence WHERE %s ORDER BY created_at DESC LIMIT 100`, where), args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	items := make([]map[string]any, 0)
	for rows.Next() {
		var id, evType, reason, source, wsID, priority, evStatus string
		var collectorID, legalHoldID *string
		var isEncrypted bool
		var createdAt time.Time
		if err := rows.Scan(&id, &evType, &reason, &source, &wsID, &collectorID, &priority, &isEncrypted, &evStatus, &legalHoldID, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append(items, map[string]any{
			"id": id, "type": evType, "reason": reason, "source": source,
			"workspace_id": wsID, "collector_id": collectorID,
			"priority": priority, "is_encrypted": isEncrypted,
			"status": evStatus, "legal_hold_id": legalHoldID, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"evidence": items, "count": len(items)})
}

// ============================================================
// Trust-Safety: Legal Holds
// ============================================================

func (h *Handlers) LegalHoldCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string         `json:"name"`
		Description *string        `json:"description"`
		Scope       map[string]any `json:"scope"`
		Criteria    map[string]any `json:"criteria"`
		CreatedBy   *string        `json:"createdBy"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Name == "" || body.Scope == nil || body.Criteria == nil {
		errJSON(w, http.StatusBadRequest, "name, scope, and criteria are required")
		return
	}
	scopeJSON, _ := json.Marshal(body.Scope)
	criteriaJSON, _ := json.Marshal(body.Criteria)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_legal_holds (source_account_id, name, description, scope, criteria, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sa(r), body.Name, body.Description, scopeJSON, criteriaJSON, body.CreatedBy).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"legal_hold_id": id})
}

func (h *Handlers) LegalHoldList(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	where := "source_account_id = $1"
	args := []any{sa(r)}
	if status != "" {
		where += " AND status = $2"
		args = append(args, status)
	}
	rows, err := h.DB.Pool.Query(r.Context(),
		fmt.Sprintf(`SELECT id, name, description, status, created_by, created_at FROM np_cs_legal_holds WHERE %s ORDER BY created_at DESC`, where), args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	holds := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, holdStatus string
		var description, createdBy *string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &description, &holdStatus, &createdBy, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		holds = append(holds, map[string]any{
			"id": id, "name": name, "description": description,
			"status": holdStatus, "created_by": createdBy, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"legal_holds": holds})
}

// ============================================================
// Trust-Safety: Evidence Exports
// ============================================================

func (h *Handlers) EvidenceExportCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		LegalHoldID *string  `json:"legalHoldId"`
		EvidenceIDs []string `json:"evidenceIds"`
		Format      string   `json:"format"`
		RequestedBy *string  `json:"requestedBy"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Format == "" {
		body.Format = "json"
	}
	idsJSON, _ := json.Marshal(body.EvidenceIDs)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_evidence_exports (source_account_id, legal_hold_id, evidence_ids, format, requested_by)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id`,
		sa(r), body.LegalHoldID, idsJSON, body.Format, body.RequestedBy).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"export_id": id})
}

func (h *Handlers) EvidenceExportList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, legal_hold_id, format, status, requested_by, download_url, created_at
		 FROM np_cs_evidence_exports WHERE source_account_id = $1 ORDER BY created_at DESC LIMIT 50`, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	exports := make([]map[string]any, 0)
	for rows.Next() {
		var id, format, status string
		var legalHoldID, requestedBy, downloadURL *string
		var createdAt time.Time
		if err := rows.Scan(&id, &legalHoldID, &format, &status, &requestedBy, &downloadURL, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		exports = append(exports, map[string]any{
			"id": id, "legal_hold_id": legalHoldID, "format": format,
			"status": status, "requested_by": requestedBy, "download_url": downloadURL, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"exports": exports})
}

// ============================================================
// Trust-Safety: Statistics
// ============================================================

func (h *Handlers) TrustSafetyStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	account := sa(r)
	var totalEvidence, totalHolds, activeHolds int
	_ = h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM np_cs_evidence WHERE source_account_id = $1", account).Scan(&totalEvidence)
	_ = h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM np_cs_legal_holds WHERE source_account_id = $1", account).Scan(&totalHolds)
	_ = h.DB.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM np_cs_legal_holds WHERE source_account_id = $1 AND status = 'active'", account).Scan(&activeHolds)
	writeJSON(w, http.StatusOK, map[string]any{
		"total_evidence": totalEvidence,
		"total_holds":    totalHolds,
		"active_holds":   activeHolds,
	})
}

// ============================================================
// Spam: Analyze
// ============================================================

func (h *Handlers) SpamAnalyze(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content     string `json:"content"`
		UserID      string `json:"userId"`
		ChannelID   string `json:"channelId"`
		WorkspaceID string `json:"workspaceId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Content == "" {
		errJSON(w, http.StatusBadRequest, "content is required")
		return
	}
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, name, rule_type, pattern, action FROM np_cs_spam_rules
		 WHERE source_account_id = $1 AND is_enabled = true ORDER BY priority DESC`, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type matchItem struct {
		RuleID   string `json:"rule_id"`
		RuleName string `json:"rule_name"`
		Action   string `json:"action"`
	}
	matches := make([]matchItem, 0)
	for rows.Next() {
		var ruleID, name, ruleType, action string
		var pattern *string
		if err := rows.Scan(&ruleID, &name, &ruleType, &pattern, &action); err != nil {
			continue
		}
		if pattern != nil && *pattern != "" && strings.Contains(strings.ToLower(body.Content), strings.ToLower(*pattern)) {
			matches = append(matches, matchItem{RuleID: ruleID, RuleName: name, Action: action})
		}
	}
	isSpam := len(matches) > 0
	score := 0.0
	if isSpam {
		score = 0.9
	}
	recommendedAction := "allow"
	if isSpam {
		recommendedAction = "flag"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"is_spam":            isSpam,
		"spam_score":         score,
		"matched_rules":      matches,
		"recommended_action": recommendedAction,
	})
}

// ============================================================
// Spam: Config
// ============================================================

func (h *Handlers) SpamConfigGet(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspaceId")
	if workspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	var cfg SpamConfig
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT workspace_id, source_account_id, sensitivity, auto_delete, notify_moderators, quarantine_threshold, config, updated_at
		 FROM np_cs_spam_configs WHERE workspace_id = $1 AND source_account_id = $2`,
		workspaceID, sa(r)).Scan(&cfg.WorkspaceID, &cfg.SourceAccountID, &cfg.Sensitivity,
		&cfg.AutoDelete, &cfg.NotifyModerators, &cfg.QuarantineThreshold, new([]byte), &cfg.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{
			"workspace_id": workspaceID, "sensitivity": "medium",
			"auto_delete": false, "notify_moderators": true, "quarantine_threshold": 0.8,
		})
		return
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, cfg)
}

func (h *Handlers) SpamConfigUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID         string         `json:"workspaceId"`
		Sensitivity         *string        `json:"sensitivity"`
		AutoDelete          *bool          `json:"autoDelete"`
		NotifyModerators    *bool          `json:"notifyModerators"`
		QuarantineThreshold *float64       `json:"quarantineThreshold"`
		Config              map[string]any `json:"config"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.WorkspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	sensitivity := "medium"
	if body.Sensitivity != nil {
		sensitivity = *body.Sensitivity
	}
	autoDelete := false
	if body.AutoDelete != nil {
		autoDelete = *body.AutoDelete
	}
	notifyModerators := true
	if body.NotifyModerators != nil {
		notifyModerators = *body.NotifyModerators
	}
	quarantineThreshold := 0.8
	if body.QuarantineThreshold != nil {
		quarantineThreshold = *body.QuarantineThreshold
	}
	configJSON, _ := json.Marshal(body.Config)
	_, err := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO np_cs_spam_configs (workspace_id, source_account_id, sensitivity, auto_delete, notify_moderators, quarantine_threshold, config)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)
		 ON CONFLICT (workspace_id, source_account_id) DO UPDATE SET
		 sensitivity = EXCLUDED.sensitivity, auto_delete = EXCLUDED.auto_delete,
		 notify_moderators = EXCLUDED.notify_moderators, quarantine_threshold = EXCLUDED.quarantine_threshold,
		 config = EXCLUDED.config, updated_at = NOW()`,
		body.WorkspaceID, sa(r), sensitivity, autoDelete, notifyModerators, quarantineThreshold, configJSON)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ============================================================
// Spam: Rate Limits
// ============================================================

func (h *Handlers) RateLimitList(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	where := "source_account_id = $1"
	args := []any{sa(r)}
	n := 2
	if userID != "" {
		where += fmt.Sprintf(" AND user_id = $%d", n)
		args = append(args, userID)
		n++
	}
	_ = n
	rows, err := h.DB.Pool.Query(r.Context(),
		fmt.Sprintf(`SELECT id, user_id, channel_id, workspace_id, limit_type, action_taken, created_at
		 FROM np_cs_rate_limit_violations WHERE %s ORDER BY created_at DESC LIMIT 100`, where), args...)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	violations := make([]map[string]any, 0)
	for rows.Next() {
		var id, userIDVal, limitType, actionTaken string
		var channelID, workspaceID *string
		var createdAt time.Time
		if err := rows.Scan(&id, &userIDVal, &channelID, &workspaceID, &limitType, &actionTaken, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		violations = append(violations, map[string]any{
			"id": id, "user_id": userIDVal, "channel_id": channelID,
			"workspace_id": workspaceID, "limit_type": limitType,
			"action_taken": actionTaken, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"violations": violations})
}

func (h *Handlers) RateLimitCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID      string  `json:"userId"`
		ChannelID   *string `json:"channelId"`
		WorkspaceID *string `json:"workspaceId"`
		LimitType   string  `json:"limitType"`
		ActionTaken string  `json:"actionTaken"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.UserID == "" || body.LimitType == "" {
		errJSON(w, http.StatusBadRequest, "userId and limitType are required")
		return
	}
	if body.ActionTaken == "" {
		body.ActionTaken = "blocked"
	}
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_rate_limit_violations (source_account_id, user_id, channel_id, workspace_id, limit_type, action_taken)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sa(r), body.UserID, body.ChannelID, body.WorkspaceID, body.LimitType, body.ActionTaken).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"violation_id": id})
}

func (h *Handlers) RateLimitDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ct, err := h.DB.Pool.Exec(r.Context(),
		"DELETE FROM np_cs_rate_limit_violations WHERE id = $1 AND source_account_id = $2", id, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ct.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "violation not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ============================================================
// Spam: Rules
// ============================================================

func (h *Handlers) SpamRuleList(w http.ResponseWriter, r *http.Request) {
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, name, description, rule_type, pattern, action, is_enabled, priority, created_at
		 FROM np_cs_spam_rules WHERE source_account_id = $1 ORDER BY priority DESC, created_at DESC`, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	rules := make([]map[string]any, 0)
	for rows.Next() {
		var id, name, ruleType, action string
		var description, pattern *string
		var isEnabled bool
		var priority int
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &description, &ruleType, &pattern, &action, &isEnabled, &priority, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		rules = append(rules, map[string]any{
			"id": id, "name": name, "description": description,
			"rule_type": ruleType, "pattern": pattern, "action": action,
			"is_enabled": isEnabled, "priority": priority, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handlers) SpamRuleCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name        string         `json:"name"`
		Description *string        `json:"description"`
		RuleType    string         `json:"rule_type"`
		Pattern     *string        `json:"pattern"`
		Conditions  map[string]any `json:"conditions"`
		Action      string         `json:"action"`
		IsEnabled   bool           `json:"is_enabled"`
		Priority    int            `json:"priority"`
		CreatedBy   *string        `json:"created_by"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.Name == "" || body.RuleType == "" {
		errJSON(w, http.StatusBadRequest, "name and rule_type are required")
		return
	}
	if body.Action == "" {
		body.Action = "flag"
	}
	condJSON, _ := json.Marshal(body.Conditions)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_spam_rules (source_account_id, name, description, rule_type, pattern, conditions, action, is_enabled, priority, created_by)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`,
		sa(r), body.Name, body.Description, body.RuleType, body.Pattern, condJSON,
		body.Action, body.IsEnabled, body.Priority, body.CreatedBy).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"rule_id": id})
}

func (h *Handlers) SpamRuleDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ct, err := h.DB.Pool.Exec(r.Context(),
		"DELETE FROM np_cs_spam_rules WHERE id = $1 AND source_account_id = $2", id, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ct.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "rule not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// ============================================================
// Raid: Status
// ============================================================

func (h *Handlers) RaidStatusGet(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspaceId")
	if workspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	var totalEvents, recentEvents int
	_ = h.DB.Pool.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM np_cs_raid_events WHERE source_account_id = $1 AND workspace_id = $2",
		sa(r), workspaceID).Scan(&totalEvents)
	_ = h.DB.Pool.QueryRow(r.Context(),
		"SELECT COUNT(*) FROM np_cs_raid_events WHERE source_account_id = $1 AND workspace_id = $2 AND created_at > NOW() - INTERVAL '1 hour'",
		sa(r), workspaceID).Scan(&recentEvents)

	var activeLockdownID *string
	var lockdownLevel string
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT id, level FROM np_cs_lockdowns WHERE source_account_id = $1 AND workspace_id = $2 AND is_active = true ORDER BY created_at DESC LIMIT 1`,
		sa(r), workspaceID).Scan(&activeLockdownID, &lockdownLevel)
	if err != nil && err != pgx.ErrNoRows {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"workspace_id":     workspaceID,
		"total_events":     totalEvents,
		"recent_events_1h": recentEvents,
		"active_lockdown":  activeLockdownID != nil,
		"lockdown_id":      activeLockdownID,
		"lockdown_level":   lockdownLevel,
	})
}

func (h *Handlers) RaidEventCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string         `json:"workspaceId"`
		ChannelID   *string        `json:"channelId"`
		EventType   string         `json:"eventType"`
		Severity    string         `json:"severity"`
		Details     map[string]any `json:"details"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.WorkspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	if body.EventType == "" {
		body.EventType = "join_spike"
	}
	if body.Severity == "" {
		body.Severity = "medium"
	}
	detailsJSON, _ := json.Marshal(body.Details)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_raid_events (source_account_id, workspace_id, channel_id, event_type, severity, details)
		 VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
		sa(r), body.WorkspaceID, body.ChannelID, body.EventType, body.Severity, detailsJSON).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"event_id": id})
}

func (h *Handlers) RaidStatusUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		EventID string `json:"eventId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.EventID == "" {
		errJSON(w, http.StatusBadRequest, "eventId is required")
		return
	}
	ct, err := h.DB.Pool.Exec(r.Context(),
		"UPDATE np_cs_raid_events SET resolved_at = NOW() WHERE id = $1 AND source_account_id = $2",
		body.EventID, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ct.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "event not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}

// ============================================================
// Raid: Lockdown
// ============================================================

func (h *Handlers) LockdownGet(w http.ResponseWriter, r *http.Request) {
	workspaceID := r.URL.Query().Get("workspaceId")
	if workspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	rows, err := h.DB.Pool.Query(r.Context(),
		`SELECT id, workspace_id, channel_id, level, reason, activated_by, is_active, created_at
		 FROM np_cs_lockdowns WHERE source_account_id = $1 AND workspace_id = $2 ORDER BY created_at DESC LIMIT 10`,
		sa(r), workspaceID)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	lockdowns := make([]map[string]any, 0)
	for rows.Next() {
		var id, wsID, level string
		var channelID, reason, activatedBy *string
		var isActive bool
		var createdAt time.Time
		if err := rows.Scan(&id, &wsID, &channelID, &level, &reason, &activatedBy, &isActive, &createdAt); err != nil {
			errJSON(w, http.StatusInternalServerError, err.Error())
			return
		}
		lockdowns = append(lockdowns, map[string]any{
			"id": id, "workspace_id": wsID, "channel_id": channelID,
			"level": level, "reason": reason, "activated_by": activatedBy,
			"is_active": isActive, "created_at": createdAt,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"lockdowns": lockdowns})
}

func (h *Handlers) LockdownCreate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WorkspaceID string         `json:"workspaceId"`
		ChannelID   *string        `json:"channelId"`
		Level       string         `json:"level"`
		Reason      *string        `json:"reason"`
		ActivatedBy *string        `json:"activatedBy"`
		Config      map[string]any `json:"config"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.WorkspaceID == "" {
		errJSON(w, http.StatusBadRequest, "workspaceId is required")
		return
	}
	if body.Level == "" {
		body.Level = "partial"
	}
	configJSON, _ := json.Marshal(body.Config)
	var id string
	err := h.DB.Pool.QueryRow(r.Context(),
		`INSERT INTO np_cs_lockdowns (source_account_id, workspace_id, channel_id, level, reason, activated_by, config)
		 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
		sa(r), body.WorkspaceID, body.ChannelID, body.Level, body.Reason, body.ActivatedBy, configJSON).Scan(&id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"lockdown_id": id})
}

func (h *Handlers) LockdownDelete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	deactivatedBy := r.URL.Query().Get("deactivatedBy")
	var deactivatedByPtr *string
	if deactivatedBy != "" {
		deactivatedByPtr = &deactivatedBy
	}
	ct, err := h.DB.Pool.Exec(r.Context(),
		`UPDATE np_cs_lockdowns SET is_active = false, deactivated_by = $1, deactivated_at = NOW(), updated_at = NOW()
		 WHERE id = $2 AND source_account_id = $3 AND is_active = true`,
		deactivatedByPtr, id, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ct.RowsAffected() == 0 {
		errJSON(w, http.StatusNotFound, "active lockdown not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deactivated": true})
}

// ============================================================
// Abuse: Trust Score
// ============================================================

func (h *Handlers) AbuseTrustGet(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("userId")
	if userID == "" {
		errJSON(w, http.StatusBadRequest, "userId is required")
		return
	}
	var score AbuseScore
	var metaRaw []byte
	err := h.DB.Pool.QueryRow(r.Context(),
		`SELECT user_id, source_account_id, trust_score, risk_level, total_events, positive_events, negative_events, last_event_at, metadata, created_at, updated_at
		 FROM np_cs_abuse_scores WHERE user_id = $1 AND source_account_id = $2`,
		userID, sa(r)).Scan(&score.UserID, &score.SourceAccountID, &score.TrustScore, &score.RiskLevel,
		&score.TotalEvents, &score.PositiveEvents, &score.NegativeEvents, &score.LastEventAt,
		&metaRaw, &score.CreatedAt, &score.UpdatedAt)
	if err == pgx.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{
			"user_id": userID, "trust_score": 0.5, "risk_level": "low",
			"total_events": 0, "positive_events": 0, "negative_events": 0,
		})
		return
	}
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.Unmarshal(metaRaw, &score.Metadata)
	writeJSON(w, http.StatusOK, score)
}

func (h *Handlers) AbuseTrustRegister(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID string `json:"userId"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.UserID == "" {
		errJSON(w, http.StatusBadRequest, "userId is required")
		return
	}
	_, err := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO np_cs_abuse_scores (user_id, source_account_id) VALUES ($1, $2)
		 ON CONFLICT (user_id, source_account_id) DO NOTHING`,
		body.UserID, sa(r))
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"registered": true})
}

func (h *Handlers) AbuseTrustUpdate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID     string   `json:"userId"`
		Event      string   `json:"event"`
		ScoreDelta *float64 `json:"scoreDelta"`
	}
	if !decode(w, r, &body) {
		return
	}
	if body.UserID == "" {
		errJSON(w, http.StatusBadRequest, "userId is required")
		return
	}
	positive := body.Event == "positive_interaction" || body.Event == "message_sent" || body.Event == "helpful_flag"
	delta := 0.0
	if body.ScoreDelta != nil {
		delta = *body.ScoreDelta
	} else if positive {
		delta = 0.01
	} else {
		delta = -0.05
	}
	posIncr, negIncr := 0, 1
	if positive {
		posIncr, negIncr = 1, 0
	}
	_, err := h.DB.Pool.Exec(r.Context(),
		`INSERT INTO np_cs_abuse_scores (user_id, source_account_id, trust_score, positive_events, negative_events, total_events, last_event_at)
		 VALUES ($1, $2, GREATEST(0, LEAST(1, 0.5 + $3)), $4, $5, 1, NOW())
		 ON CONFLICT (user_id, source_account_id) DO UPDATE SET
		 trust_score = GREATEST(0, LEAST(1, np_cs_abuse_scores.trust_score + $3)),
		 positive_events = np_cs_abuse_scores.positive_events + $4,
		 negative_events = np_cs_abuse_scores.negative_events + $5,
		 total_events = np_cs_abuse_scores.total_events + 1,
		 last_event_at = NOW(), updated_at = NOW()`,
		body.UserID, sa(r), delta, posIncr, negIncr)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"success": true})
}
