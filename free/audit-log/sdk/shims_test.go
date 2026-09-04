package sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadConfig_Defaults(t *testing.T) {
	// Without env vars, defaults should apply
	t.Setenv("PORT", "")
	t.Setenv("DATABASE_URL", "")
	cfg := LoadConfig()
	if cfg.Port != 3000 {
		t.Errorf("Port = %d, want 3000", cfg.Port)
	}
	if cfg.DatabaseURL != "" {
		t.Errorf("DatabaseURL = %q, want empty", cfg.DatabaseURL)
	}
}

func TestLoadConfig_CustomPort(t *testing.T) {
	t.Setenv("PORT", "8080")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	cfg := LoadConfig()
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.DatabaseURL != "postgres://localhost/test" {
		t.Errorf("DatabaseURL = %q, want postgres://localhost/test", cfg.DatabaseURL)
	}
}

func TestLoadConfig_InvalidPort(t *testing.T) {
	t.Setenv("PORT", "not-a-number")
	cfg := LoadConfig()
	if cfg.Port != 3000 {
		t.Errorf("invalid PORT should default to 3000, got %d", cfg.Port)
	}
}

func TestNewServer(t *testing.T) {
	s := NewServer(9999)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
	if s.Router() == nil {
		t.Error("Router() returned nil")
	}
}

func TestRecovery_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Recovery(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("Recovery passthrough status = %d, want 200", w.Code)
	}
}

func TestLogger_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := Logger(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test-path", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("Logger passthrough status = %d, want 200", w.Code)
	}
}

func TestCORS_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := CORS(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("CORS passthrough status = %d, want 200", w.Code)
	}
}

func TestSourceAccountID_AllSpellings(t *testing.T) {
	cases := []struct {
		header string
		value  string
		want   string
	}{
		{"X-Source-Account-ID", "acct-1", "acct-1"},
		{"X-Source-Account-Id", "acct-2", "acct-2"},
		{"X-Hasura-Source-Account-Id", "acct-3", "acct-3"},
		{"X-Source-Account", "acct-4", "acct-4"},
		{"", "", "primary"}, // no header → default
	}
	for _, tc := range cases {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if tc.header != "" {
			r.Header.Set(tc.header, tc.value)
		}
		got := SourceAccountID(r)
		if got != tc.want {
			t.Errorf("SourceAccountID with header %q = %q, want %q", tc.header, got, tc.want)
		}
	}
}

func TestRequestID_Passthrough(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	handler := RequestID(inner)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("RequestID passthrough status = %d, want 200", w.Code)
	}
}
