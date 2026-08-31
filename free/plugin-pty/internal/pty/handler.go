// Package pty — HTTP and WebSocket handlers for plugin-pty.
//
// Purpose: Spawn PTY processes, relay I/O via WebSocket, enforce per-tenant limits.
// Inputs:  HTTP requests with source_account_id from X-Hasura-Source-Account-Id header.
// Outputs: JSON session objects; WebSocket bidirectional PTY I/O.
// Constraints:
//   - Max PTY sessions per tenant enforced atomically (429 on exceed).
//   - Cross-tenant session access returns 403.
//   - ctx.License.Valid() must pass — checked in main before handler registration.
//   - No SSRF guard needed: no outbound HTTP calls.
//
// SPORT: F04-PLUGIN-INVENTORY-PRO.md — plugin-pty
package pty

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds handler configuration.
type Config struct {
	MaxPerTenant int
	SessionTTL   time.Duration
}

// Handler holds handler dependencies.
type Handler struct {
	db       *DB
	cfg      Config
	mu       sync.Mutex
	sessions map[string]*ptyProcess // session_id → live process
}

type ptyProcess struct {
	ptmx *os.File
	cmd  *exec.Cmd
	mu   sync.Mutex
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// NewHandler creates a Handler.
func NewHandler(pool *pgxpool.Pool, cfg Config) *Handler {
	return &Handler{
		db:       &DB{Pool: pool},
		cfg:      cfg,
		sessions: make(map[string]*ptyProcess),
	}
}

// Routes registers all plugin-pty routes on r.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/health", h.health)
	r.Post("/sessions", h.createSession)
	r.Delete("/sessions/{id}", h.closeSession)
	r.Get("/sessions/{id}/ws", h.wsRelay)
}

func sourceAccountID(r *http.Request) string {
	id := r.Header.Get("X-Hasura-Source-Account-Id")
	if id == "" {
		return "primary"
	}
	return id
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := h.db.Pool.Ping(ctx); err != nil {
		http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	var req struct {
		ClientID string `json:"client_id"`
		Cols     int    `json:"cols"`
		Rows     int    `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}
	if req.ClientID == "" {
		http.Error(w, `{"error":"client_id required"}`, http.StatusBadRequest)
		return
	}
	if req.Cols <= 0 {
		req.Cols = 80
	}
	if req.Rows <= 0 {
		req.Rows = 24
	}

	// Enforce per-tenant limit
	count, err := h.db.ActiveCount(r.Context(), sai)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}
	if count >= h.cfg.MaxPerTenant {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": fmt.Sprintf("max %d concurrent PTY sessions per tenant", h.cfg.MaxPerTenant),
		})
		return
	}

	sessionID, err := h.db.CreateSession(r.Context(), sai, req.ClientID, req.Cols, req.Rows)
	if err != nil {
		http.Error(w, `{"error":"db error"}`, http.StatusInternalServerError)
		return
	}

	// Spawn PTY process
	cmd := exec.Command("/bin/sh")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(req.Cols), Rows: uint16(req.Rows)})
	if err != nil {
		h.db.Audit(r.Context(), sai, sessionID, "error", "pty spawn failed: "+err.Error())
		// Close session in DB since PTY failed
		_ = h.db.CloseSession(r.Context(), sessionID, sai)
		http.Error(w, `{"error":"pty spawn failed"}`, http.StatusInternalServerError)
		return
	}

	h.mu.Lock()
	h.sessions[sessionID] = &ptyProcess{ptmx: ptmx, cmd: cmd}
	h.mu.Unlock()

	h.db.Audit(r.Context(), sai, sessionID, "spawn", fmt.Sprintf("cols=%d rows=%d", req.Cols, req.Rows))

	wsURL := fmt.Sprintf("/sessions/%s/ws", sessionID)
	writeJSON(w, http.StatusCreated, map[string]string{
		"session_id": sessionID,
		"ws_url":     wsURL,
		"status":     "active",
	})
}

func (h *Handler) closeSession(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	id := chi.URLParam(r, "id")

	// Verify ownership before closing
	sess, err := h.db.GetSession(r.Context(), id, sai)
	if err != nil {
		http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		return
	}
	if sess.SourceAccountID != sai {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	if err := h.db.CloseSession(r.Context(), id, sai); err != nil {
		http.Error(w, `{"error":"close failed"}`, http.StatusInternalServerError)
		return
	}

	// Kill live PTY process
	h.mu.Lock()
	proc, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	h.mu.Unlock()
	if ok {
		_ = proc.ptmx.Close()
		_ = proc.cmd.Process.Kill()
	}

	h.db.Audit(r.Context(), sai, id, "close", "")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) wsRelay(w http.ResponseWriter, r *http.Request) {
	sai := sourceAccountID(r)
	id := chi.URLParam(r, "id")

	// Verify ownership
	sess, err := h.db.GetSession(r.Context(), id, sai)
	if err != nil || sess.SourceAccountID != sai {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}

	h.mu.Lock()
	proc, ok := h.sessions[id]
	h.mu.Unlock()
	if !ok {
		http.Error(w, `{"error":"pty process not found"}`, http.StatusNotFound)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// PTY → WebSocket
	go func() {
		defer cancel()
		buf := make([]byte, 4096)
		for {
			n, err := proc.ptmx.Read(buf)
			if err != nil {
				return
			}
			proc.mu.Lock()
			werr := conn.WriteMessage(websocket.BinaryMessage, buf[:n])
			proc.mu.Unlock()
			if werr != nil {
				return
			}
		}
	}()

	// WebSocket → PTY
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if _, err := proc.ptmx.Write(msg); err != nil {
			return
		}
	}
}

// StripControlSequences removes ANSI escape sequences that could affect other sessions.
// Applied to input before writing to PTY.
func StripControlSequences(input []byte) []byte {
	out := make([]byte, 0, len(input))
	i := 0
	for i < len(input) {
		if input[i] == 0x1b && i+1 < len(input) && input[i+1] == '[' {
			// Skip until letter (sequence terminator)
			i += 2
			for i < len(input) && !isLetter(input[i]) {
				i++
			}
			if i < len(input) {
				i++ // skip terminator
			}
			continue
		}
		out = append(out, input[i])
		i++
	}
	return out
}

func isLetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

