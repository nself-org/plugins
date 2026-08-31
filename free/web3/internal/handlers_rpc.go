package internal

// Purpose: Server-side RPC proxy that forwards JSON-RPC calls to chain nodes.
// Inputs: chainId path param, JSON-RPC body from client.
// Outputs: JSON-RPC response from the upstream node.
// Constraints: RPC URLs come from WEB3_RPC_URL_<CHAINID> env; never exposed to client.
//   SSRF guard: only pre-configured chain IDs accepted; no arbitrary URL forwarding.
// SPORT: POST /api/v1/rpc/{chainId}

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var rpcClient = &http.Client{Timeout: 30 * time.Second}

// RPCBridge handles POST /api/v1/rpc/{chainId}
// Looks up WEB3_RPC_URL_<CHAINID> env, proxies the request body verbatim,
// and returns the upstream response. Returns 503 if the chain is not configured.
func (h *Handlers) RPCBridge(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chainId")

	// Validate chainId is a safe numeric-ish string (prevents env var injection).
	if !isSafeChainID(chainID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid chainId"})
		return
	}

	envKey := "WEB3_RPC_URL_" + strings.ToUpper(chainID)
	rpcURL := os.Getenv(envKey)
	if rpcURL == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("chain %s not configured (set %s)", chainID, envKey),
		})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB cap
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot read body"})
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, rpcURL, bytes.NewReader(body))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprintf("build request: %v", err)})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := rpcClient.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("upstream: %v", err)})
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck // best-effort stream copy
}

// isSafeChainID permits only digit strings up to 12 chars (covers all current EVM chains).
func isSafeChainID(s string) bool {
	if len(s) == 0 || len(s) > 12 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
