package sdk

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// TestGenerateIdentity verifies that GenerateIdentity produces a non-nil
// identity with properly sized Ed25519 keys.
func TestGenerateIdentity(t *testing.T) {
	id, err := GenerateIdentity("test-plugin")
	if err != nil {
		t.Fatalf("GenerateIdentity returned error: %v", err)
	}
	if id == nil {
		t.Fatal("GenerateIdentity returned nil identity")
	}
	if id.PluginName != "test-plugin" {
		t.Errorf("PluginName = %q, want %q", id.PluginName, "test-plugin")
	}
	if len(id.PublicKey) != 32 {
		t.Errorf("PublicKey len = %d, want 32", len(id.PublicKey))
	}
	if len(id.privateKey) != 64 {
		t.Errorf("privateKey len = %d, want 64", len(id.privateKey))
	}
	if id.PublicKeyBase64() == "" {
		t.Error("PublicKeyBase64 returned empty string")
	}
}

// TestSignAndVerify signs a request and verifies that VerifySignedRequest
// accepts it using the sender's public key.
func TestSignAndVerify(t *testing.T) {
	id, err := GenerateIdentity("sender-plugin")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/api/data", nil)
	id.SignRequest(req)

	if req.Header.Get("X-Plugin-Id") != "sender-plugin" {
		t.Errorf("X-Plugin-Id = %q, want %q", req.Header.Get("X-Plugin-Id"), "sender-plugin")
	}
	if req.Header.Get("X-Plugin-Timestamp") == "" {
		t.Error("X-Plugin-Timestamp header is missing")
	}
	if req.Header.Get("X-Plugin-Signature") == "" {
		t.Error("X-Plugin-Signature header is missing")
	}

	if err := VerifySignedRequest(req, id.PublicKeyBase64()); err != nil {
		t.Errorf("VerifySignedRequest returned unexpected error: %v", err)
	}
}

// TestReplayRejection verifies that a request with a timestamp older than
// 5 minutes is rejected by VerifySignedRequest.
func TestReplayRejection(t *testing.T) {
	id, err := GenerateIdentity("replay-plugin")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	req, _ := http.NewRequest(http.MethodPost, "/api/action", nil)

	// Manually construct a stale timestamp (6 minutes ago).
	staleTS := strconv.FormatInt(time.Now().Add(-6*time.Minute).Unix(), 10)
	msg := signatureMessage("replay-plugin", staleTS, http.MethodPost, "/api/action")
	sig := id.Sign(msg)

	req.Header.Set("X-Plugin-Id", "replay-plugin")
	req.Header.Set("X-Plugin-Timestamp", staleTS)
	req.Header.Set("X-Plugin-Signature", sig)

	if err := VerifySignedRequest(req, id.PublicKeyBase64()); err == nil {
		t.Error("VerifySignedRequest should have rejected a stale timestamp, but returned nil")
	}
}

// TestTamperedRequest verifies that modifying the request path after signing
// causes VerifySignedRequest to return an error.
func TestTamperedRequest(t *testing.T) {
	id, err := GenerateIdentity("tamper-plugin")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	// Sign the request against /api/original.
	req, _ := http.NewRequest(http.MethodGet, "/api/original", nil)
	id.SignRequest(req)

	// Tamper: change the path after signing.
	req.URL.Path = "/api/tampered"

	if err := VerifySignedRequest(req, id.PublicKeyBase64()); err == nil {
		t.Error("VerifySignedRequest should have rejected a tampered path, but returned nil")
	}
}

// TestRequirePluginSignaturePassthrough verifies the middleware passes through
// when no trusted key env var is configured (graceful degradation mode).
func TestRequirePluginSignaturePassthrough(t *testing.T) {
	// Ensure the env var is absent (it shouldn't be set in CI, but be explicit).
	t.Setenv("PLUGIN_TRUSTED_KEY_SENDER_PLUGIN", "")

	reached := false
	handler := RequirePluginSignature("sender-plugin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest(http.MethodGet, "/api/test", nil)
	// No signature headers — should pass through because env var is empty.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Error("middleware blocked the request, expected passthrough when no trusted key is configured")
	}
}

// TestRequirePluginSignatureValid verifies the middleware accepts a correctly
// signed request when the trusted key env var is set.
func TestRequirePluginSignatureValid(t *testing.T) {
	id, err := GenerateIdentity("auth-plugin")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	t.Setenv("PLUGIN_TRUSTED_KEY_AUTH_PLUGIN", id.PublicKeyBase64())

	reached := false
	handler := RequirePluginSignature("auth-plugin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest(http.MethodGet, "/api/secured", nil)
	id.SignRequest(req)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !reached {
		t.Errorf("middleware rejected a valid signed request; status=%d", rr.Code)
	}
}

// TestRequirePluginSignatureInvalid verifies the middleware rejects a request
// with a bad signature when the trusted key env var is set.
func TestRequirePluginSignatureInvalid(t *testing.T) {
	// Generate two identities — sign with one, verify with the other's key.
	signer, _ := GenerateIdentity("bad-signer")
	trusted, _ := GenerateIdentity("trusted-plugin")

	t.Setenv("PLUGIN_TRUSTED_KEY_BAD_SIGNER", trusted.PublicKeyBase64())

	handler := RequirePluginSignature("bad-signer")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req, _ := http.NewRequest(http.MethodGet, "/api/secured", nil)
	// Sign with signer's private key, but middleware will verify against trusted's public key.
	signer.SignRequest(req)

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("middleware returned %d, want %d", rr.Code, http.StatusUnauthorized)
	}
}
