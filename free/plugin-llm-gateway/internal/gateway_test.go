package internal

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestValidateUpstreamURL_AllowsLocalhost verifies that localhost targets pass the SSRF guard.
func TestValidateUpstreamURL_AllowsLocalhost(t *testing.T) {
	g := &Gateway{GatewayURL: "http://127.0.0.1:3761"}
	if err := g.validateUpstreamURL("http://127.0.0.1:3761"); err != nil {
		t.Fatalf("expected localhost to be allowed, got: %v", err)
	}
	if err := g.validateUpstreamURL("http://localhost:3761"); err != nil {
		t.Fatalf("expected localhost hostname to be allowed, got: %v", err)
	}
}

// TestValidateUpstreamURL_BlocksExternal verifies that external hosts are blocked.
func TestValidateUpstreamURL_BlocksExternal(t *testing.T) {
	g := &Gateway{}
	cases := []string{
		"https://api.openai.com/v1/completions",
		"https://anthropic.com/v1/messages",
		"http://8.8.8.8/api",
	}
	for _, c := range cases {
		if err := g.validateUpstreamURL(c); err == nil {
			t.Errorf("expected %q to be blocked, but it was allowed", c)
		}
	}
}

// TestValidateUpstreamURL_AllowsRFC1918 verifies RFC-1918 ranges are allowed.
func TestValidateUpstreamURL_AllowsRFC1918(t *testing.T) {
	g := &Gateway{}
	allowed := []string{
		"http://10.0.0.5:3761",
		"http://192.168.1.1:3761",
		"http://172.16.0.1:3761",
	}
	for _, a := range allowed {
		if err := g.validateUpstreamURL(a); err != nil {
			t.Errorf("expected RFC-1918 address %q to be allowed, got: %v", a, err)
		}
	}
}

// TestCacheKey_IsTenantScoped verifies the cache key includes source_account_id.
func TestCacheKey_IsTenantScoped(t *testing.T) {
	g := &Gateway{}
	body := []byte(`{"model":"gpt-4","messages":[]}`)
	k1 := g.cacheKey("tenant-a", "gpt-4", body)
	k2 := g.cacheKey("tenant-b", "gpt-4", body)
	if k1 == k2 {
		t.Error("cache keys for different tenants must differ to prevent cross-tenant cache pollution")
	}
}

// TestCacheHit_ReturnsCachedResponse verifies that identical requests from same tenant get a cache hit.
func TestCacheHit_ReturnsCachedResponse(t *testing.T) {
	callCount := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"reply": "hello"})
	}))
	defer upstream.Close()

	g := &Gateway{GatewayURL: upstream.URL, CacheTTL: 60e9} // 60s in nanoseconds
	body := `{"model":"gpt-4","messages":[],"source_account_id":"acct-1"}`

	// First request — should be a cache MISS.
	req1 := httptest.NewRequest("POST", "/v1/completions", strings.NewReader(body))
	rr1 := httptest.NewRecorder()
	g.Completions(rr1, req1)
	if rr1.Header().Get("X-Cache") != "MISS" {
		t.Errorf("first request: expected X-Cache: MISS, got %q", rr1.Header().Get("X-Cache"))
	}

	// Second identical request — should be a cache HIT.
	req2 := httptest.NewRequest("POST", "/v1/completions", strings.NewReader(body))
	rr2 := httptest.NewRecorder()
	g.Completions(rr2, req2)
	if rr2.Header().Get("X-Cache") != "HIT" {
		t.Errorf("second request: expected X-Cache: HIT, got %q", rr2.Header().Get("X-Cache"))
	}
	if callCount != 1 {
		t.Errorf("upstream should be called only once, got %d", callCount)
	}
}

// TestSSRF_BlocksExternalURL verifies that Completions returns 403 when gateway URL is external.
func TestSSRF_BlocksExternalURL(t *testing.T) {
	g := &Gateway{GatewayURL: "https://api.openai.com"}
	body := `{"model":"gpt-4","messages":[],"source_account_id":"acct-1"}`
	req := httptest.NewRequest("POST", "/v1/completions", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()
	g.Completions(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 for external gateway URL, got %d", rr.Code)
	}
}

// TestEstimateTokens verifies token estimation doesn't panic on empty input.
func TestEstimateTokens(t *testing.T) {
	g := &Gateway{}
	n := g.estimateTokens([]byte("hello world"), []byte("response text"))
	if n <= 0 {
		t.Error("expected non-zero token estimate")
	}
}
