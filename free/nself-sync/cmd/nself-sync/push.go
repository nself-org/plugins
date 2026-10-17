package main

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
)

// pushRequestEvent is one event in the wire-format push body. Mirrors the
// Rust nclaw core `EventEnvelope` after V04-F02 (user_id in signing material)
// and V04-F03 (canonical JSON). The server takes user_id from the JWT — the
// UserID field here is the client's claim and is verified to match.
type pushRequestEvent struct {
	EventID       string          `json:"event_id"`
	EntityType    string          `json:"entity_type"`
	EntityID      string          `json:"entity_id"`
	Op            string          `json:"op"`
	HLCWallMs     int64           `json:"hlc_wall_ms"`
	HLCLamport    int64           `json:"hlc_lamport"`
	HLCDeviceID   string          `json:"hlc_device_id"`
	UserID        string          `json:"user_id"`
	TenantID      string          `json:"tenant_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	SchemaVersion int             `json:"schema_version"`
	Signature     []byte          `json:"signature"`
}

// pushRequest is the JSON body posted to /sync/push. DeviceID is verified
// against the JWT did claim before any operation is processed; the cursor
// (if present) is informational only — operations are dedup'd by event_id.
type pushRequest struct {
	DeviceID      string             `json:"device_id"`
	Events        []pushRequestEvent `json:"events"`
	Cursor        *store.Cursor      `json:"cursor,omitempty"`
	SchemaVersion int                `json:"schema_version,omitempty"`
}

// pushResult is one entry in the push response. Status is "accepted" for a
// newly-inserted operation, "duplicate" for an idempotent re-push, or
// "rejected" with a Reason explaining why (invalid signature, op mismatch, etc).
type pushResult struct {
	EventID string `json:"event_id"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
}

// pushResponse is the JSON returned by /sync/push.
type pushResponse struct {
	Results []pushResult `json:"results"`
	// Acks mirrors v1.1.0 wire compatibility — list of accepted event_ids.
	Acks []string `json:"acks"`
}

// handlePush ingests a batched set of event envelopes from a signed-in device.
//
// Flow per request:
//  1. JWT auth (HS256, mandatory exp/user_id/device_id).
//  2. JSON parse + bounded body (max 1 MiB).
//  3. device_id binding — body device_id must match JWT did.
//  4. For each event:
//     a. Verify event.user_id matches JWT sub (V04-F02 hardening — client
//     is rejected if it claims a different user_id even though the
//     server-side persisted value comes from JWT regardless).
//     b. Validate op is insert|update|delete.
//     c. Fetch the device's Ed25519 public key.
//     d. Rebuild the canonical signing material and verify the signature.
//     e. Insert (idempotent on event_id) using user_id from the JWT.
//  5. Return per-event results plus a flat "acks" list (v1.1.0 compat).
//
// Error mapping (request-level): 400 invalid body / 401 invalid JWT /
// 403 device_id mismatch / 500 internal. Per-event errors do NOT fail the
// whole request — each event reports its own status.
func (s *server) handlePush(w http.ResponseWriter, r *http.Request) {
	tok := bearerToken(r)
	if tok == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing or malformed Authorization header")
		return
	}
	claims, err := auth.Verify(tok, s.secret)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// 1 MiB body cap. Push is small-batch; the Rust client batches ~32 events
	// at a time. Cap stops abuse without rejecting legitimate traffic.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	var req pushRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		// Body may have already been consumed past the limit — treat both
		// over-size and malformed as 400. The MaxBytesError type is in the
		// stdlib http package since Go 1.19.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.DeviceID == "" {
		writeJSONError(w, http.StatusBadRequest, "device_id is required")
		return
	}
	if req.DeviceID != claims.DeviceID {
		writeJSONError(w, http.StatusForbidden, "device_id does not match token")
		return
	}
	if len(req.Events) == 0 {
		// Empty batch is valid — client may be heartbeating.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pushResponse{Results: []pushResult{}, Acks: []string{}})
		return
	}

	// Fetch the device pubkey once — every event in the batch is signed by
	// the same device per the request contract.
	pubkey, err := s.ops.DevicePubkey(r.Context(), claims.UserID, claims.DeviceID)
	switch {
	case errors.Is(err, store.ErrDevicePubkeyNotFound):
		writeJSONError(w, http.StatusForbidden, "device not registered for this user")
		return
	case err != nil:
		log.Printf("nself-sync: push: pubkey lookup: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(pubkey) != ed25519.PublicKeySize {
		log.Printf("nself-sync: push: device pubkey wrong size: %d", len(pubkey))
		writeJSONError(w, http.StatusInternalServerError, "device key invalid")
		return
	}

	results := make([]pushResult, 0, len(req.Events))
	acks := make([]string, 0, len(req.Events))
	for _, ev := range req.Events {
		// 4a — server-side identity binding. Even though Insert uses the JWT
		// user_id for the persisted row, surfacing the mismatch in the
		// response helps clients catch impersonation bugs early.
		if ev.UserID != "" && ev.UserID != claims.UserID {
			results = append(results, pushResult{
				EventID: ev.EventID, Status: "rejected", Reason: "user_id mismatch",
			})
			continue
		}
		// 4b — op validation.
		if err := store.ValidateOp(ev.Op); err != nil {
			results = append(results, pushResult{
				EventID: ev.EventID, Status: "rejected", Reason: err.Error(),
			})
			continue
		}
		// 4d — signature verification against canonical material. The signing
		// material must include the JWT user_id (V04-F02) to prevent another
		// user from forging events.
		material := canonicalSigningMaterial(ev, claims.UserID)
		if !ed25519.Verify(pubkey, material, ev.Signature) {
			results = append(results, pushResult{
				EventID: ev.EventID, Status: "rejected", Reason: "invalid signature",
			})
			continue
		}
		// 4e — persist.
		row := store.PushEvent{
			EventID:       ev.EventID,
			EntityType:    ev.EntityType,
			EntityID:      ev.EntityID,
			Op:            ev.Op,
			HLCWallMs:     ev.HLCWallMs,
			HLCLamport:    ev.HLCLamport,
			HLCDeviceID:   ev.HLCDeviceID,
			UserID:        claims.UserID, // authoritative — never trust client field
			DeviceID:      claims.DeviceID,
			TenantID:      ev.TenantID,
			Payload:       ev.Payload,
			SchemaVersion: ev.SchemaVersion,
			Signature:     ev.Signature,
		}
		inserted, err := s.ops.Insert(r.Context(), row)
		if err != nil {
			log.Printf("nself-sync: push: insert event %s: %v", ev.EventID, err)
			results = append(results, pushResult{
				EventID: ev.EventID, Status: "rejected", Reason: "store error",
			})
			continue
		}
		if inserted {
			results = append(results, pushResult{EventID: ev.EventID, Status: "accepted"})
		} else {
			results = append(results, pushResult{EventID: ev.EventID, Status: "duplicate"})
		}
		acks = append(acks, ev.EventID)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(pushResponse{Results: results, Acks: acks})
}
