package main

// envelope.go — the /v1 wire envelope: {"error":{"code","message"}} errors
// and JSON success bodies.
//
// Purpose: one choke point for the response contract consumed by
//   cli/internal/sentryapi (nself sentry CLI + MCP tools). Non-2xx bodies
//   from upstream plugins arrive in assorted shapes ({"error":"str"},
//   quota JSON, plain text) and are re-enveloped here so clients see one
//   stable format.
// Outputs: writeJSON / writeErr / relayUpstreamError.
// Constraints: 402/429 map to code "quota_exceeded" (client shows the
//   upgrade hint), 401 → "unauthorized", 404 → "not_found".

import (
	"encoding/json"
	"net/http"
	"strings"
)

// writeJSON writes v as a JSON response with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr writes the contract error envelope.
func writeErr(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}

// codeForStatus maps an HTTP status to the contract error code.
func codeForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusPaymentRequired, http.StatusTooManyRequests:
		return "quota_exceeded"
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return "invalid_request"
	}
	if status >= 500 {
		return "upstream_error"
	}
	return "error"
}

// upstreamMessage extracts a human message from an upstream error body.
// Understands {"error":"..."} (plugins), {"message":"..."} (quota JSON),
// and falls back to the trimmed raw body.
func upstreamMessage(body []byte) string {
	var probe struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if err := json.Unmarshal(body, &probe); err == nil {
		if probe.Message != "" {
			return probe.Message
		}
		var s string
		if json.Unmarshal(probe.Error, &s) == nil && s != "" {
			return s
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	if s == "" {
		s = "upstream error"
	}
	return s
}

// relayUpstreamError re-envelopes a non-2xx upstream response, preserving
// the status code (quota 402/429 pass through so clients see the upgrade
// hint) and normalizing the body to the contract error shape.
func relayUpstreamError(w http.ResponseWriter, status int, body []byte) {
	writeErr(w, status, codeForStatus(status), upstreamMessage(body))
}
