package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// TestErrPtr verifies that errPtr returns a non-nil pointer to the given
// string.
func TestErrPtr(t *testing.T) {
	msg := "something went wrong"
	p := errPtr(msg)
	if p == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *p != msg {
		t.Errorf("*errPtr = %q, want %q", *p, msg)
	}
}

// TestBuildWebhookPayload verifies that buildWebhookPayload returns valid JSON
// containing the subject and body fields.
func TestBuildWebhookPayload(t *testing.T) {
	payload := buildWebhookPayload("Hello", "World")
	if payload == "" {
		t.Fatal("expected non-empty payload")
	}

	var m map[string]string
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("payload is not valid JSON: %v", err)
	}
	if m["subject"] != "Hello" {
		t.Errorf("subject = %q, want %q", m["subject"], "Hello")
	}
	if m["body"] != "World" {
		t.Errorf("body = %q, want %q", m["body"], "World")
	}
}

// TestBuildWebhookPayload_EmptyValues verifies that empty strings are encoded
// correctly (not omitted).
func TestBuildWebhookPayload_EmptyValues(t *testing.T) {
	payload := buildWebhookPayload("", "")
	if !strings.Contains(payload, `"subject"`) {
		t.Error("expected 'subject' key in payload even when empty")
	}
}

// TestSendEmail_NoSMTPHost verifies that SendEmail fails when SMTP_HOST is not
// configured.
func TestSendEmail_NoSMTPHost(t *testing.T) {
	os.Unsetenv("SMTP_HOST")
	result := SendEmail("user@example.com", "Subject", "Body")
	if result.Success {
		t.Error("expected failure when SMTP_HOST is not configured")
	}
	if result.Channel != "email" {
		t.Errorf("Channel = %q, want %q", result.Channel, "email")
	}
	if result.Error == nil || *result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSendEmail_EmptyRecipient verifies that an empty recipient address fails.
func TestSendEmail_EmptyRecipient(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	result := SendEmail("", "Subject", "Body")
	if result.Success {
		t.Error("expected failure for empty recipient")
	}
	if result.Error == nil {
		t.Error("expected error message for empty recipient")
	}
}

// TestSendWebhook_EmptyURL verifies that an empty URL returns a failure.
func TestSendWebhook_EmptyURL(t *testing.T) {
	result := SendWebhook("", `{"event":"test"}`)
	if result.Success {
		t.Error("expected failure for empty URL")
	}
	if result.Channel != "webhook" {
		t.Errorf("Channel = %q, want %q", result.Channel, "webhook")
	}
}

// TestSendWebhook_HappyPath verifies that a successful POST to a webhook URL
// returns a success result.
func TestSendWebhook_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %q", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := SendWebhook(srv.URL, `{"event":"test"}`)
	if !result.Success {
		msg := ""
		if result.Error != nil {
			msg = *result.Error
		}
		t.Errorf("expected success, got failure: %s", msg)
	}
}

// TestSendWebhook_ServerError verifies that a non-2xx response returns a
// failure.
func TestSendWebhook_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	result := SendWebhook(srv.URL, `{}`)
	if result.Success {
		t.Error("expected failure for 500 response")
	}
}
