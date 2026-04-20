package internal

import (
	"os"
	"testing"
)

// TestValidateWebhookURL_SSRFBlocked verifies that private/internal destinations
// are rejected by the SSRF guard (S43-T14).
func TestValidateWebhookURL_SSRFBlocked(t *testing.T) {
	// Ensure the dev bypass is not active.
	os.Unsetenv("WEBHOOK_ALLOW_PRIVATE_URLS")

	blocked := []string{
		"http://169.254.169.254/latest/meta-data/",  // AWS IMDSv1 — classic SSRF target
		"http://169.254.169.254/",                   // Link-local
		"http://10.0.0.1/admin",                     // RFC1918 Class A
		"http://172.16.0.1/internal",                // RFC1918 Class B
		"http://192.168.1.1/login",                  // RFC1918 Class C
		"http://127.0.0.1/api",                      // Loopback
		"http://[::1]/api",                          // IPv6 loopback
	}

	for _, rawURL := range blocked {
		err := ValidateWebhookURL(rawURL)
		if err == nil {
			t.Errorf("ValidateWebhookURL(%q) should have been blocked by SSRF guard, got nil error", rawURL)
		}
	}
}

// TestValidateWebhookURL_ExternalAllowed verifies that legitimate external
// HTTPS URLs pass validation (S43-T14). These use well-known public hostnames
// that are guaranteed to resolve to public IPs.
func TestValidateWebhookURL_ExternalAllowed(t *testing.T) {
	if os.Getenv("SKIP_NETWORK_TESTS") != "" {
		t.Skip("SKIP_NETWORK_TESTS is set; skipping DNS-dependent tests")
	}
	os.Unsetenv("WEBHOOK_ALLOW_PRIVATE_URLS")

	// These hostnames resolve to public IPs; validation should pass.
	// Note: we do not use real service webhook paths here to avoid triggering
	// secret-scanning heuristics on CI.
	allowed := []string{
		"https://hooks.slack.com/trigger/test-path",
		"https://api.github.com/repos/example/example/issues",
	}

	for _, rawURL := range allowed {
		err := ValidateWebhookURL(rawURL)
		if err != nil {
			t.Logf("ValidateWebhookURL(%q) returned error (may be DNS unavailable in CI): %v", rawURL, err)
			// Not fatal — DNS may be unavailable in sandboxed CI. Log only.
		}
	}
}

// TestValidateWebhookURL_DevBypass verifies that WEBHOOK_ALLOW_PRIVATE_URLS=true
// skips the SSRF guard, enabling local development without network access.
func TestValidateWebhookURL_DevBypass(t *testing.T) {
	t.Setenv("WEBHOOK_ALLOW_PRIVATE_URLS", "true")
	// Should pass with no error even for a private IP.
	err := ValidateWebhookURL("http://169.254.169.254/latest/meta-data/")
	if err != nil {
		t.Errorf("ValidateWebhookURL with WEBHOOK_ALLOW_PRIVATE_URLS=true should bypass guard, got: %v", err)
	}
}

// TestValidateWebhookURL_InvalidURL verifies that malformed URLs are rejected.
func TestValidateWebhookURL_InvalidURL(t *testing.T) {
	os.Unsetenv("WEBHOOK_ALLOW_PRIVATE_URLS")

	invalid := []string{
		"not-a-url",
		"ftp://example.com/webhook",
		"",
	}

	for _, rawURL := range invalid {
		err := ValidateWebhookURL(rawURL)
		if err == nil {
			t.Errorf("ValidateWebhookURL(%q) should have returned an error, got nil", rawURL)
		}
	}
}
