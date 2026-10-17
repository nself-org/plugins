package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// APIError is the standard error envelope returned by all plugin endpoints.
type APIError struct {
	// Code is an optional machine-readable error code.
	Code string `json:"code,omitempty"`
	// Message is the human-readable error description.
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

// Response is a typed JSON response envelope used by plugin handlers.
//
// Purpose: eliminate map[string]interface{} response construction across the
// plugin ecosystem and restore compile-time type guarantees.
// Inputs:  T — any serialisable response data type.
// Outputs: JSON body { "data": T, "error": APIError?, "meta": map[string]string }.
// Constraints: Meta is optional; omit by leaving nil.
// SPORT: F08-SERVICE-INVENTORY — plugin SDK Response[T] typed envelope.
type Response[T any] struct {
	// Data holds the typed response payload.
	Data T `json:"data"`
	// Error is non-nil when the response represents a failure.
	Error *APIError `json:"error,omitempty"`
	// Meta carries optional key/value annotations (e.g. pagination cursors).
	Meta map[string]string `json:"meta,omitempty"`
}

// RespondTyped writes a typed JSON response envelope with compile-time safety.
// Use this in preference to RespondRaw for all new plugin handlers.
func RespondTyped[T any](w http.ResponseWriter, status int, data T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response[T]{Data: data})
}

// RespondTypedMeta writes a typed JSON response envelope with optional metadata.
func RespondTypedMeta[T any](w http.ResponseWriter, status int, data T, meta map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Response[T]{Data: data, Meta: meta})
}

// Respond writes a JSON response with the given status code and body.
// Deprecated: prefer RespondTyped[T] for new code.
// If body is nil, only the status code is written (no response body).
func Respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// RespondRaw is an alias for Respond retained for backward compatibility.
// Deprecated: prefer RespondTyped[T] for new code.
func RespondRaw(w http.ResponseWriter, status int, body interface{}) {
	Respond(w, status, body)
}

// Error writes a JSON error response with the given status code and message.
// message may be a string, an error, or any value (formatted via fmt.Sprintf).
func Error(w http.ResponseWriter, status int, message any) {
	var msg string
	switch m := message.(type) {
	case error:
		msg = m.Error()
	case string:
		msg = m
	default:
		msg = fmt.Sprintf("%v", m)
	}
	Respond(w, status, map[string]string{"error": msg})
}
