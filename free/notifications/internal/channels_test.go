package internal

import (
	"os"
	"testing"
)

// TestSendEmail_NoSMTPHost verifies that SendEmail returns a failure when
// SMTP_HOST is not configured.
func TestSendEmail_NoSMTPHost(t *testing.T) {
	os.Unsetenv("SMTP_HOST")
	result := SendEmail("user@example.com", "Test", "Hello")
	if result.Success {
		t.Error("expected failure when SMTP_HOST is not set")
	}
	if result.Channel != "email" {
		t.Errorf("expected channel 'email', got %q", result.Channel)
	}
	if result.Error == nil || *result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSendEmail_EmptyRecipient verifies that an empty 'to' address returns an error.
func TestSendEmail_EmptyRecipient(t *testing.T) {
	t.Setenv("SMTP_HOST", "smtp.example.com")
	result := SendEmail("", "Subject", "Body")
	if result.Success {
		t.Error("expected failure for empty recipient")
	}
	if result.Error == nil {
		t.Error("expected error for empty recipient")
	}
}

// TestSendPush_NoProvider verifies that SendPush returns a failure when
// NOTIFICATIONS_PUSH_PROVIDER is not configured.
func TestSendPush_NoProvider(t *testing.T) {
	os.Unsetenv("NOTIFICATIONS_PUSH_PROVIDER")
	result := SendPush("device-token-123", "Title", "Body")
	if result.Success {
		t.Error("expected failure when push provider is not configured")
	}
	if result.Channel != "push" {
		t.Errorf("expected channel 'push', got %q", result.Channel)
	}
	if result.Error == nil || *result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSendPush_WithProvider verifies the "not implemented" path when a
// provider name is set but no real SDK is integrated.
func TestSendPush_WithProvider(t *testing.T) {
	t.Setenv("NOTIFICATIONS_PUSH_PROVIDER", "fcm")
	result := SendPush("device-token", "Hello", "World")
	if result.Success {
		t.Error("expected failure (placeholder not implemented)")
	}
	if result.Channel != "push" {
		t.Errorf("expected channel 'push', got %q", result.Channel)
	}
}

// TestSendSMS_NoProvider verifies that SendSMS returns a failure when
// NOTIFICATIONS_SMS_PROVIDER is not configured.
func TestSendSMS_NoProvider(t *testing.T) {
	os.Unsetenv("NOTIFICATIONS_SMS_PROVIDER")
	result := SendSMS("+1234567890", "Hello!")
	if result.Success {
		t.Error("expected failure when SMS provider is not configured")
	}
	if result.Channel != "sms" {
		t.Errorf("expected channel 'sms', got %q", result.Channel)
	}
}

// TestSendSMS_EmptyPhone verifies that an empty phone number returns an error.
func TestSendSMS_EmptyPhone(t *testing.T) {
	t.Setenv("NOTIFICATIONS_SMS_PROVIDER", "twilio")
	result := SendSMS("", "Hello!")
	if result.Success {
		t.Error("expected failure for empty phone number")
	}
	if result.Error == nil {
		t.Error("expected error for empty phone number")
	}
}

// TestSendSMS_WithProvider verifies the "not implemented" path when a
// provider name is set.
func TestSendSMS_WithProvider(t *testing.T) {
	t.Setenv("NOTIFICATIONS_SMS_PROVIDER", "twilio")
	result := SendSMS("+1234567890", "Hello!")
	if result.Success {
		t.Error("expected failure (placeholder not implemented)")
	}
	if result.Channel != "sms" {
		t.Errorf("expected channel 'sms', got %q", result.Channel)
	}
}

// TestErrPtr verifies the errPtr helper returns a non-nil pointer to the string.
func TestErrPtr(t *testing.T) {
	msg := "something went wrong"
	p := errPtr(msg)
	if p == nil {
		t.Fatal("expected non-nil pointer")
	}
	if *p != msg {
		t.Errorf("got %q, want %q", *p, msg)
	}
}
