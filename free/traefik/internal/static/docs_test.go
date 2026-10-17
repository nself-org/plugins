package static

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDocsFiles(t *testing.T) {
	dist := t.TempDir()
	for _, name := range []string{"openapi.json", "scalar.html", "secret"} {
		if err := os.WriteFile(filepath.Join(dist, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h := Handler(t.TempDir(), dist)
	for _, c := range []struct {
		method, path, body string
		status             int
	}{
		{"GET", "/__nself/docs/openapi.json", "openapi.json", 200},
		{"GET", "/__nself/docs/scalar.html?x=1", "scalar.html", 200},
		{"HEAD", "/__nself/docs/openapi.json", "", 200},
		{"POST", "/__nself/docs/openapi.json", "", 405},
		{"GET", "/__nself/docs/", "", 404},
		{"GET", "/__nself/docs/secret", "", 404},
		{"GET", "/__nself/docs/openapi.json/x", "", 404},
		{"GET", "/__nself/docs/../secret", "", 404},
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(c.method, c.path, nil))
		if rec.Code != c.status || rec.Body.String() != c.body {
			t.Errorf("%s %s: got %d %q, want %d %q", c.method, c.path, rec.Code, rec.Body.String(), c.status, c.body)
		}
	}
	if err := os.Remove(filepath.Join(dist, "openapi.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dist, "secret"), filepath.Join(dist, "openapi.json")); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/__nself/docs/openapi.json", nil))
	if rec.Code != 404 {
		t.Errorf("symlink served: %d", rec.Code)
	}
}
