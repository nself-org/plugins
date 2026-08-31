package lemonsqueezy

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/nself-org/nself-payments/internal/provider"
)

func buildLSSignature(t testing.TB, secret string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestVerifyWebhookSignature_Valid verifies a valid LS HMAC-SHA256 signature.
func TestVerifyWebhookSignature_Valid(t *testing.T) {
	a := New("apikey", "store1")
	body := lsWebhookPayload("subscription_created", "active")
	secret := "ls_test_secret"
	sig := buildLSSignature(t, secret, body)

	eventType, err := a.VerifyWebhookSignature(sig, body, secret)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eventType != "subscription_created" {
		t.Errorf("expected subscription_created, got %q", eventType)
	}
}

// TestVerifyWebhookSignature_WrongSecret verifies wrong secret is rejected.
func TestVerifyWebhookSignature_WrongSecret(t *testing.T) {
	a := New("apikey", "store1")
	body := lsWebhookPayload("subscription_created", "active")
	sig := buildLSSignature(t, "right_secret", body)

	_, err := a.VerifyWebhookSignature(sig, body, "wrong_secret")
	if err == nil {
		t.Fatal("expected error for wrong secret")
	}
}

// TestVerifyWebhookSignature_EmptySignature verifies empty sig is rejected.
func TestVerifyWebhookSignature_EmptySignature(t *testing.T) {
	a := New("apikey", "store1")
	_, err := a.VerifyWebhookSignature("", []byte("body"), "secret")
	if err == nil {
		t.Fatal("expected error for empty signature")
	}
}

// TestParseWebhookEvent_SubscriptionCreated parses subscription_created.
func TestParseWebhookEvent_SubscriptionCreated(t *testing.T) {
	a := New("apikey", "store1")
	body := lsWebhookPayload("subscription_created", "active")

	sub, err := a.ParseWebhookEvent("subscription_created", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub == nil {
		t.Fatal("expected subscription, got nil")
	}
	if sub.Status != provider.StatusActive {
		t.Errorf("expected active, got %q", sub.Status)
	}
}

// TestParseWebhookEvent_SubscriptionPaymentFailed maps to past_due.
func TestParseWebhookEvent_SubscriptionPaymentFailed(t *testing.T) {
	a := New("apikey", "store1")
	body := lsWebhookPayload("subscription_payment_failed", "past_due")

	sub, err := a.ParseWebhookEvent("subscription_payment_failed", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub == nil {
		t.Fatal("expected subscription, got nil")
	}
	if sub.Status != provider.StatusPastDue {
		t.Errorf("expected past_due, got %q", sub.Status)
	}
}

// TestParseWebhookEvent_SubscriptionCancelled maps to canceled.
func TestParseWebhookEvent_SubscriptionCancelled(t *testing.T) {
	a := New("apikey", "store1")
	body := lsWebhookPayload("subscription_cancelled", "cancelled")

	sub, err := a.ParseWebhookEvent("subscription_cancelled", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub == nil {
		t.Fatal("expected subscription, got nil")
	}
	if sub.Status != provider.StatusCanceled {
		t.Errorf("expected canceled, got %q", sub.Status)
	}
}

// TestParseWebhookEvent_NonSubscriptionEvent returns nil.
func TestParseWebhookEvent_NonSubscriptionEvent(t *testing.T) {
	a := New("apikey", "store1")
	body := []byte(`{"meta":{"event_name":"order_created"},"data":{}}`)

	sub, err := a.ParseWebhookEvent("order_created", body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sub != nil {
		t.Errorf("expected nil for non-subscription event, got %+v", sub)
	}
}

// TestMapLSStatus_AllVariants tests the status mapping function.
func TestMapLSStatus_AllVariants(t *testing.T) {
	cases := []struct {
		input    string
		expected provider.SubscriptionStatus
	}{
		{"active", provider.StatusActive},
		{"past_due", provider.StatusPastDue},
		{"cancelled", provider.StatusCanceled},
		{"expired", provider.StatusCanceled},
		{"paused", provider.StatusPaused},
		{"on_trial", provider.StatusTrialing},
		{"unknown_state", provider.SubscriptionStatus("unknown_state")},
	}
	for _, tc := range cases {
		got := mapLSStatus(tc.input)
		if got != tc.expected {
			t.Errorf("mapLSStatus(%q): expected %q, got %q", tc.input, tc.expected, got)
		}
	}
}

// TestName returns the correct provider name.
func TestName(t *testing.T) {
	a := New("key", "store")
	if a.Name() != "lemonsqueezy" {
		t.Errorf("expected lemonsqueezy, got %q", a.Name())
	}
}

// TestRecordUsage_NoopForLS verifies LS RecordUsage returns nil (no native support).
func TestRecordUsage_NoopForLS(t *testing.T) {
	a := New("key", "store")
	err := a.RecordUsage(nil, provider.UsageRecord{MetricID: "api_calls", Quantity: 10})
	if err != nil {
		t.Errorf("expected nil (no-op), got: %v", err)
	}
}

// helpers

func lsWebhookPayload(eventName, status string) []byte {
	payload := map[string]interface{}{
		"meta": map[string]string{
			"event_name": eventName,
		},
		"data": lsSubscriptionData{
			ID: "sub_ls_1",
			Attributes: struct {
				CustomerID  string `json:"customer_id"`
				ProductID   string `json:"product_id"`
				VariantID   string `json:"variant_id"`
				Status      string `json:"status"`
				RenewsAt    string `json:"renews_at"`
				CreatedAt   string `json:"created_at"`
				TrialEndsAt string `json:"trial_ends_at"`
				EndsAt      string `json:"ends_at"`
				CancelledAt string `json:"cancelled_at"`
			}{
				CustomerID: "1234",
				ProductID:  "prod_1",
				VariantID:  "var_1",
				Status:     status,
				CreatedAt:  "2026-01-01T00:00:00Z",
				RenewsAt:   "2026-02-01T00:00:00Z",
			},
		},
	}
	b, _ := json.Marshal(payload)
	return b
}
