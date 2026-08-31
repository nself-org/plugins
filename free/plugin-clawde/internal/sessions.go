// Package internal provides session management and daemon health for plugin-clawde.
//
// Purpose: Track ClawDE session lifecycle (create, heartbeat, close) and daemon health.
//   Sessions are scoped per source_account_id (Multi-App Isolation Convention).
//   Events are append-only and can be streamed via SSE.
// Inputs: HTTP JSON requests with source_account_id.
// Outputs: JSON session state, SSE event streams.
// Constraints:
//   - Sessions expire after NSELF_CLAWDE_SESSION_TTL_MINUTES (default 60) with no heartbeat.
//   - source_account_id required on all operations (Multi-App Isolation Convention).
//   - Daemon health probes are fire-and-forget; failures are recorded but not fatal.
//   - SSRF N/A: daemon probe uses localhost only (NSELF_CLAWDE_DAEMON_ADDR).
package internal

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Handlers holds the DB handle and daemon address for plugin-clawde.
type Handlers struct {
	DB         *sql.DB
	DaemonAddr string // default "localhost:3848" (ClawDE PTY bridge)
}

// SessionStatus represents the lifecycle state of a ClawDE session.
type SessionStatus string

const (
	SessionActive  SessionStatus = "active"
	SessionClosed  SessionStatus = "closed"
	SessionExpired SessionStatus = "expired"
)

// Session is a ClawDE session record.
type Session struct {
	ID              string        `json:"id"`
	SourceAccountID string        `json:"source_account_id"`
	Status          SessionStatus `json:"status"`
	CreatedAt       time.Time     `json:"created_at"`
	LastHeartbeat   time.Time     `json:"last_heartbeat"`
	ClosedAt        *time.Time    `json:"closed_at,omitempty"`
}

// Health returns 200 OK when DB is reachable, 503 otherwise.
func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	if h.DB == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}
	if err := h.DB.PingContext(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"unhealthy"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// CreateSession handles POST /sessions. Creates a new active session.
func (h *Handlers) CreateSession(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID              string `json:"id"`
		SourceAccountID string `json:"source_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, `{"error":"id required"}`, http.StatusBadRequest)
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = "primary"
	}

	now := time.Now().UTC()
	_, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO np_clawde_sessions (id, source_account_id, status, created_at, last_heartbeat)
		VALUES ($1, $2, $3, $4, $4)
		ON CONFLICT (id, source_account_id) DO UPDATE SET
			status = 'active',
			last_heartbeat = EXCLUDED.last_heartbeat
	`, req.ID, req.SourceAccountID, string(SessionActive), now)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	s := Session{
		ID:              req.ID,
		SourceAccountID: req.SourceAccountID,
		Status:          SessionActive,
		CreatedAt:       now,
		LastHeartbeat:   now,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(s)
}

// Heartbeat handles POST /sessions/{id}/heartbeat. Updates last_heartbeat.
func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/heartbeat")
	if sessionID == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}

	accountID := r.URL.Query().Get("source_account_id")
	if accountID == "" {
		accountID = "primary"
	}

	result, err := h.DB.ExecContext(r.Context(), `
		UPDATE np_clawde_sessions
		SET last_heartbeat = NOW()
		WHERE id = $1 AND source_account_id = $2 AND status = 'active'
	`, sessionID, accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"session not found or not active"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

// CloseSession handles DELETE /sessions/{id}. Marks session closed.
func (h *Handlers) CloseSession(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/sessions/")
	if sessionID == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}

	accountID := r.URL.Query().Get("source_account_id")
	if accountID == "" {
		accountID = "primary"
	}

	result, err := h.DB.ExecContext(r.Context(), `
		UPDATE np_clawde_sessions
		SET status = 'closed', closed_at = NOW()
		WHERE id = $1 AND source_account_id = $2 AND status = 'active'
	`, sessionID, accountID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		http.Error(w, `{"error":"session not found or already closed"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"closed"}`))
}

// AppendEvent handles POST /sessions/{id}/events. Appends an event to np_clawde_events.
func (h *Handlers) AppendEvent(w http.ResponseWriter, r *http.Request) {
	sessionID := strings.TrimPrefix(r.URL.Path, "/sessions/")
	sessionID = strings.TrimSuffix(sessionID, "/events")
	if sessionID == "" {
		http.Error(w, `{"error":"session id required"}`, http.StatusBadRequest)
		return
	}

	var req struct {
		EventType       string `json:"event_type"`
		Payload         string `json:"payload"`
		SourceAccountID string `json:"source_account_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid_json"}`, http.StatusBadRequest)
		return
	}
	if req.EventType == "" {
		http.Error(w, `{"error":"event_type required"}`, http.StatusBadRequest)
		return
	}
	if req.SourceAccountID == "" {
		req.SourceAccountID = "primary"
	}

	_, err := h.DB.ExecContext(r.Context(), `
		INSERT INTO np_clawde_events (session_id, source_account_id, event_type, payload, created_at)
		VALUES ($1, $2, $3, $4, NOW())
	`, sessionID, req.SourceAccountID, req.EventType, req.Payload)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"recorded"}`))
}
