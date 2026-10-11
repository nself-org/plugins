package main

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const hdr = "package p\n\nimport (\n\t\"encoding/json\"\n\t\"net/http\"\n)\n\n"

func TestHealthEdit(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // substring of the edited file; "" means health must be manual
	}{
		{"funcdecl handler", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"funclit handler on /health route", `func routes(r router) {
	r.Get("/health", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"named writer rw, multiline literal", `func Health(rw http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(rw).Encode(map[string]any{
		"status": "ok",
	})
}
`, `"migrations": bootMigrationsHealth(rw),`},
		{"nearest enclosing literal wins", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	f := func(out http.ResponseWriter) {
		_ = json.NewEncoder(out).Encode(map[string]any{"status": "ok"})
	}
	f(w)
}
`, `bootMigrationsHealth(out)`},
		{"WriteHeader before the literal", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"Encode(w) before the literal", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"ready": true})
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"http.Error in a returning guard branch is fine", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"http.Error in a branch that does not return", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"http.Error in an else branch that returns is fine", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	if ok(r) {
		x = 1
	} else {
		http.Error(w, "no", http.StatusBadRequest)
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"Write before the literal", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte("x"))
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"helper taking the writer before the literal", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	setCORS(w)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"returning branch inside a closure before the literal", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	f := func() {
		if r == nil {
			w.Write(nil)
			return
		}
	}
	f()
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, ""},
		{"Header().Set before the literal is fine", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"literal passed straight to a helper taking the writer", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	respond(w, map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"call on another writer before the literal is fine", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	log.Print(other)
	fmt.Fprintf(os.Stderr, "x")
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"WriteHeader after the literal is fine", `func handleHealth(w http.ResponseWriter, r *http.Request) {
	body := map[string]any{"status": "ok"}
	_ = json.NewEncoder(w).Encode(body)
	w.WriteHeader(http.StatusOK)
}
`, `"migrations": bootMigrationsHealth(w)`},
		{"no writer parameter", `func healthBody() map[string]any {
	return map[string]any{"status": "ok"}
}
`, ""},
		{"blank writer parameter", `func handleHealth(_ http.ResponseWriter, r *http.Request) {
	_ = map[string]any{"status": "ok"}
}
`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "h.go")
			if err := os.WriteFile(f, []byte(hdr+c.body), 0o644); err != nil {
				t.Fatal(err)
			}
			hs := healthEdit([]string{f})
			if hs == nil {
				t.Fatal("health literal not found")
			}
			if c.want == "" {
				if !hs.manual {
					t.Fatalf("want manual, got an edit: %+v", hs.edit)
				}
				return
			}
			if hs.manual {
				t.Fatal("want wired, got manual")
			}
			if err := applyEdits(f, []edit{hs.edit}); err != nil {
				t.Fatal(err)
			}
			out, _ := os.ReadFile(f)
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("edited file lacks %q:\n%s", c.want, out)
			}
		})
	}
}

func TestHealthEditSkipsWiredAndNonStatus(t *testing.T) {
	src := hdr + `func handleHealth(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok", "migrations": bootMigrationsHealth(w)})
	_ = json.NewEncoder(w).Encode(map[string]any{"other": 1})
}
`
	f := filepath.Join(t.TempDir(), "h.go")
	if err := os.WriteFile(f, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if hs := healthEdit([]string{f}); hs != nil {
		t.Fatalf("want nil, got %+v", hs)
	}
}

// The template is what every converted plugin copies: it must parse, gofmt cleanly and keep the 503 shape.
func TestHelperTemplate(t *testing.T) {
	body := fmt.Sprintf(helperTmpl, "main", "example.com/x", "np_x")
	out, err := format.Source([]byte(body))
	if err != nil {
		t.Fatalf("template does not parse: %v", err)
	}
	if string(out) != body {
		t.Error("template is not gofmt-clean")
	}
	for _, want := range []string{
		"func bootMigrationsHealth(w http.ResponseWriter) map[string]int",
		`log.Printf("migrations: status: %v", err)`,
		"w.WriteHeader(http.StatusServiceUnavailable)",
		`"net/http"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("template lacks %q", want)
		}
	}
}
