// Tests for handlePush. Uses an in-memory OperationStore so no DB is needed.
//
// Covers happy path + all six documented error paths:
//   - missing Authorization header → 401
//   - invalid JWT → 401
//   - malformed JSON body → 400
//   - body device_id != JWT did → 403
//   - device not registered → 403
//   - per-event: user_id mismatch → 200 with status=rejected
//   - per-event: invalid Ed25519 signature → 200 with status=rejected
//   - per-event: invalid op → 200 with status=rejected
//   - idempotency: re-push same event_id → status=duplicate
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/nself-sync/internal/store"
)

// ---------- in-memory OperationStore fake ----------

type fakeOps struct {
	mu          sync.Mutex
	pubkeys     map[string][]byte // userID|deviceID -> pubkey
	inserted    map[string]store.PushEvent
	insertCalls int
}

func newFakeOps() *fakeOps {
	return &fakeOps{
		pubkeys:  make(map[string][]byte),
		inserted: make(map[string]store.PushEvent),
	}
}

func (f *fakeOps) setDevice(userID, deviceID string, pub []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pubkeys[userID+"|"+deviceID] = pub
}

func (f *fakeOps) DevicePubkey(ctx context.Context, userID, deviceID string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	pub, ok := f.pubkeys[userID+"|"+deviceID]
	if !ok {
		return nil, store.ErrDevicePubkeyNotFound
	}
	return pub, nil
}

func (f *fakeOps) Insert(ctx context.Context, ev store.PushEvent) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.insertCalls++
	if err := store.ValidateOp(ev.Op); err != nil {
		return false, err
	}
	if _, exists := f.inserted[ev.EventID]; exists {
		return false, nil
	}
	f.inserted[ev.EventID] = ev
	return true, nil
}

// ---------- test helpers ----------

const (
	pushTestUserID   = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	pushTestDeviceID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

// pushTestJWT mints a valid HS256 JWT for the given user + device using the
// shared signToken helper from main_test.go.
func pushTestJWT(t *testing.T, secret []byte, userID, deviceID string, expOffset time.Duration) string {
	t.Helper()
	return signToken(t, secret, map[string]any{
		"sub": userID,
		"did": deviceID,
		"exp": time.Now().Add(expOffset).Unix(),
	})
}

// signedEvent constructs a signed pushRequestEvent. signedAsUserID is the
// user_id mixed into the canonical signing material — pass a different value
// to simulate a user_id forgery attempt where the signature is valid for a
// different user.
func signedEvent(t *testing.T, priv ed25519.PrivateKey, signedAsUserID, claimedUserID, deviceID, eventID, payload string) pushRequestEvent {
	t.Helper()
	ev := pushRequestEvent{
		EventID:       eventID,
		EntityType:    "Message",
		EntityID:      "11111111-1111-1111-1111-111111111111",
		Op:            "insert",
		HLCWallMs:     1700000000000,
		HLCLamport:    1,
		HLCDeviceID:   deviceID,
		UserID:        claimedUserID,
		Payload:       json.RawMessage(payload),
		SchemaVersion: 1,
	}
	material := canonicalSigningMaterial(ev, signedAsUserID)
	ev.Signature = ed25519.Sign(priv, material)
	return ev
}

func newPushServer(t *testing.T) (*server, *fakeOps, []byte) {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatalf("rand secret: %v", err)
	}
	ops := newFakeOps()
	return &server{ops: ops, secret: secret}, ops, secret
}

func doPush(t *testing.T, s *server, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if r, ok := body.(string); ok {
		buf.WriteString(r)
	} else {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/sync/push", &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handlePush(rr, req)
	return rr
}

// ---------- handlePush tests ----------

func TestPush_HappyPath_AcceptsAndInserts(t *testing.T) {
	t.Parallel()
	s, ops, secret := newPushServer(t)
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	ops.setDevice(pushTestUserID, pushTestDeviceID, pub)
	ev := signedEvent(t, priv, pushTestUserID, pushTestUserID, pushTestDeviceID,
		"cccccccc-cccc-cccc-cccc-cccccccccccc", `{"hello":"world"}`)

	rr := doPush(t, s, pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour),
		pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{ev}})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var resp pushResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode resp: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Status != "accepted" {
		t.Errorf("results: %+v", resp.Results)
	}
	if len(resp.Acks) != 1 || resp.Acks[0] != ev.EventID {
		t.Errorf("acks: %+v", resp.Acks)
	}
	if ops.insertCalls != 1 {
		t.Errorf("insertCalls=%d want 1", ops.insertCalls)
	}
}

func TestPush_Idempotent_SecondPushReturnsDuplicate(t *testing.T) {
	t.Parallel()
	s, ops, secret := newPushServer(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ops.setDevice(pushTestUserID, pushTestDeviceID, pub)
	ev := signedEvent(t, priv, pushTestUserID, pushTestUserID, pushTestDeviceID,
		"dddddddd-dddd-dddd-dddd-dddddddddddd", `{"k":"v"}`)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)

	wantStatuses := []string{"accepted", "duplicate"}
	for i, want := range wantStatuses {
		rr := doPush(t, s, tok, pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{ev}})
		if rr.Code != http.StatusOK {
			t.Fatalf("iter %d: status %d body %s", i, rr.Code, rr.Body.String())
		}
		var resp pushResponse
		_ = json.NewDecoder(rr.Body).Decode(&resp)
		if resp.Results[0].Status != want {
			t.Errorf("iter %d: status=%q want %q", i, resp.Results[0].Status, want)
		}
	}
}

func TestPush_MissingAuthHeader_401(t *testing.T) {
	t.Parallel()
	s, _, _ := newPushServer(t)
	rr := doPush(t, s, "", pushRequest{DeviceID: pushTestDeviceID})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d want 401", rr.Code)
	}
}

func TestPush_InvalidJWT_401(t *testing.T) {
	t.Parallel()
	s, _, _ := newPushServer(t)
	rr := doPush(t, s, "not.a.real.token", pushRequest{DeviceID: pushTestDeviceID})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d want 401", rr.Code)
	}
}

func TestPush_BadJSON_400(t *testing.T) {
	t.Parallel()
	s, _, secret := newPushServer(t)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, "{not valid json")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestPush_DeviceIDMismatch_403(t *testing.T) {
	t.Parallel()
	s, _, secret := newPushServer(t)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, pushRequest{DeviceID: "99999999-9999-9999-9999-999999999999"})
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestPush_DeviceNotRegistered_403(t *testing.T) {
	t.Parallel()
	s, _, secret := newPushServer(t)
	// No setDevice call; pubkey lookup fails.
	body := pushRequest{
		DeviceID: pushTestDeviceID,
		Events:   []pushRequestEvent{{EventID: "x", Op: "insert"}},
	}
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, body)
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
}

func TestPush_ForgedUserIDField_Rejected(t *testing.T) {
	t.Parallel()
	s, ops, secret := newPushServer(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ops.setDevice(pushTestUserID, pushTestDeviceID, pub)
	// Client claims a different user_id than the JWT subject.
	otherUser := "99999999-9999-9999-9999-999999999999"
	ev := signedEvent(t, priv, pushTestUserID, otherUser, pushTestDeviceID,
		"eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", `{}`)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{ev}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var resp pushResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "rejected" ||
		!strings.Contains(resp.Results[0].Reason, "user_id mismatch") {
		t.Errorf("expected user_id mismatch, got %+v", resp.Results)
	}
	if ops.insertCalls != 0 {
		t.Errorf("expected 0 inserts, got %d", ops.insertCalls)
	}
}

func TestPush_InvalidSignature_Rejected(t *testing.T) {
	t.Parallel()
	s, ops, secret := newPushServer(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ops.setDevice(pushTestUserID, pushTestDeviceID, pub)
	ev := signedEvent(t, priv, pushTestUserID, pushTestUserID, pushTestDeviceID,
		"77777777-7777-7777-7777-777777777777", `{"k":"v"}`)
	// Corrupt the signature.
	ev.Signature[0] ^= 0xff
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{ev}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d", rr.Code)
	}
	var resp pushResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "rejected" ||
		!strings.Contains(resp.Results[0].Reason, "signature") {
		t.Errorf("expected signature rejection, got %+v", resp.Results)
	}
	if ops.insertCalls != 0 {
		t.Errorf("expected 0 inserts on bad sig, got %d", ops.insertCalls)
	}
}

func TestPush_InvalidOp_Rejected(t *testing.T) {
	t.Parallel()
	s, ops, secret := newPushServer(t)
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	ops.setDevice(pushTestUserID, pushTestDeviceID, pub)
	ev := signedEvent(t, priv, pushTestUserID, pushTestUserID, pushTestDeviceID,
		"88888888-8888-8888-8888-888888888888", `{}`)
	ev.Op = "bogus"
	// Re-sign with the bogus op so we hit the op check, not the sig path.
	material := canonicalSigningMaterial(ev, pushTestUserID)
	ev.Signature = ed25519.Sign(priv, material)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{ev}})
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d body=%s", rr.Code, rr.Body.String())
	}
	var resp pushResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Results) != 1 || resp.Results[0].Status != "rejected" {
		t.Errorf("expected rejected, got %+v", resp.Results)
	}
	if ops.insertCalls != 0 {
		t.Errorf("expected 0 inserts on bad op, got %d", ops.insertCalls)
	}
}

func TestPush_EmptyBatch_OK(t *testing.T) {
	t.Parallel()
	s, _, secret := newPushServer(t)
	tok := pushTestJWT(t, secret, pushTestUserID, pushTestDeviceID, time.Hour)
	rr := doPush(t, s, tok, pushRequest{DeviceID: pushTestDeviceID, Events: []pushRequestEvent{}})
	if rr.Code != http.StatusOK {
		t.Fatalf("got %d want 200", rr.Code)
	}
	var resp pushResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if len(resp.Acks) != 0 || len(resp.Results) != 0 {
		t.Errorf("expected empty: %+v", resp)
	}
}

// ---------- canonical helpers ----------

func TestCanonicalJSON_SortsKeysDeterministically(t *testing.T) {
	t.Parallel()
	a := canonicalJSON(json.RawMessage(`{"b":1,"a":2,"c":3}`))
	b := canonicalJSON(json.RawMessage(`{"c":3,"a":2,"b":1}`))
	if string(a) != string(b) {
		t.Errorf("canonicalJSON differs:\n  a=%s\n  b=%s", a, b)
	}
}

func TestCanonicalSigningMaterial_PayloadOrderIndependent(t *testing.T) {
	t.Parallel()
	base := pushRequestEvent{
		EventID:     "33333333-3333-3333-3333-333333333333",
		EntityType:  "X",
		EntityID:    "44444444-4444-4444-4444-444444444444",
		Op:          "insert",
		HLCWallMs:   1,
		HLCLamport:  2,
		HLCDeviceID: pushTestDeviceID,
		UserID:      pushTestUserID,
	}
	a := base
	a.Payload = json.RawMessage(`{"x":1,"y":2}`)
	b := base
	b.Payload = json.RawMessage(`{"y":2,"x":1}`)
	if !bytes.Equal(canonicalSigningMaterial(a, pushTestUserID),
		canonicalSigningMaterial(b, pushTestUserID)) {
		t.Error("canonicalSigningMaterial varies with payload key order")
	}
}

// TestSigningMaterial_CrossLanguage locks the canonical signing material
// byte-for-byte against the Rust client in `nclaw/core/src/sync/sign.rs`.
//
// Drift between the two implementations means every Rust-signed event will
// be rejected by this server with "invalid signature". The Rust side
// asserts the same bytes in `nclaw/core/tests/cross_lang_sign_test.rs`
// using the same canonical envelope and the same expected hex string.
//
// Hotfix reference: P102 Wave 11 V04-F05 cross-language convergence.
func TestSigningMaterial_CrossLanguage(t *testing.T) {
	t.Parallel()

	ev := pushRequestEvent{
		EventID:       "22222222-2222-2222-2222-222222222222",
		EntityType:    "Note",
		EntityID:      "33333333-3333-3333-3333-333333333333",
		Op:            "insert",
		HLCWallMs:     1715626800000,
		HLCLamport:    17,
		HLCDeviceID:   "44444444-4444-4444-4444-444444444444",
		UserID:        "11111111-1111-1111-1111-111111111111",
		TenantID:      "",
		Payload:       json.RawMessage(`{"k":"v"}`),
		SchemaVersion: 1,
	}
	jwtUserID := "11111111-1111-1111-1111-111111111111"

	// Expected canonical bytes — must match Rust's
	// `signing_material()` output in `nclaw/core/src/sync/sign.rs`.
	// Layout (119 bytes total):
	//   event_id(16) | etype_len(4 i32 LE) | "Note"(4) | entity_id(16) |
	//   op(1=0x00 insert) | wall_ms(8 i64 LE) | lamport(8 u64 LE) |
	//   hlc_device_id(16) | user_id(16) | device_id(16) |
	//   tenant_flag(1=0x00 absent) | schema_version(4 i32 LE) | payload(9 canonical JSON)
	const expectedHex = "22222222222222222222222222222222040000004e6f74653333333333333333333333333333333300807353738f010000110000000000000044444444444444444444444444444444111111111111111111111111111111114444444444444444444444444444444400010000007b226b223a2276227d"
	expected, err := hexDecode(expectedHex)
	if err != nil {
		t.Fatalf("decode expected hex: %v", err)
	}

	got := canonicalSigningMaterial(ev, jwtUserID)
	if len(got) != 119 {
		t.Fatalf("expected 119-byte material, got %d", len(got))
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("cross-language signing material drift\n  want hex: %s\n  got hex:  %x",
			expectedHex, got)
	}
}

// hexDecode decodes a contiguous hex string (no whitespace) into bytes.
// Local helper to avoid importing encoding/hex into this large test file
// twice (already imported transitively via other tests, but kept self-
// contained for clarity).
func hexDecode(s string) ([]byte, error) {
	if len(s)%2 != 0 {
		return nil, errOddHex
	}
	out := make([]byte, len(s)/2)
	for i := 0; i < len(s); i += 2 {
		hi, err := nibble(s[i])
		if err != nil {
			return nil, err
		}
		lo, err := nibble(s[i+1])
		if err != nil {
			return nil, err
		}
		out[i/2] = hi<<4 | lo
	}
	return out, nil
}

func nibble(b byte) (byte, error) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', nil
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, nil
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, nil
	default:
		return 0, errBadHex
	}
}

var (
	errOddHex = &hexErr{msg: "odd-length hex string"}
	errBadHex = &hexErr{msg: "non-hex character"}
)

type hexErr struct{ msg string }

func (e *hexErr) Error() string { return e.msg }
