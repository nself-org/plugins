package httpx

import (
	"strings"
	"testing"
)

// TestValidateOutboundURL_RFC1918Blocked verifies that RFC1918 addresses are
// blocked by the SSRF guard.
func TestValidateOutboundURL_RFC1918Blocked(t *testing.T) {
	privateURLs := []string{
		"http://10.0.0.1/hook",
		"http://192.168.1.1/hook",
		"http://172.16.0.1/hook",
		"http://172.31.255.255/hook",
		"http://169.254.169.254/latest/meta-data",
		"http://127.0.0.1/hook",
	}
	for _, rawURL := range privateURLs {
		t.Run(rawURL, func(t *testing.T) {
			// Use NSELF_ALLOW_PRIVATE_URLS=false (default).
			t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "false")
			err := ValidateOutboundURL(rawURL)
			if err == nil {
				t.Errorf("expected SSRF block for %q, got nil error", rawURL)
			}
		})
	}
}

// TestValidateOutboundURL_IMDSBlocked verifies that the AWS/GCP instance
// metadata service endpoint is blocked.
func TestValidateOutboundURL_IMDSBlocked(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "false")
	err := ValidateOutboundURL("http://169.254.169.254/latest/meta-data")
	if err == nil {
		t.Error("expected SSRF block for IMDS URL 169.254.169.254, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "SSRF") &&
		!strings.Contains(err.Error(), "private") &&
		!strings.Contains(err.Error(), "internal") {
		t.Errorf("expected SSRF/private/internal in error, got: %v", err)
	}
}

// TestValidateOutboundURL_InvalidSchemeBlocked verifies that non-http(s)
// schemes are rejected.
func TestValidateOutboundURL_InvalidSchemeBlocked(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "false")
	err := ValidateOutboundURL("ftp://example.com/hook")
	if err == nil {
		t.Error("expected error for ftp:// scheme, got nil")
	}
}

// TestValidateOutboundURL_DevBypassAllowed verifies that NSELF_ALLOW_PRIVATE_URLS=true
// bypasses the guard (dev-only opt-in).
func TestValidateOutboundURL_DevBypassAllowed(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "true")
	err := ValidateOutboundURL("http://127.0.0.1/hook")
	if err != nil {
		t.Errorf("expected nil error with dev bypass, got: %v", err)
	}
}

// TestValidateOutboundURL_EmptyURL verifies that an empty URL string returns
// an error.
func TestValidateOutboundURL_EmptyURL(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "false")
	err := ValidateOutboundURL("")
	if err == nil {
		t.Error("expected error for empty URL, got nil")
	}
}

// TestValidateOutboundURL_MissingHost verifies that a URL with no host is
// rejected.
func TestValidateOutboundURL_MissingHost(t *testing.T) {
	t.Setenv("NSELF_ALLOW_PRIVATE_URLS", "false")
	err := ValidateOutboundURL("https:///no-host")
	if err == nil {
		t.Error("expected error for URL with missing host, got nil")
	}
}
