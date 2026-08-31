package internal

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	saved := map[string]string{
		"PORT":         os.Getenv("PORT"),
		"DATABASE_URL": os.Getenv("DATABASE_URL"),
	}
	os.Unsetenv("PORT")
	os.Unsetenv("DATABASE_URL")
	defer func() {
		for k, v := range saved {
			if v == "" {
				os.Unsetenv(k)
			} else {
				os.Setenv(k, v)
			}
		}
	}()

	c := LoadConfig()
	if c.Port != "3106" {
		t.Errorf("default Port = %q; want 3106", c.Port)
	}
	if !strings.HasPrefix(c.DatabaseURL, "postgres://") {
		t.Errorf("default DatabaseURL = %q; want fallback dsn", c.DatabaseURL)
	}
}

func TestLoadConfig_EnvOverrides(t *testing.T) {
	os.Setenv("PORT", "9999")
	os.Setenv("DATABASE_URL", "postgres://x")
	defer func() {
		os.Unsetenv("PORT")
		os.Unsetenv("DATABASE_URL")
	}()
	c := LoadConfig()
	if c.Port != "9999" || c.DatabaseURL != "postgres://x" {
		t.Errorf("overrides not applied: %+v", c)
	}
}

func TestSA_Header(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	r.Header.Set("X-Source-Account-ID", "acct-7")
	if got := h.sa(r); got != "acct-7" {
		t.Errorf("sa = %q", got)
	}
}

func TestSA_Default(t *testing.T) {
	h := &Handlers{}
	r := httptest.NewRequest("GET", "/x", nil)
	if got := h.sa(r); got != "primary" {
		t.Errorf("sa default = %q", got)
	}
}

func TestWriteJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, map[string]string{"k": "v"})
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Error("content-type missing")
	}
}

func TestGenerateToken_Length(t *testing.T) {
	tok := generateToken()
	if len(tok) != 32 { // 16 bytes hex-encoded
		t.Errorf("token length = %d; want 32", len(tok))
	}
	if _, err := hex.DecodeString(tok); err != nil {
		t.Errorf("token not valid hex: %v", err)
	}
}

func TestGenerateToken_Uniqueness(t *testing.T) {
	a := generateToken()
	b := generateToken()
	if a == b {
		t.Error("two consecutive tokens should differ")
	}
}

func TestRenderTemplate_BasicSubstitution(t *testing.T) {
	tmpl := &Template{TemplateContent: "Hello {{name}}!"}
	got := renderTemplate(tmpl, map[string]any{"name": "World"})
	if got != "Hello World!" {
		t.Errorf("render = %q", got)
	}
}

func TestRenderTemplate_MissingKeyEmpty(t *testing.T) {
	tmpl := &Template{TemplateContent: "Hi {{absent}}"}
	got := renderTemplate(tmpl, map[string]any{})
	if got != "Hi " {
		t.Errorf("missing key should render empty; got %q", got)
	}
}

func TestRenderTemplate_CSSWrap(t *testing.T) {
	css := ".x{}"
	tmpl := &Template{TemplateContent: "<p>x</p>", CSSContent: &css}
	got := renderTemplate(tmpl, nil)
	if !strings.Contains(got, "<style>.x{}</style>") {
		t.Errorf("CSS not injected: %q", got)
	}
}

func TestRenderTemplate_HeaderFooter(t *testing.T) {
	hdr := "<header/>"
	ftr := "<footer/>"
	tmpl := &Template{TemplateContent: "BODY", HeaderContent: &hdr, FooterContent: &ftr}
	got := renderTemplate(tmpl, nil)
	if !strings.HasPrefix(got, "<header/>") || !strings.HasSuffix(got, "<footer/>") {
		t.Errorf("header/footer not applied: %q", got)
	}
}

func TestHealth(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/health", nil)
	h.Health(rec, r)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d", rec.Code)
	}
	var out map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" || out["plugin"] != "documents" {
		t.Errorf("body = %v", out)
	}
}

func TestNewHandlers(t *testing.T) {
	h := NewHandlers(nil)
	if h == nil {
		t.Error("NewHandlers should not be nil")
	}
}
