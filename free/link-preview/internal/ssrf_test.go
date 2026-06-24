package internal

import (
	"strings"
	"testing"
)

// TestValidatePreviewURL_BlocksInternal verifies that the SSRF guard rejects
// URLs resolving to private/internal addresses (loopback, RFC1918, cloud
// metadata) when the dev opt-out is not set.
func TestValidatePreviewURL_BlocksInternal(t *testing.T) {
	t.Setenv("LINK_PREVIEW_ALLOW_PRIVATE_URLS", "")
	cases := []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://169.254.169.254/latest/meta-data/", // cloud metadata
		"http://10.0.0.5/",
		"http://192.168.1.1/",
		"http://[::1]/",
	}
	for _, raw := range cases {
		if err := validatePreviewURL(raw); err == nil {
			t.Errorf("expected SSRF block for %q, got nil", raw)
		}
	}
}

// TestValidatePreviewURL_RejectsBadScheme verifies non-http(s) schemes are denied.
func TestValidatePreviewURL_RejectsBadScheme(t *testing.T) {
	t.Setenv("LINK_PREVIEW_ALLOW_PRIVATE_URLS", "")
	for _, raw := range []string{"file:///etc/passwd", "gopher://x/", "ftp://x/"} {
		if err := validatePreviewURL(raw); err == nil {
			t.Errorf("expected scheme rejection for %q, got nil", raw)
		}
	}
}

// TestValidatePreviewURL_OptOut verifies the dev opt-out bypasses the guard.
func TestValidatePreviewURL_OptOut(t *testing.T) {
	t.Setenv("LINK_PREVIEW_ALLOW_PRIVATE_URLS", "true")
	if err := validatePreviewURL("http://127.0.0.1/"); err != nil {
		t.Errorf("opt-out should bypass guard, got %v", err)
	}
}

// TestValidatePreviewURL_AllowsPublic verifies a public host is permitted.
func TestValidatePreviewURL_AllowsPublic(t *testing.T) {
	t.Setenv("LINK_PREVIEW_ALLOW_PRIVATE_URLS", "")
	if err := validatePreviewURL("https://example.com/"); err != nil {
		// DNS-dependent; only fail on a clear SSRF block, not on offline DNS.
		if strings.Contains(err.Error(), "private/internal address") {
			t.Errorf("public host wrongly blocked: %v", err)
		}
	}
}
