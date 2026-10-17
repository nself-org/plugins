package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
)

func handlePull(w http.ResponseWriter, r *http.Request) {
	// Real implementation pending (T04).
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"events":[],"has_more":false}`))
}

// handleSnapshot streams the authenticated user's materialized current state
// as NDJSON. Each line is one record; a trailing metadata frame carries the
// cursor for resumed pulls.
//
// GET /sync/snapshot
//
//	Authorization: Bearer <jwt>
//	?cursor_wall_ms=<int64>&cursor_lamport=<int64>   (optional — delta snapshot)
//
// Response: 200 OK, application/x-ndjson, chunked.
//
//	{"type":"row","data":<SnapshotRow>}\n
//	{"type":"row","data":<SnapshotRow>}\n
//	...
//	{"type":"end","cursor":{"hlc_wall_ms":<n>,"hlc_lamport":<n>}}\n
//
// Auth failures return 401 before any rows stream. DB failures mid-stream
// terminate with an `{"type":"error","message":"..."}` frame; the partial body
// is still chunked so clients see the error rather than a hung connection.
func (s *server) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if tok == "" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	claims, err := auth.Verify(tok, s.secret)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	cursorWall, _ := strconv.ParseInt(r.URL.Query().Get("cursor_wall_ms"), 10, 64)
	cursorLam, _ := strconv.ParseInt(r.URL.Query().Get("cursor_lamport"), 10, 64)

	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)

	type rowFrame struct {
		Type string            `json:"type"`
		Data store.SnapshotRow `json:"data"`
	}
	type endFrame struct {
		Type   string               `json:"type"`
		Cursor store.SnapshotCursor `json:"cursor"`
	}
	type errFrame struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	}

	emit := func(row store.SnapshotRow) error {
		if err := enc.Encode(rowFrame{Type: "row", Data: row}); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}

	cur, err := s.snap.StreamSnapshot(r.Context(), claims.UserID, cursorWall, cursorLam, emit)
	if err != nil {
		// Stream-side error after headers are flushed. Best-effort error frame.
		if errors.Is(err, context.Canceled) {
			return
		}
		_ = enc.Encode(errFrame{Type: "error", Message: "snapshot stream failed"})
		if flusher != nil {
			flusher.Flush()
		}
		return
	}

	_ = enc.Encode(endFrame{Type: "end", Cursor: cur})
	if flusher != nil {
		flusher.Flush()
	}
}

// bearerToken extracts the JWT from the Authorization header.
// Returns "" when missing or not bearer-formatted.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || h[:len(prefix)] != prefix {
		return ""
	}
	return h[len(prefix):]
}
