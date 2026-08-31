// Purpose: Tests for the RPCBridge proxy handler and its chainId safety guard.
// Inputs: httptest requests with various chainId path params and WEB3_RPC_URL_<CHAINID> env.
// Outputs: asserts 400 on unsafe chainId, 503 when unconfigured, 200 proxying an upstream stub.
// Constraints: No real chain RPC; uses httptest.Server as the upstream stub.
package internal

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func reqWithChainID(method, url, body, chainID string) *http.Request {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, url, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, url, nil)
	}
	return reqWithChiParam2(r, "chainId", chainID)
}

func TestIsSafeChainID(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid short", "1", true},
		{"valid multi-digit", "137", true},
		{"empty", "", false},
		{"too long", "1234567890123", false},
		{"non-numeric", "abc", false},
		{"injection attempt", "1;DROP", false},
		{"max length ok", "123456789012", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSafeChainID(tc.in); got != tc.want {
				t.Errorf("isSafeChainID(%q) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRPCBridge_UnsafeChainID(t *testing.T) {
	h := &Handlers{}
	rec := httptest.NewRecorder()
	req := reqWithChainID(http.MethodPost, "/api/v1/rpc/abc", "", "abc")
	h.RPCBridge(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRPCBridge_ChainNotConfigured(t *testing.T) {
	os.Unsetenv("WEB3_RPC_URL_1")
	h := &Handlers{}
	rec := httptest.NewRecorder()
	req := reqWithChainID(http.MethodPost, "/api/v1/rpc/1", "", "1")
	h.RPCBridge(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestRPCBridge_ProxiesUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"jsonrpc":"2.0","result":"0x1","id":1}`))
	}))
	defer upstream.Close()

	os.Setenv("WEB3_RPC_URL_1", upstream.URL)
	defer os.Unsetenv("WEB3_RPC_URL_1")

	h := &Handlers{}
	rec := httptest.NewRecorder()
	req := reqWithChainID(http.MethodPost, "/api/v1/rpc/1", `{"jsonrpc":"2.0","method":"eth_blockNumber","id":1}`, "1")
	h.RPCBridge(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"result":"0x1"`) {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestRPCBridge_UpstreamUnreachable(t *testing.T) {
	os.Setenv("WEB3_RPC_URL_1", "http://127.0.0.1:1")
	defer os.Unsetenv("WEB3_RPC_URL_1")

	h := &Handlers{}
	rec := httptest.NewRecorder()
	req := reqWithChainID(http.MethodPost, "/api/v1/rpc/1", "{}", "1")
	h.RPCBridge(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
}
