package internal

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// newTestClient creates a StripeClient pointing at the given test server.
func newTestClient(serverURL, apiKey string) *StripeClient {
	c := NewStripeClient(apiKey)
	c.baseURL = serverURL
	return c
}

// TestStripeClient_Get_HappyPath verifies that the client GETs a URL, sends
// the Authorization header, and returns the response body.
func TestStripeClient_Get_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %q", r.Method)
		}
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("expected Authorization header 'Bearer test-key', got %q", auth)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"id": "cus_123"})
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	body, err := c.get("/v1/customers/cus_123", nil)
	if err != nil {
		t.Fatalf("get error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty body")
	}
}

// TestStripeClient_Get_Error verifies that a 4xx response returns an error.
func TestStripeClient_Get_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"Not Found"}}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, "test-key")
	_, err := c.get("/v1/customers/missing", nil)
	if err == nil {
		t.Error("expected error for 404 response, got nil")
	}
}

// TestStripeClient_Get_WithParams verifies that query parameters are appended.
func TestStripeClient_Get_WithParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected limit=10 in query, got %q", r.URL.Query().Get("limit"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	params := url.Values{}
	params.Set("limit", "10")
	c := newTestClient(srv.URL, "key")
	_, err := c.get("/v1/customers", params)
	if err != nil {
		t.Fatalf("get with params error: %v", err)
	}
}

// TestStripeClient_Post_HappyPath verifies a successful POST request.
func TestStripeClient_Post_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %q", r.Method)
		}
		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("expected form content-type, got %q", ct)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"cu_new"}`))
	}))
	defer srv.Close()

	params := url.Values{}
	params.Set("email", "user@example.com")
	c := newTestClient(srv.URL, "key")
	body, err := c.post("/v1/customers", params)
	if err != nil {
		t.Fatalf("post error: %v", err)
	}
	if len(body) == 0 {
		t.Error("expected non-empty response body")
	}
}

// TestStripeClient_WithAPIKey verifies that WithAPIKey returns a new client
// with the updated key but the same base URL and HTTP client.
func TestStripeClient_WithAPIKey(t *testing.T) {
	c := NewStripeClient("original-key")
	c2 := c.WithAPIKey("new-key")
	if c2 == c {
		t.Error("expected a new client instance")
	}
	if c2.apiKey != "new-key" {
		t.Errorf("expected apiKey 'new-key', got %q", c2.apiKey)
	}
	if c2.baseURL != c.baseURL {
		t.Errorf("expected same baseURL, got %q", c2.baseURL)
	}
}
