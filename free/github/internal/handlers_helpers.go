package internal

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

// --- Helpers -----------------------------------------------------------------

// parsePagination extracts limit and offset from query parameters with defaults.
func parsePagination(r *http.Request) (int, int) {
	limit := 100
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

// parseOptionalInt64 extracts an optional int64 query parameter.
func parseOptionalInt64(r *http.Request, param string) *int64 {
	v := r.URL.Query().Get(param)
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("[github:server] Failed to encode response: %v", err)
	}
}

// writeErr writes an error response and logs it.
func writeErr(w http.ResponseWriter, msg string, err error) {
	log.Printf("[github:server] %s: %v", msg, err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": msg})
}

