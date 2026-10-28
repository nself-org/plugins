package internal

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHasInternalSecret proves the canonical header is accepted, every legacy name is still read (D-0198), and a
// missing, empty or wrong secret is refused, including when the configured secret is empty.
func TestHasInternalSecret(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		want    string
		ok      bool
	}{
		{"canonical", map[string]string{"X-Internal-Token": "s3"}, "s3", true},
		{"legacy X-Plugin-Secret", map[string]string{"X-Plugin-Secret": "s3"}, "s3", true},
		{"legacy X-Internal-Secret", map[string]string{"X-Internal-Secret": "s3"}, "s3", true},
		{"legacy X-Internal-Key", map[string]string{"X-Internal-Key": "s3"}, "s3", true},
		{"canonical wins over a legacy header", map[string]string{"X-Internal-Token": "s3", "X-Plugin-Secret": "other"}, "s3", true},
		{"wrong canonical is not rescued by a legacy header", map[string]string{"X-Internal-Token": "bad", "X-Plugin-Secret": "s3"}, "s3", false},
		{"wrong value", map[string]string{"X-Internal-Token": "bad"}, "s3", false},
		{"no header", nil, "s3", false},
		{"empty configured secret never matches", map[string]string{"X-Internal-Token": ""}, "", false},
	}
	for _, c := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		for k, v := range c.headers {
			r.Header.Set(k, v)
		}
		if got := hasInternalSecret(r, c.want); got != c.ok {
			t.Errorf("%s: hasInternalSecret = %v, want %v", c.name, got, c.ok)
		}
	}
}

// TestAuthorizeAdminRequest_CanonicalHeader proves the admin gate accepts X-Internal-Token.
func TestAuthorizeAdminRequest_CanonicalHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Internal-Token", "my-secret")
	if !authorizeAdminRequest(r, "my-secret", "") {
		t.Error("expected authorizeAdminRequest to accept X-Internal-Token")
	}
}
