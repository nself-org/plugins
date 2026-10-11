package static

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandler(t *testing.T) {
	root := t.TempDir()
	ch := filepath.Join(root, ".well-known", "acme-challenge")
	_ = os.MkdirAll(filepath.Join(ch, "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(ch, "tok_en-1"), []byte("key-auth"), 0o644)
	_ = os.WriteFile(filepath.Join(ch, "sub", "inner"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "secret"), []byte("s"), 0o644)
	_ = os.Symlink(filepath.Join(root, "secret"), filepath.Join(ch, "link"))
	h := Handler(root)
	cases := []struct {
		method, path string
		want         int
		body         string
	}{
		{"GET", "/.well-known/acme-challenge/tok_en-1", 200, "key-auth"},
		{"HEAD", "/.well-known/acme-challenge/tok_en-1", 200, ""},
		{"POST", "/.well-known/acme-challenge/tok_en-1", 403, ""},
		{"POST", "/.well-known/acme-challenge/bad.token", 404, ""},
		{"GET", "/.well-known/acme-challenge/missing", 404, ""},
		{"GET", "/.well-known/acme-challenge/", 404, ""},
		{"GET", "/.well-known/acme-challenge/sub/inner", 404, ""},
		{"GET", "/.well-known/acme-challenge/link", 404, ""},
		{"GET", "/.well-known/acme-challenge/..%2f..%2fsecret", 404, ""},
		{"GET", "/secret", 404, ""},
		{"GET", "/__nself/ok", 200, "OK"},
		{"POST", "/__nself/status/403", 403, ""},
		{"GET", "/__nself/status/404", 404, ""},
		{"GET", "/__nself/status/abc", 404, ""},
		{"GET", "/__nself/status/101", 404, ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.want || (c.body != "" && rec.Body.String() != c.body) {
			t.Errorf("%s %s: got %d %q, want %d %q", c.method, c.path, rec.Code, rec.Body.String(), c.want, c.body)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/.well-known/acme-challenge/tok_en-1", nil))
	if rec.Header().Get("ETag") == "" || rec.Header().Get("Content-Type") != "application/octet-stream" || rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("headers: %v", rec.Header())
	}
	_ = http.StatusOK
}
