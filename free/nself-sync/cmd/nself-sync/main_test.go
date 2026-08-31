package main

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/nself-sync/internal/store"
)

// fakeSnapStore implements store.SnapshotStore for tests.
type fakeSnapStore struct {
	rows      []store.SnapshotRow
	cursor    store.SnapshotCursor
	err       error
	gotUser   string
	gotWall   int64
	gotLam    int64
	failAfter int // -1 = never; 0 = before first row
}

func (f *fakeSnapStore) StreamSnapshot(ctx context.Context, userID string, cursorWall, cursorLam int64, fn func(store.SnapshotRow) error) (store.SnapshotCursor, error) {
	f.gotUser = userID
	f.gotWall = cursorWall
	f.gotLam = cursorLam
	if f.err != nil {
		return store.SnapshotCursor{}, f.err
	}
	for i, r := range f.rows {
		// Honor delta filter: skip rows at-or-below the cursor.
		if r.HLCWallMs < cursorWall || (r.HLCWallMs == cursorWall && r.HLCLamport <= cursorLam) {
			continue
		}
		if f.failAfter >= 0 && i >= f.failAfter {
			return store.SnapshotCursor{}, errors.New("mid-stream db failure")
		}
		if err := fn(r); err != nil {
			return store.SnapshotCursor{}, err
		}
	}
	return f.cursor, nil
}

// signToken builds an HS256 JWT with the given claims and secret. Used to
// exercise the auth path without depending on an external signing library.
func signToken(t *testing.T, secret []byte, claims map[string]any) string {
	t.Helper()
	hdr := map[string]string{"alg": "HS256", "typ": "JWT"}
	hb, _ := json.Marshal(hdr)
	pb, _ := json.Marshal(claims)
	enc := base64.RawURLEncoding.EncodeToString
	signing := enc(hb) + "." + enc(pb)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(signing))
	return signing + "." + enc(mac.Sum(nil))
}

func validToken(t *testing.T, secret []byte, userID string) string {
	t.Helper()
	return signToken(t, secret, map[string]any{
		"sub": userID,
		"did": "device-1",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
}

// parseNDJSON splits a response body into NDJSON frames as decoded maps.
func parseNDJSON(t *testing.T, body string) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("ndjson decode %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func sampleRows() []store.SnapshotRow {
	return []store.SnapshotRow{
		{
			EventID: "11111111-1111-1111-1111-111111111111", EntityType: "message", EntityID: "e1",
			Op: "insert", HLCWallMs: 1000, HLCLamport: 1, Payload: json.RawMessage(`{"k":1}`), SchemaVersion: 1,
		},
		{
			EventID: "22222222-2222-2222-2222-222222222222", EntityType: "message", EntityID: "e2",
			Op: "update", HLCWallMs: 2000, HLCLamport: 1, Payload: json.RawMessage(`{"k":2}`), SchemaVersion: 1,
		},
	}
}

// TestSnapshot_FullStream — no cursor supplied → all rows streamed + end frame.
func TestSnapshot_FullStream(t *testing.T) {
	secret := []byte("test-secret")
	fake := &fakeSnapStore{
		rows:      sampleRows(),
		cursor:    store.SnapshotCursor{HLCWallMs: 2000, HLCLamport: 1},
		failAfter: -1,
	}
	s := &server{snap: fake, secret: secret}

	req := httptest.NewRequest(http.MethodGet, "/sync/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, secret, "user-42"))
	rr := httptest.NewRecorder()
	s.handleSnapshot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("content-type: got %q, want application/x-ndjson", ct)
	}

	frames := parseNDJSON(t, rr.Body.String())
	if len(frames) != 3 {
		t.Fatalf("frame count: got %d, want 3 (2 rows + end); frames: %+v", len(frames), frames)
	}
	if frames[0]["type"] != "row" || frames[1]["type"] != "row" {
		t.Errorf("first two frames must be rows: %+v", frames)
	}
	end := frames[2]
	if end["type"] != "end" {
		t.Errorf("last frame type: got %v, want end", end["type"])
	}
	cur, ok := end["cursor"].(map[string]any)
	if !ok {
		t.Fatalf("end cursor missing or wrong shape: %+v", end)
	}
	if fmt.Sprint(cur["hlc_wall_ms"]) != "2000" || fmt.Sprint(cur["hlc_lamport"]) != "1" {
		t.Errorf("cursor: got %+v, want wall=2000 lam=1", cur)
	}
	if fake.gotUser != "user-42" {
		t.Errorf("store invoked with user_id=%q, want user-42", fake.gotUser)
	}
}

// TestSnapshot_DeltaWithCursor — cursor at first row's HLC → only second row emitted.
func TestSnapshot_DeltaWithCursor(t *testing.T) {
	secret := []byte("test-secret")
	fake := &fakeSnapStore{
		rows:      sampleRows(),
		cursor:    store.SnapshotCursor{HLCWallMs: 2000, HLCLamport: 1},
		failAfter: -1,
	}
	s := &server{snap: fake, secret: secret}

	req := httptest.NewRequest(http.MethodGet, "/sync/snapshot?cursor_wall_ms=1000&cursor_lamport=1", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, secret, "user-42"))
	rr := httptest.NewRecorder()
	s.handleSnapshot(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	if fake.gotWall != 1000 || fake.gotLam != 1 {
		t.Errorf("store cursor: got (%d,%d), want (1000,1)", fake.gotWall, fake.gotLam)
	}
	frames := parseNDJSON(t, rr.Body.String())
	// Expect: 1 row (the one at 2000,1) + end
	rowFrames := 0
	for _, f := range frames {
		if f["type"] == "row" {
			rowFrames++
		}
	}
	if rowFrames != 1 {
		t.Errorf("delta should emit 1 row; got %d, frames=%+v", rowFrames, frames)
	}
}

// TestSnapshot_Unauthorized — missing Authorization header → 401, no stream.
func TestSnapshot_Unauthorized(t *testing.T) {
	secret := []byte("test-secret")
	fake := &fakeSnapStore{failAfter: -1}
	s := &server{snap: fake, secret: secret}

	req := httptest.NewRequest(http.MethodGet, "/sync/snapshot", nil)
	rr := httptest.NewRecorder()
	s.handleSnapshot(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", rr.Code)
	}
	if fake.gotUser != "" {
		t.Error("store must not be invoked when auth fails")
	}
}

// TestSnapshot_BadJWT — bearer header present but signed with wrong secret → 401.
func TestSnapshot_BadJWT(t *testing.T) {
	secret := []byte("test-secret")
	fake := &fakeSnapStore{failAfter: -1}
	s := &server{snap: fake, secret: secret}

	bogus := validToken(t, []byte("wrong-secret"), "user-42")
	req := httptest.NewRequest(http.MethodGet, "/sync/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+bogus)
	rr := httptest.NewRecorder()
	s.handleSnapshot(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d, want 401", rr.Code)
	}
}

// TestSnapshot_DBError — store returns error before any row → error frame
// emitted on the stream (headers already flushed; cannot change status).
func TestSnapshot_DBError(t *testing.T) {
	secret := []byte("test-secret")
	fake := &fakeSnapStore{
		err:       errors.New("connection refused"),
		failAfter: -1,
	}
	s := &server{snap: fake, secret: secret}

	req := httptest.NewRequest(http.MethodGet, "/sync/snapshot", nil)
	req.Header.Set("Authorization", "Bearer "+validToken(t, secret, "user-42"))
	rr := httptest.NewRecorder()
	s.handleSnapshot(rr, req)

	// Status was already written 200 before the error surfaced — that's the
	// streaming contract. The client sees an error frame in the body.
	frames := parseNDJSON(t, rr.Body.String())
	gotErr := false
	for _, f := range frames {
		if f["type"] == "error" {
			gotErr = true
		}
	}
	if !gotErr {
		t.Errorf("expected error frame, got frames: %+v", frames)
	}
}

// --- /sync/ack handler tests (T04) ---
//
// The ack tests use an in-memory CursorStore (store.MemCursorStore) so the
// full HTTP pipeline runs without Postgres. They cover the documented status
// codes in handleAck's contract.

// newAckServer wires a server with a MemCursorStore that has one pre-seeded
// device (deviceID owned by userID). Returns the server, the store handle
// (for assertions), the secret, and a default valid JWT for that pair.
func newAckServer(t *testing.T, userID, deviceID string) (*server, *store.MemCursorStore, []byte, string) {
	t.Helper()
	secret := []byte("ack-test-secret")
	mem := store.NewMemCursorStore()
	mem.SetDevice(deviceID, userID)
	srv := &server{cursors: mem, secret: secret}
	tok := signToken(t, secret, map[string]any{
		"sub": userID,
		"did": deviceID,
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	return srv, mem, secret, tok
}

// doAck submits a POST /sync/ack request and returns the response recorder.
func doAck(t *testing.T, h http.HandlerFunc, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf strings.Builder
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("doAck encode: %v", err)
		}
		buf.Write(b)
	}
	req := httptest.NewRequest(http.MethodPost, "/sync/ack", strings.NewReader(buf.String()))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

// TestAck_AdvancesCursor — happy path: fresh device acked at (10, 0).
func TestAck_AdvancesCursor(t *testing.T) {
	srv, mem, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	rr := doAck(t, srv.handleAck, tok, ackRequest{
		DeviceID: "device-aaa", HLCWallMs: 10, HLCLamport: 0,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp ackResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Cursor.WallMs != 10 || resp.Cursor.Lamport != 0 {
		t.Errorf("cursor in response: got %+v", resp.Cursor)
	}
	if got := mem.Get("device-aaa"); got.WallMs != 10 || got.Lamport != 0 {
		t.Errorf("store: got %+v", got)
	}
}

// TestAck_RegressionIsNoop — re-acking older cursor leaves stored unchanged.
func TestAck_RegressionIsNoop(t *testing.T) {
	srv, mem, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	if rr := doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: 50, HLCLamport: 3}); rr.Code != 200 {
		t.Fatalf("seed: %d %s", rr.Code, rr.Body.String())
	}
	rr := doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: 10, HLCLamport: 0})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp ackResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Cursor.WallMs != 50 || resp.Cursor.Lamport != 3 {
		t.Errorf("regression must return existing cursor, got %+v", resp.Cursor)
	}
	if got := mem.Get("device-aaa"); got.WallMs != 50 || got.Lamport != 3 {
		t.Errorf("store regressed: %+v", got)
	}
}

// TestAck_Idempotent — re-acking the same cursor returns the same value.
func TestAck_Idempotent(t *testing.T) {
	srv, _, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	body := ackRequest{DeviceID: "device-aaa", HLCWallMs: 42, HLCLamport: 7}
	rr1 := doAck(t, srv.handleAck, tok, body)
	rr2 := doAck(t, srv.handleAck, tok, body)
	if rr1.Code != 200 || rr2.Code != 200 {
		t.Fatalf("idempotent acks: %d / %d", rr1.Code, rr2.Code)
	}
	var r1, r2 ackResponse
	_ = json.NewDecoder(rr1.Body).Decode(&r1)
	_ = json.NewDecoder(rr2.Body).Decode(&r2)
	if r1.Cursor != r2.Cursor {
		t.Errorf("idempotent diverged: %+v vs %+v", r1.Cursor, r2.Cursor)
	}
}

// TestAck_DeviceMismatch403 — body device_id != JWT did → 403, store not hit.
func TestAck_DeviceMismatch403(t *testing.T) {
	srv, mem, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	rr := doAck(t, srv.handleAck, tok, ackRequest{
		DeviceID: "device-stolen", HLCWallMs: 99, HLCLamport: 0,
	})
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := mem.Get("device-aaa"); got.WallMs != 0 || got.Lamport != 0 {
		t.Errorf("legit device must not be touched on mismatch: %+v", got)
	}
}

// TestAck_InvalidJWT401 — signature mismatch → 401.
func TestAck_InvalidJWT401(t *testing.T) {
	srv, _, _, _ := newAckServer(t, "user-aaa", "device-aaa")
	bad := signToken(t, []byte("different-secret-for-forgery"), map[string]any{
		"sub": "user-aaa", "did": "device-aaa",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	rr := doAck(t, srv.handleAck, bad, ackRequest{DeviceID: "device-aaa", HLCWallMs: 1, HLCLamport: 0})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAck_MissingAuth401 — no Authorization header → 401.
func TestAck_MissingAuth401(t *testing.T) {
	srv, _, _, _ := newAckServer(t, "user-aaa", "device-aaa")
	rr := doAck(t, srv.handleAck, "", ackRequest{DeviceID: "device-aaa", HLCWallMs: 1})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAck_ExpiredJWT401 — exp in past → 401.
func TestAck_ExpiredJWT401(t *testing.T) {
	srv, _, secret, _ := newAckServer(t, "user-aaa", "device-aaa")
	expired := signToken(t, secret, map[string]any{
		"sub": "user-aaa", "did": "device-aaa",
		"exp": time.Now().Add(-1 * time.Hour).Unix(),
	})
	rr := doAck(t, srv.handleAck, expired, ackRequest{DeviceID: "device-aaa", HLCWallMs: 1})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// TestAck_BadJSON400 — malformed body → 400.
func TestAck_BadJSON400(t *testing.T) {
	srv, _, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	req := httptest.NewRequest(http.MethodPost, "/sync/ack", strings.NewReader("{not json"))
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleAck(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// TestAck_NegativeCursor400 — negative HLC fields rejected.
func TestAck_NegativeCursor400(t *testing.T) {
	srv, _, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	rr := doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: -1})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestAck_UnknownDevice404 — JWT did claim names an unregistered device.
func TestAck_UnknownDevice404(t *testing.T) {
	srv, _, secret, _ := newAckServer(t, "user-aaa", "device-aaa")
	// Build a JWT whose did claim points at an unregistered device, so the
	// body-vs-claim equality check passes and the request reaches the store.
	tok := signToken(t, secret, map[string]any{
		"sub": "user-aaa", "did": "device-unregistered",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	})
	rr := doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-unregistered", HLCWallMs: 1})
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestAck_LamportAdvance — same wall_ms, larger lamport advances; smaller
// lamport is a no-op.
func TestAck_LamportAdvance(t *testing.T) {
	srv, mem, _, tok := newAckServer(t, "user-aaa", "device-aaa")
	_ = doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: 100, HLCLamport: 5})
	rr := doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: 100, HLCLamport: 9})
	if rr.Code != http.StatusOK {
		t.Fatalf("advance: %d", rr.Code)
	}
	if got := mem.Get("device-aaa"); got.WallMs != 100 || got.Lamport != 9 {
		t.Errorf("advance failed: %+v", got)
	}
	_ = doAck(t, srv.handleAck, tok, ackRequest{DeviceID: "device-aaa", HLCWallMs: 100, HLCLamport: 2})
	if got := mem.Get("device-aaa"); got.WallMs != 100 || got.Lamport != 9 {
		t.Errorf("lamport regression must be noop: %+v", got)
	}
}

// TestBearerToken — verifies header parsing rejects malformed inputs.
func TestBearerToken(t *testing.T) {
	cases := []struct {
		name string
		hdr  string
		want string
	}{
		{"empty", "", ""},
		{"no prefix", "abc.def.ghi", ""},
		{"wrong scheme", "Basic xyz", ""},
		{"bearer only", "Bearer ", ""},
		{"valid", "Bearer abc.def.ghi", "abc.def.ghi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.hdr != "" {
				r.Header.Set("Authorization", tc.hdr)
			}
			if got := bearerToken(r); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
