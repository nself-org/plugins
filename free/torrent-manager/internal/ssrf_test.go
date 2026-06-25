package internal

import (
	"strings"
	"testing"
)

// TestValidateAdminURL_BlocksPrivateIPs verifies that private/internal
// destinations are rejected by the SSRF guard.
func TestValidateAdminURL_BlocksPrivateIPs(t *testing.T) {
	t.Setenv("TORRENT_ALLOW_PRIVATE_URLS", "")

	blocked := []struct {
		url    string
		reason string
	}{
		{"http://192.168.1.1:9091/transmission/rpc", "RFC1918 Class C"},
		{"http://10.0.0.1:9091/transmission/rpc", "RFC1918 Class A"},
		{"http://172.16.0.1:9091/", "RFC1918 Class B"},
		{"http://169.254.169.254/latest/meta-data/", "cloud metadata (link-local)"},
		{"http://127.0.0.1:9091/", "loopback"},
	}

	for _, tc := range blocked {
		err := ValidateAdminURL(tc.url)
		if err == nil {
			t.Errorf("ValidateAdminURL(%q) should be blocked (%s), got nil", tc.url, tc.reason)
		}
	}
}

// TestValidateAdminURL_BlocksBadScheme verifies non-http(s) schemes are rejected.
func TestValidateAdminURL_BlocksBadScheme(t *testing.T) {
	t.Setenv("TORRENT_ALLOW_PRIVATE_URLS", "")

	badSchemes := []string{
		"ftp://example.com:9091/",
		"file:///etc/passwd",
		"ldap://example.com/",
	}

	for _, u := range badSchemes {
		err := ValidateAdminURL(u)
		if err == nil {
			t.Errorf("ValidateAdminURL(%q) should reject non-http(s) scheme, got nil", u)
		}
	}
}

// TestValidateAdminURL_BlocksDisallowedPort verifies that non-allowlisted ports
// are rejected.
func TestValidateAdminURL_BlocksDisallowedPort(t *testing.T) {
	t.Setenv("TORRENT_ALLOW_PRIVATE_URLS", "")

	// Port 22 (SSH) is not in the allowlist.
	err := ValidateAdminURL("http://example.com:22/")
	if err == nil {
		t.Error("ValidateAdminURL with port 22 should be rejected, got nil")
	}
	if !strings.Contains(err.Error(), "port") {
		t.Errorf("expected port error, got: %v", err)
	}
}

// TestValidateAdminURL_DevBypass verifies that TORRENT_ALLOW_PRIVATE_URLS=true
// bypasses the guard (dev-only opt-in, never in prod).
func TestValidateAdminURL_DevBypass(t *testing.T) {
	t.Setenv("TORRENT_ALLOW_PRIVATE_URLS", "true")

	// Private IP that would normally be blocked.
	err := ValidateAdminURL("http://192.168.1.1:9091/transmission/rpc")
	if err != nil {
		t.Errorf("ValidateAdminURL with dev bypass should return nil, got %v", err)
	}
}
