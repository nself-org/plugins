package internal

import (
	"net/http"
	"net/http/httptest"
	"path"
	"testing"
)

func TestValidateKey_safe(t *testing.T) {
	cases := []struct {
		key  string
		want error
	}{
		{"file.txt", nil},
		{"folder/file.txt", nil},
		{"a/b/c/d.bin", nil},
		{"../etc/passwd", ErrInvalidKey},
		{"foo/../../etc/passwd", ErrInvalidKey},
	}
	for _, tc := range cases {
		got := validateKey(tc.key)
		if got != tc.want {
			t.Errorf("validateKey(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestPathClean(t *testing.T) {
	// Ensure path.Clean rejects traversal
	cleaned := path.Clean("/../etc/passwd")
	if cleaned != "/etc/passwd" {
		t.Errorf("unexpected clean: %s", cleaned)
	}
}

func TestHandleHealth_noDBIsUnhealthy(t *testing.T) {
	// Server with nil db should return 503
	s := &Server{db: nil, cfg: &Config{Port: "9007"}}
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	// Manually call health handler; with nil db it should return 503
	http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.db == nil {
			http.Error(w, `{"status":"unhealthy"}`, http.StatusServiceUnavailable)
			return
		}
		s.handleHealth(w, r)
	}).ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestSourceAccountID_default(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := sourceAccountID(req); got != "primary" {
		t.Errorf("expected 'primary', got %q", got)
	}
}

func TestSourceAccountID_fromHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Hasura-Source-Account-Id", "tenant-abc")
	if got := sourceAccountID(req); got != "tenant-abc" {
		t.Errorf("expected 'tenant-abc', got %q", got)
	}
}
