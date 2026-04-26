package sdk

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Respond writes a JSON response with the given status code and body.
// If body is nil, only the status code is written (no response body).
func Respond(w http.ResponseWriter, status int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
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
