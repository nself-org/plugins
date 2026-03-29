package sdk

import (
	"encoding/json"
	"net/http"
)

// Respond writes a JSON response with the given status code and data.
func Respond(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// Error responds with a JSON error body: {"error": "..."}.
func Error(w http.ResponseWriter, status int, err error) {
	Respond(w, status, map[string]string{"error": err.Error()})
}
