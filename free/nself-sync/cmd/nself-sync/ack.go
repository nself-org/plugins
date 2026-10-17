package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
)

// ackRequest is the JSON body posted to POST /sync/ack. DeviceID is verified
// against the JWT did claim before the request reaches the store layer.
// HLCWallMs + HLCLamport together form the cursor being acknowledged.
type ackRequest struct {
	DeviceID   string `json:"device_id"`
	HLCWallMs  int64  `json:"hlc_wall_ms"`
	HLCLamport int64  `json:"hlc_lamport"`
}

// ackResponse is the JSON returned on a successful ack. Cursor reflects the
// post-merge server value — clients can use it to detect another device's
// concurrent advance past the value they sent.
type ackResponse struct {
	Cursor store.Cursor `json:"cursor"`
}

// handleAck records that the calling device has applied operations up to a
// given HLC cursor. The server advances the device's stored cursor
// monotonically so future /pull or /subscribe calls resume from there.
//
// POST /sync/ack
//
//	Authorization: Bearer <jwt>
//	Body: {"device_id": "...", "hlc_wall_ms": <int>, "hlc_lamport": <int>}
//
// Status codes:
//   - 200 with {"cursor": {...}} on success (cursor is the post-merge value)
//   - 400 on malformed body or negative cursor fields
//   - 401 when Authorization is missing or JWT invalid/expired
//   - 403 when body device_id does not match JWT did claim, or when the
//     token's user does not own the device
//   - 404 when the device is unknown or revoked
//   - 500 on unexpected DB errors
//
// Idempotent: re-acking the same or a smaller cursor returns the existing
// stored value unchanged (the underlying UPSERT keeps the lexicographic max).
func (s *server) handleAck(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if tok == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing bearer token")
		return
	}
	claims, err := auth.Verify(tok, s.secret)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	var req ackRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.DeviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "device_id required")
		return
	}
	if req.HLCWallMs < 0 || req.HLCLamport < 0 {
		writeJSONError(w, http.StatusBadRequest, "cursor fields must be non-negative")
		return
	}
	// Defense-in-depth — JWT binds the request to a device; reject mismatch
	// even though the store enforces ownership again.
	if req.DeviceID != claims.DeviceID {
		writeJSONError(w, http.StatusForbidden, "device_id does not match token")
		return
	}

	requested := store.Cursor{WallMs: req.HLCWallMs, Lamport: req.HLCLamport}
	stored, err := s.cursors.AdvanceCursor(r.Context(), claims.UserID, claims.DeviceID, requested)
	switch {
	case errors.Is(err, store.ErrDeviceNotFound):
		writeJSONError(w, http.StatusNotFound, "device not registered")
		return
	case errors.Is(err, store.ErrDeviceUserMismatch):
		writeJSONError(w, http.StatusForbidden, "device does not belong to user")
		return
	case err != nil:
		log.Printf("nself-sync: ack: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ackResponse{Cursor: stored})
}

// writeJSONError emits a JSON error body with the given status code. Used by
// the ack handler so test clients can decode error responses uniformly.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
