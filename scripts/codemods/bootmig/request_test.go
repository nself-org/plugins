package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHealthRequestContext(t *testing.T) {
	for _, name := range []string{"req", "request"} {
		t.Run(name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "h.go")
			src := hdr + "func handleHealth(out http.ResponseWriter, " + name + " *http.Request) {\n" +
				`_ = json.NewEncoder(out).Encode(map[string]any{"status": "ok"})` + "\n}\n"
			if err := os.WriteFile(f, []byte(src), 0o600); err != nil {
				t.Fatal(err)
			}
			hs := healthEdit([]string{f})
			if hs == nil || hs.manual {
				t.Fatal("request handler was not wired")
			}
			if !strings.Contains(hs.edit.text, "bootMigrationsHealth("+name+".Context(), out)") {
				t.Fatalf("request context missing: %+v", hs.edit)
			}
		})
	}
}

func TestHealthWithoutRequestIsManual(t *testing.T) {
	for _, params := range []string{"w http.ResponseWriter", "w http.ResponseWriter, _ *http.Request"} {
		f := filepath.Join(t.TempDir(), "h.go")
		src := hdr + "func handleHealth(" + params + ") {\n" +
			`_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})` + "\n}\n"
		if err := os.WriteFile(f, []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
		hs := healthEdit([]string{f})
		if hs == nil || !hs.manual {
			t.Fatalf("unavailable request context should require manual wiring: %+v", hs)
		}
	}
}
