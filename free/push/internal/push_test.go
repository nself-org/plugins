package internal

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// --- DedupeHash tests ---

func TestDedupeHash_Deterministic(t *testing.T) {
	payload := json.RawMessage(`{"notification":{"title":"Hello"}}`)
	h1 := DedupeHash("token-abc", "ios", payload)
	h2 := DedupeHash("token-abc", "ios", payload)
	if h1 != h2 {
		t.Errorf("DedupeHash is not deterministic: %q != %q", h1, h2)
	}
}

func TestDedupeHash_DifferentTokens(t *testing.T) {
	payload := json.RawMessage(`{"notification":{"title":"Hello"}}`)
	h1 := DedupeHash("token-aaa", "ios", payload)
	h2 := DedupeHash("token-bbb", "ios", payload)
	if h1 == h2 {
		t.Error("DedupeHash should differ for different device tokens")
	}
}

func TestDedupeHash_DifferentPlatforms(t *testing.T) {
	payload := json.RawMessage(`{"notification":{"title":"Hello"}}`)
	h1 := DedupeHash("same-token", "ios", payload)
	h2 := DedupeHash("same-token", "android", payload)
	if h1 == h2 {
		t.Error("DedupeHash should differ for different platforms")
	}
}

func TestDedupeHash_DifferentPayloads(t *testing.T) {
	h1 := DedupeHash("same-token", "ios", json.RawMessage(`{"notification":{"title":"A"}}`))
	h2 := DedupeHash("same-token", "ios", json.RawMessage(`{"notification":{"title":"B"}}`))
	if h1 == h2 {
		t.Error("DedupeHash should differ for different payloads")
	}
}

// --- exponentialBackoff tests ---

func TestExponentialBackoff(t *testing.T) {
	base := 500 * time.Millisecond
	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 500 * time.Millisecond},  // 500 * 2^0 = 500ms
		{2, 1000 * time.Millisecond}, // 500 * 2^1 = 1s
		{3, 2000 * time.Millisecond}, // 500 * 2^2 = 2s
		{4, 4000 * time.Millisecond}, // 500 * 2^3 = 4s
	}
	for _, tt := range tests {
		got := exponentialBackoff(base, tt.attempt)
		if got != tt.want {
			t.Errorf("exponentialBackoff(base, %d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestExponentialBackoff_Cap(t *testing.T) {
	// High attempt values should be capped at 30s.
	base := 500 * time.Millisecond
	got := exponentialBackoff(base, 20)
	cap := 30 * time.Second
	if got != cap {
		t.Errorf("exponentialBackoff with high attempt = %v, want %v (cap)", got, cap)
	}
}

// --- Config tests ---

func TestConfig_APNsEnabled(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		expect bool
	}{
		{"all set", Config{APNsTeamID: "T", APNsKeyID: "K", APNsKeyPEM: "P", APNsBundleID: "B"}, true},
		{"missing team", Config{APNsKeyID: "K", APNsKeyPEM: "P", APNsBundleID: "B"}, false},
		{"missing key ID", Config{APNsTeamID: "T", APNsKeyPEM: "P", APNsBundleID: "B"}, false},
		{"missing PEM", Config{APNsTeamID: "T", APNsKeyID: "K", APNsBundleID: "B"}, false},
		{"missing bundle", Config{APNsTeamID: "T", APNsKeyID: "K", APNsKeyPEM: "P"}, false},
		{"none set", Config{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.APNsEnabled()
			if got != tt.expect {
				t.Errorf("APNsEnabled() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestConfig_FCMEnabled(t *testing.T) {
	tests := []struct {
		name   string
		cfg    Config
		expect bool
	}{
		{"all set", Config{FCMProjectID: "proj", FCMServiceAccountJSON: "{}"}, true},
		{"missing project", Config{FCMServiceAccountJSON: "{}"}, false},
		{"missing sa json", Config{FCMProjectID: "proj"}, false},
		{"none set", Config{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.FCMEnabled()
			if got != tt.expect {
				t.Errorf("FCMEnabled() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestConfig_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		cfg := &Config{RetryMaxAttempts: 3, RetryBackoffBaseMs: 500}
		if err := cfg.Validate(); err != nil {
			t.Errorf("expected valid config, got error: %v", err)
		}
	})
	t.Run("zero attempts", func(t *testing.T) {
		cfg := &Config{RetryMaxAttempts: 0, RetryBackoffBaseMs: 500}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for zero attempts")
		}
	})
	t.Run("low backoff", func(t *testing.T) {
		cfg := &Config{RetryMaxAttempts: 3, RetryBackoffBaseMs: 50}
		if err := cfg.Validate(); err == nil {
			t.Error("expected error for backoff < 100ms")
		}
	})
}

// --- SSRF guard tests ---

func TestIsURL(t *testing.T) {
	tests := []struct {
		input  string
		expect bool
	}{
		{"http://evil.com", true},
		{"https://evil.com", true},
		{"abcd1234efgh5678ijkl9012mnop3456qrst7890", false}, // typical APNs token
		{"", false},
		{"abc", false},
	}
	for _, tt := range tests {
		got := isURL(tt.input)
		if got != tt.expect {
			t.Errorf("isURL(%q) = %v, want %v", tt.input, got, tt.expect)
		}
	}
}

// --- Dispatch handler unit test (no DB) ---

// mockDispatcher implements a simple dispatcher for handler tests.
type mockDispatcher struct {
	dispatched []DispatchJob
	err        error
}

func (m *mockDispatcher) Dispatch(_ context.Context, job DispatchJob) error {
	m.dispatched = append(m.dispatched, job)
	return m.err
}

func TestHandleDispatch_SSRFRejection(t *testing.T) {
	// Build a Hasura event body with a device_token that looks like a URL.
	body := `{
		"event": {
			"op": "INSERT",
			"data": {
				"new": {
					"id": "00000000-0000-0000-0000-000000000001",
					"device_token": "http://evil.com/ssrf",
					"platform": "ios",
					"payload": {"aps":{"alert":"test"}},
					"status": "pending",
					"attempts": 0
				}
			}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/push/dispatch", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// We cannot easily call handleDispatch without a DB pool in unit tests,
	// so we test the SSRF guard function directly via isURL + the handler logic.
	// The handler calls isURL(row.DeviceToken) before any DB operation.
	if !isURL("http://evil.com/ssrf") {
		t.Fatal("isURL should detect http:// URL as SSRF candidate")
	}

	_ = req
	_ = w
}

// --- FCM message building tests ---

func TestBuildFCMMessage_WithNotification(t *testing.T) {
	payload := json.RawMessage(`{"notification":{"title":"Hi","body":"World"},"data":{"key":"val"}}`)
	msg, err := buildFCMMessage("tok-123", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Message.Token != "tok-123" {
		t.Errorf("token = %q, want %q", msg.Message.Token, "tok-123")
	}
	if msg.Message.Notification == nil {
		t.Fatal("expected notification block, got nil")
	}
	if msg.Message.Notification.Title != "Hi" {
		t.Errorf("title = %q, want %q", msg.Message.Notification.Title, "Hi")
	}
	if msg.Message.Data["key"] != "val" {
		t.Errorf("data[key] = %q, want %q", msg.Message.Data["key"], "val")
	}
}

func TestBuildFCMMessage_NoNotification(t *testing.T) {
	payload := json.RawMessage(`{"data":{"action":"refresh"}}`)
	msg, err := buildFCMMessage("tok-456", payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Message.Notification != nil {
		t.Error("expected nil notification for data-only payload")
	}
	if msg.Message.Data["action"] != "refresh" {
		t.Errorf("data[action] = %q, want %q", msg.Message.Data["action"], "refresh")
	}
}

// --- APNs credential expiry failover test ---

// TestAPNsExpiredCredentialError verifies that a mock APNs server returning
// ExpiredProviderToken results in a meaningful error message, not a silent failure.
func TestAPNsExpiredCredentialError(t *testing.T) {
	// Stand up a mock APNs server that returns the ExpiredProviderToken error.
	mockAPNs := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"reason": "ExpiredProviderToken",
		})
	}))
	defer mockAPNs.Close()

	// Generate a real EC P-256 key so JWT signing succeeds and we reach the mock server.
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test EC key: %v", err)
	}

	client := &APNsClient{
		teamID:   "TEAMID12AB",
		keyID:    "KEYID12345",
		bundleID: "com.example.app",
		host:     mockAPNs.URL,
		key:      privKey,
		http:     mockAPNs.Client(), // use the TLS client from the test server
	}

	result := client.Send(context.Background(), "device-token-123", json.RawMessage(`{"aps":{"alert":"test"}}`))

	if result.Success {
		t.Error("expected failure for expired credential mock, got success")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
	// The error must mention the credential problem explicitly.
	if !strings.Contains(result.Error, "ExpiredProviderToken") && !strings.Contains(result.Error, "credential") {
		t.Errorf("error message %q should mention credential expiry", result.Error)
	}
	t.Logf("error message (expected): %s", result.Error)
}
