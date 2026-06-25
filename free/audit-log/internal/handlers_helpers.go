package internal

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func authorizeAdminRequest(r *http.Request, secret, adminSecret string) bool {
	if ps := r.Header.Get("X-Plugin-Secret"); ps != "" && ps == secret {
		return true
	}
	if adminSecret != "" {
		if as := r.Header.Get("X-Hasura-Admin-Secret"); as != "" && as == adminSecret {
			return true
		}
	}
	return false
}

// parseQueryFilter builds a QueryFilter from the request's URL query parameters.
// Returns an error for any malformed parameter so the caller can return HTTP 400.
// Size-cap exception: single-responsibility HTTP route handler — 58L of request decode + validate + DB op + response encode; splitting adds indirection without cohesion gain.
func parseQueryFilter(r *http.Request) (QueryFilter, error) {
	q := r.URL.Query()

	f := QueryFilter{
		EventType:       q.Get("event_type"),
		ActorUserID:     q.Get("actor_user_id"),
		Severity:        q.Get("severity"),
		SourceAccountID: q.Get("source_account_id"),
	}

	// Parse limit (cap at 1000).
	f.Limit = 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return QueryFilter{}, fmt.Errorf("invalid 'limit' parameter")
		}
		if n > 1000 {
			n = 1000
		}
		f.Limit = n
	}

	// Parse offset.
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return QueryFilter{}, fmt.Errorf("invalid 'offset' parameter")
		}
		f.Offset = n
	}

	// Parse optional time bounds (from/to).
	if v := q.Get("from"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return QueryFilter{}, fmt.Errorf("invalid 'from' timestamp; expected RFC 3339")
		}
		f.From = &t
	}
	if v := q.Get("to"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return QueryFilter{}, fmt.Errorf("invalid 'to' timestamp; expected RFC 3339")
		}
		f.To = &t
	}

	// Validate enum filters if provided.
	if f.EventType != "" && !validEventTypes[f.EventType] {
		return QueryFilter{}, fmt.Errorf("invalid event_type filter")
	}
	if f.Severity != "" && !validSeverities[f.Severity] {
		return QueryFilter{}, fmt.Errorf("invalid severity filter; accepted values: info, warning, critical")
	}

	return f, nil
}

// realIP extracts the client IP from standard proxy headers or falls back to
// the remote address.
func realIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// X-Forwarded-For can be a comma-separated list; the leftmost is the
		// original client.
		parts := strings.SplitN(v, ",", 2)
		return strings.TrimSpace(parts[0])
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	if idx := strings.LastIndex(addr, ":"); idx != -1 {
		addr = addr[:idx]
	}
	return addr
}
