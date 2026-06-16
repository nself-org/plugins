package metrics

import (
	"testing"
)

// Tests for SetDefault, Default, and statusClass which were not covered.

func TestSetAndGetDefault(t *testing.T) {
	// Start with nil default
	SetDefault(nil)
	if got := Default(); got != nil {
		t.Errorf("Default() = %v, want nil after SetDefault(nil)", got)
	}

	// Set a real registry and retrieve it
	r := NewRegistry("test_set_default", "1.0.0")
	SetDefault(r)
	got := Default()
	if got != r {
		t.Errorf("Default() = %v, want %v", got, r)
	}

	// Reset to nil to avoid polluting other tests
	SetDefault(nil)
}

func TestStatusClass(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{299, "2xx"},
		{301, "3xx"},
		{302, "3xx"},
		{400, "4xx"},
		{404, "4xx"},
		{499, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{100, "1xx"},
		{101, "1xx"},
	}
	for _, tt := range tests {
		got := statusClass(tt.code)
		if got != tt.want {
			t.Errorf("statusClass(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}
