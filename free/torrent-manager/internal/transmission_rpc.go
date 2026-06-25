package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// retry automatically.
// Size-cap exception: 54L — single-responsibility operation; splitting would create artificial fragmentation without structural or maintainability gain.
func (t *TransmissionClient) doRPC(method string, args interface{}) (json.RawMessage, error) {
	body, err := json.Marshal(rpcRequest{
		Method:    method,
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := t.sendRequest(body)
	if err != nil {
		return nil, err
	}

	// If we get a 409, Transmission sends us a new session ID.
	if resp.StatusCode == http.StatusConflict {
		newID := resp.Header.Get("X-Transmission-Session-Id")
		resp.Body.Close()
		if newID != "" {
			t.mu.Lock()
			t.sessionID = newID
			t.mu.Unlock()

			resp, err = t.sendRequest(body)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("409 conflict but no session ID header")
		}
	}

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	var rpcResp rpcResponse
	if err := json.Unmarshal(respBody, &rpcResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if rpcResp.Result != "success" {
		return nil, fmt.Errorf("rpc error: %s", rpcResp.Result)
	}

	return rpcResp.Arguments, nil
}

// sendRequest sends the raw JSON body to the Transmission RPC endpoint.
func (t *TransmissionClient) sendRequest(body []byte) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, t.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("X-Transmission-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	if t.username != "" {
		req.SetBasicAuth(t.username, t.password)
	}

	return t.client.Do(req)
}
