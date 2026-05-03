package internal

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// buildStripeSignature constructs a valid Stripe webhook signature header
// for the given body and secret.
func buildStripeSignature(body []byte, secret string) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	signed := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	return fmt.Sprintf("t=%s,v1=%s", ts, sig)
}

// TestVerifyStripeSignature_Valid verifies a correctly signed webhook passes.
func TestVerifyStripeSignature_Valid(t *testing.T) {
	body := []byte(`{"id":"evt_123","type":"customer.created"}`)
	secret := "whsec_test_secret"
	header := buildStripeSignature(body, secret)

	if err := VerifyStripeSignature(body, header, secret); err != nil {
		t.Errorf("expected valid signature to pass, got: %v", err)
	}
}

// TestVerifyStripeSignature_WrongSecret verifies a wrong secret fails.
func TestVerifyStripeSignature_WrongSecret(t *testing.T) {
	body := []byte(`{"id":"evt_123","type":"customer.created"}`)
	header := buildStripeSignature(body, "correct-secret")

	if err := VerifyStripeSignature(body, header, "wrong-secret"); err == nil {
		t.Error("expected wrong secret to fail, got nil error")
	}
}

// TestVerifyStripeSignature_MissingHeader verifies that an empty header fails.
func TestVerifyStripeSignature_MissingHeader(t *testing.T) {
	if err := VerifyStripeSignature([]byte("{}"), "", "secret"); err == nil {
		t.Error("expected error for missing header, got nil")
	}
}

// TestVerifyStripeSignature_MissingSecret verifies that an empty secret fails.
func TestVerifyStripeSignature_MissingSecret(t *testing.T) {
	body := []byte(`{}`)
	header := buildStripeSignature(body, "some-secret")
	if err := VerifyStripeSignature(body, header, ""); err == nil {
		t.Error("expected error for missing secret, got nil")
	}
}

// TestVerifyStripeSignature_ExpiredTimestamp verifies that an old timestamp
// is rejected.
func TestVerifyStripeSignature_ExpiredTimestamp(t *testing.T) {
	body := []byte(`{"id":"evt_old"}`)
	secret := "secret"
	// Build a header with a timestamp 10 minutes in the past.
	ts := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	signed := ts + "." + string(body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signed))
	sig := hex.EncodeToString(mac.Sum(nil))
	header := fmt.Sprintf("t=%s,v1=%s", ts, sig)

	if err := VerifyStripeSignature(body, header, secret); err == nil {
		t.Error("expected expired timestamp to fail, got nil")
	}
}

// TestVerifyStripeSignature_MalformedHeader verifies that a malformed header fails.
func TestVerifyStripeSignature_MalformedHeader(t *testing.T) {
	cases := []string{
		"no_equals_sign",
		"t=abc,v1=",   // missing signature
		"v1=abc",      // missing timestamp
	}
	for _, h := range cases {
		if err := VerifyStripeSignature([]byte("{}"), h, "secret"); err == nil {
			t.Errorf("expected error for header %q, got nil", h)
		}
	}
}

// TestIsDeleteEvent verifies the delete event type detection.
func TestIsDeleteEvent(t *testing.T) {
	cases := []struct {
		eventType string
		want      bool
	}{
		{"customer.deleted", true},
		{"payment_method.deleted", true},
		{"invoiceitem.deleted", true},
		{"customer.tax_id.deleted", true},
		{"customer.created", false},
		{"payment_intent.succeeded", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isDeleteEvent(tc.eventType)
		if got != tc.want {
			t.Errorf("isDeleteEvent(%q) = %v, want %v", tc.eventType, got, tc.want)
		}
	}
}

// TestExtractObjectInfo verifies extraction from a Stripe data object.
func TestExtractObjectInfo(t *testing.T) {
	cases := []struct {
		raw        string
		wantType   string
		wantID     string
	}{
		{`{"object":"customer","id":"cus_123"}`, "customer", "cus_123"},
		{`{"object":"","id":""}`, "unknown", "unknown"},
		{`{}`, "unknown", "unknown"},
		{`not-json`, "unknown", "unknown"},
	}
	for _, tc := range cases {
		gotType, gotID := extractObjectInfo(json.RawMessage(tc.raw))
		if gotType != tc.wantType || gotID != tc.wantID {
			t.Errorf("extractObjectInfo(%q) = (%q, %q), want (%q, %q)",
				tc.raw, gotType, gotID, tc.wantType, tc.wantID)
		}
	}
}

// TestBuildStripeSignatureFormat verifies that our test helper generates a
// header with the expected t=,v1= format (documents the format for readers).
func TestBuildStripeSignatureFormat(t *testing.T) {
	header := buildStripeSignature([]byte("{}"), "secret")
	if !strings.HasPrefix(header, "t=") {
		t.Errorf("expected header to start with 't=', got %q", header)
	}
	if !strings.Contains(header, ",v1=") {
		t.Errorf("expected header to contain ',v1=', got %q", header)
	}
}
