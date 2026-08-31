package main

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/nself-org/nself-sync/internal/auth"
	"github.com/nself-org/nself-sync/internal/store"
	"github.com/nself-org/nself-sync/internal/ws"
)

const ListenAddr = "127.0.0.1:3844"

// server holds wired dependencies. Globals are avoided so tests can build
// per-test instances with fakes.
type server struct {
	snap    store.SnapshotStore
	cursors store.CursorStore
	ops     store.OperationStore
	secret  []byte
}

// snapStoreRef is set at boot in main() and read by free-function handlers
// during the gradual migration to method handlers. Tests construct their own
// server{} and call its methods directly — they never read this ref.
var snapStoreRef atomic.Value // store.SnapshotStore

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatalf("DATABASE_URL not set")
	}
	st, err := store.New(ctx, dsn)
	if err != nil {
		log.Fatalf("store init: %v", err)
	}
	defer st.Close()
	snapStoreRef.Store(store.SnapshotStore(st))

	secret, err := auth.SecretFromEnv()
	if err != nil {
		log.Fatalf("auth secret: %v", err)
	}
	srvDeps := &server{snap: st, cursors: st, ops: st, secret: secret}

	// WebSocket subscribe factory: one dedicated pg conn per WS session.
	// The factory adapter bridges *store.Listener (pgx.Notification) to
	// ws.Listener (transport-neutral Notification).
	subscribeFactory := func(ctx context.Context) (ws.Listener, error) {
		l, err := st.NewListener(ctx)
		if err != nil {
			return nil, err
		}
		return &pgListenerAdapter{l: l}, nil
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Route("/sync", func(r chi.Router) {
		r.Post("/push", srvDeps.handlePush)
		r.Post("/pull", handlePull)
		r.Post("/ack", srvDeps.handleAck)
		r.Get("/subscribe", ws.Handler(secret, subscribeFactory))
		r.Get("/snapshot", srvDeps.handleSnapshot)
	})

	srv := &http.Server{Addr: ListenAddr, Handler: r}
	go func() {
		log.Printf("nself-sync listening on %s", ListenAddr)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	srv.Shutdown(shutdownCtx)
}

// pgListenerAdapter bridges *store.Listener (which uses pgconn.Notification
// per pgx v5) to the transport-neutral ws.Listener interface.
type pgListenerAdapter struct {
	l *store.Listener
}

func (a *pgListenerAdapter) Listen(ctx context.Context, channel string) error {
	return a.l.Listen(ctx, channel)
}

func (a *pgListenerAdapter) WaitForNotification(ctx context.Context) (*ws.Notification, error) {
	n, err := a.l.WaitForNotification(ctx)
	if err != nil {
		return nil, err
	}
	if n == nil {
		return nil, nil
	}
	return &ws.Notification{Channel: n.Channel, Payload: n.Payload}, nil
}

func (a *pgListenerAdapter) EventsSince(ctx context.Context, userID string, cur store.Cursor, limit int) ([]store.SyncEvent, error) {
	return a.l.EventsSince(ctx, userID, cur, limit)
}

func (a *pgListenerAdapter) Close(ctx context.Context) error {
	return a.l.Close(ctx)
}

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
//   1. JWT auth (HS256, mandatory exp/user_id/device_id).
//   2. JSON parse + bounded body (max 1 MiB).
//   3. device_id binding — body device_id must match JWT did.
//   4. For each event:
//        a. Verify event.user_id matches JWT sub (V04-F02 hardening — client
//           is rejected if it claims a different user_id even though the
//           server-side persisted value comes from JWT regardless).
//        b. Validate op is insert|update|delete.
//        c. Fetch the device's Ed25519 public key.
//        d. Rebuild the canonical signing material and verify the signature.
//        e. Insert (idempotent on event_id) using user_id from the JWT.
//   5. Return per-event results plus a flat "acks" list (v1.1.0 compat).
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

// canonicalSigningMaterial rebuilds the byte string the device signed when it
// produced the operation. Mirrors `nclaw/core/src/sync/sign.rs::signing_material`
// after V04-F02 (user_id appended) + V04-F03 (deterministic field ordering).
//
// Layout (length-prefixed where strings/blobs appear; little-endian for ints):
//
//	event_id (uuid bytes) || entity_type (len:u32, bytes) || entity_id (uuid bytes) ||
//	op (u8: 0=insert,1=update,2=delete) ||
//	hlc_wall_ms (i64 LE) || hlc_lamport (u64 LE) || hlc_device_id (uuid bytes) ||
//	user_id (uuid bytes — V04-F02) ||
//	device_id (uuid bytes) ||
//	tenant_id (1 byte: 0=absent, 1=present || uuid bytes) ||
//	schema_version (i32 LE) ||
//	payload (canonical-JSON bytes, sorted keys; empty when nil)
//
// All UUID encoding uses the canonical 16-byte big-endian form. The function
// is pure — same inputs always produce the same bytes.
func canonicalSigningMaterial(ev pushRequestEvent, jwtUserID string) []byte {
	buf := make([]byte, 0, 256)
	buf = appendUUID(buf, ev.EventID)
	buf = appendLenString(buf, ev.EntityType)
	buf = appendUUID(buf, ev.EntityID)
	buf = append(buf, opByte(ev.Op))
	buf = appendInt64LE(buf, ev.HLCWallMs)
	buf = appendInt64LE(buf, ev.HLCLamport)
	buf = appendUUID(buf, ev.HLCDeviceID)
	buf = appendUUID(buf, jwtUserID)
	// device_id is implied by the JWT did claim — bind it into the material
	// so a stolen signature can't be replayed against a different device.
	buf = appendUUID(buf, ev.HLCDeviceID) // hlc device == operation device
	if ev.TenantID == "" {
		buf = append(buf, 0)
	} else {
		buf = append(buf, 1)
		buf = appendUUID(buf, ev.TenantID)
	}
	buf = appendInt32LE(buf, int32(ev.SchemaVersion))
	if len(ev.Payload) > 0 {
		buf = append(buf, canonicalJSON(ev.Payload)...)
	}
	return buf
}

func opByte(op string) byte {
	switch op {
	case "insert":
		return 0
	case "update":
		return 1
	case "delete":
		return 2
	default:
		return 0xff
	}
}

func appendUUID(dst []byte, s string) []byte {
	u, err := uuidFromString(s)
	if err != nil {
		// Append zero-UUID on parse failure — signature will not verify, the
		// operation is rejected upstream.
		return append(dst, make([]byte, 16)...)
	}
	return append(dst, u[:]...)
}

func appendLenString(dst []byte, s string) []byte {
	dst = appendInt32LE(dst, int32(len(s)))
	return append(dst, s...)
}

func appendInt64LE(dst []byte, v int64) []byte {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	return append(dst, b[:]...)
}

func appendInt32LE(dst []byte, v int32) []byte {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], uint32(v))
	return append(dst, b[:]...)
}

// uuidFromString parses a canonical-form UUID string into 16 bytes. Accepts
// both hyphenated (`xxxxxxxx-xxxx-...`) and bare hex forms.
func uuidFromString(s string) ([16]byte, error) {
	var out [16]byte
	clean := make([]byte, 0, 32)
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			continue
		}
		clean = append(clean, s[i])
	}
	if len(clean) != 32 {
		return out, fmt.Errorf("invalid uuid length: %d", len(clean))
	}
	if _, err := hex.Decode(out[:], clean); err != nil {
		return out, err
	}
	return out, nil
}

// canonicalJSON re-serializes a JSON value with sorted object keys so two
// devices producing the same logical payload end up with identical signing
// material. Mirrors the Rust client's V04-F03 behavior.
func canonicalJSON(raw json.RawMessage) []byte {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw
	}
	out, err := marshalCanonical(v)
	if err != nil {
		return raw
	}
	return out
}

// marshalCanonical walks the decoded value and emits JSON with sorted map keys.
// Maps preserve their UTF-8 string keys; arrays and scalars marshal via
// stdlib encoding/json. Numbers stay as json.Number where applicable.
func marshalCanonical(v any) ([]byte, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf := []byte{'{'}
		for i, k := range keys {
			if i > 0 {
				buf = append(buf, ',')
			}
			kb, err := json.Marshal(k)
			if err != nil {
				return nil, err
			}
			buf = append(buf, kb...)
			buf = append(buf, ':')
			vb, err := marshalCanonical(t[k])
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, '}')
		return buf, nil
	case []any:
		buf := []byte{'['}
		for i, el := range t {
			if i > 0 {
				buf = append(buf, ',')
			}
			vb, err := marshalCanonical(el)
			if err != nil {
				return nil, err
			}
			buf = append(buf, vb...)
		}
		buf = append(buf, ']')
		return buf, nil
	default:
		return json.Marshal(t)
	}
}

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
