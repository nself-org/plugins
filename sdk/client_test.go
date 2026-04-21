package sdk

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestNewPluginClient_InjectsXSourcePlugin verifies that every request made
// via NewPluginClient carries X-Source-Plugin set to the plugin name.
// S43-T16 acceptance criterion: "built-in plugins emit X-Source-Plugin on all
// outgoing inter-plugin requests".
func TestNewPluginClient_InjectsXSourcePlugin(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Source-Plugin")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewPluginClient("claw", nil)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()

	if received != "claw" {
		t.Errorf("X-Source-Plugin: got %q, want %q", received, "claw")
	}
}

// TestNewPluginClient_NormalisesCase verifies that the plugin name is
// lowercased in X-Source-Plugin regardless of how it is passed.
func TestNewPluginClient_NormalisesCase(t *testing.T) {
	var received string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received = r.Header.Get("X-Source-Plugin")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewPluginClient("CLAW", nil)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()

	if received != "claw" {
		t.Errorf("X-Source-Plugin case: got %q, want %q", received, "claw")
	}
}

// TestNewPluginClient_WithIdentity_InjectsSignatureHeaders verifies that when
// an identity is provided, X-Plugin-Id, X-Plugin-Timestamp, and
// X-Plugin-Signature are set in addition to X-Source-Plugin. S43-T16.
func TestNewPluginClient_WithIdentity_InjectsSignatureHeaders(t *testing.T) {
	id, err := GenerateIdentity("claw")
	if err != nil {
		t.Fatalf("GenerateIdentity: %v", err)
	}

	var hdrs http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdrs = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewPluginClient("claw", id)
	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("client.Get: %v", err)
	}
	defer resp.Body.Close()

	if hdrs.Get("X-Source-Plugin") != "claw" {
		t.Errorf("X-Source-Plugin: got %q, want %q", hdrs.Get("X-Source-Plugin"), "claw")
	}
	if hdrs.Get("X-Plugin-Id") == "" {
		t.Error("X-Plugin-Id header missing when identity provided")
	}
	if hdrs.Get("X-Plugin-Timestamp") == "" {
		t.Error("X-Plugin-Timestamp header missing when identity provided")
	}
	if hdrs.Get("X-Plugin-Signature") == "" {
		t.Error("X-Plugin-Signature header missing when identity provided")
	}
}

// TestNewPluginClient_DoesNotMutateOriginalRequest verifies that the
// underlying request is not mutated (clone semantics). S43-T16.
func TestNewPluginClient_DoesNotMutateOriginalRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	client := NewPluginClient("claw", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}
	defer resp.Body.Close()

	// The original request's headers must NOT be mutated.
	if req.Header.Get("X-Source-Plugin") != "" {
		t.Errorf("original request was mutated: X-Source-Plugin = %q", req.Header.Get("X-Source-Plugin"))
	}
}
