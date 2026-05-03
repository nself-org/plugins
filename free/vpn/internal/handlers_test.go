package internal

import (
	"net/http/httptest"
	"net/http"
	"testing"
)

// TestParseServerFilter_Defaults verifies that an empty query string produces
// the default ServerFilter.
func TestParseServerFilter_Defaults(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/servers", nil)
	f := parseServerFilter(r)
	if f.Limit != 100 {
		t.Errorf("default Limit = %d, want 100", f.Limit)
	}
	if f.Provider != "" || f.Country != "" {
		t.Errorf("expected empty Provider/Country, got %q/%q", f.Provider, f.Country)
	}
	if f.P2POnly || f.PortForwarding {
		t.Error("expected P2POnly and PortForwarding to be false by default")
	}
}

// TestParseServerFilter_AllParams verifies that all query params are parsed
// correctly.
func TestParseServerFilter_AllParams(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/servers?provider=mullvad&country=SE&p2p_only=true&port_forwarding=true&limit=25", nil)
	f := parseServerFilter(r)
	if f.Provider != "mullvad" {
		t.Errorf("Provider = %q, want %q", f.Provider, "mullvad")
	}
	if f.Country != "SE" {
		t.Errorf("Country = %q, want %q", f.Country, "SE")
	}
	if !f.P2POnly {
		t.Error("expected P2POnly=true")
	}
	if !f.PortForwarding {
		t.Error("expected PortForwarding=true")
	}
	if f.Limit != 25 {
		t.Errorf("Limit = %d, want 25", f.Limit)
	}
}

// TestParseServerFilter_InvalidLimit verifies that a non-numeric limit falls
// back to the default of 100.
func TestParseServerFilter_InvalidLimit(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/servers?limit=abc", nil)
	f := parseServerFilter(r)
	if f.Limit != 100 {
		t.Errorf("invalid limit fallback = %d, want 100", f.Limit)
	}
}

// TestPtrStr_NonNil verifies that ptrStr dereferences a non-nil pointer.
func TestPtrStr_NonNil(t *testing.T) {
	s := "hello"
	got := ptrStr(&s)
	if got != "hello" {
		t.Errorf("ptrStr(&%q) = %q, want %q", s, got, s)
	}
}

// TestPtrStr_Nil verifies that ptrStr returns empty string for nil.
func TestPtrStr_Nil(t *testing.T) {
	got := ptrStr(nil)
	if got != "" {
		t.Errorf("ptrStr(nil) = %q, want empty string", got)
	}
}
