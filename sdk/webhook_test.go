package sdk

import (
	"errors"
	"strconv"
	"testing"
	"time"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	body := []byte(`{"event":"ping","value":42}`)
	secret := "test-secret"
	now := time.Now()

	sig := SignWebhookPayload(body, secret, now)
	if sig == "" {
		t.Fatal("SignWebhookPayload returned empty signature")
	}

	if err := VerifyWebhookSignature(sig, body, secret, DefaultTolerance); err != nil {
		t.Fatalf("round-trip verify failed: %v", err)
	}
}

func TestVerifyMissingHeader(t *testing.T) {
	err := VerifyWebhookSignature("", []byte("x"), "s", DefaultTolerance)
	if !errors.Is(err, ErrMissingSignature) {
		t.Errorf("expected ErrMissingSignature, got %v", err)
	}
}

func TestVerifyMalformed(t *testing.T) {
	cases := []string{
		"garbage",
		"t=xyz,v1=abc",
		"v1=abc",
		"t=123",
		"t=123,v1=",
	}
	for _, c := range cases {
		if err := VerifyWebhookSignature(c, []byte("x"), "s", DefaultTolerance); err == nil {
			t.Errorf("expected error for %q, got nil", c)
		}
	}
}

func TestVerifyExpired(t *testing.T) {
	body := []byte("x")
	secret := "s"
	old := time.Now().Add(-10 * time.Minute)
	sig := SignWebhookPayload(body, secret, old)
	err := VerifyWebhookSignature(sig, body, secret, DefaultTolerance)
	if !errors.Is(err, ErrSignatureExpired) {
		t.Errorf("expected ErrSignatureExpired, got %v", err)
	}
}

func TestVerifyToleranceZeroDisablesReplayCheck(t *testing.T) {
	body := []byte("x")
	secret := "s"
	old := time.Now().Add(-10 * time.Minute)
	sig := SignWebhookPayload(body, secret, old)
	if err := VerifyWebhookSignature(sig, body, secret, 0); err != nil {
		t.Errorf("tolerance=0 should skip replay check, got %v", err)
	}
}

func TestVerifyTamperedBody(t *testing.T) {
	body := []byte(`{"ok":true}`)
	secret := "s"
	sig := SignWebhookPayload(body, secret, time.Now())

	tampered := []byte(`{"ok":false}`)
	err := VerifyWebhookSignature(sig, tampered, secret, DefaultTolerance)
	if !errors.Is(err, ErrSignatureMismatch) {
		t.Errorf("tampered body should fail mismatch, got %v", err)
	}
}

func TestVerifyWrongSecret(t *testing.T) {
	body := []byte("x")
	sig := SignWebhookPayload(body, "right", time.Now())
	err := VerifyWebhookSignature(sig, body, "wrong", DefaultTolerance)
	if !errors.Is(err, ErrSignatureMismatch) {
		t.Errorf("expected mismatch for wrong secret, got %v", err)
	}
}

func TestVerifyUnsupportedVersion(t *testing.T) {
	// A future v2 payload should not silently pass.
	header := "t=" + strconv.FormatInt(time.Now().Unix(), 10) + ",v2=deadbeef"
	err := VerifyWebhookSignature(header, []byte("x"), "s", DefaultTolerance)
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("expected ErrUnsupportedVersion, got %v", err)
	}
}
