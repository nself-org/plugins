// Package httpmid provides HTTP middleware for distributed tracing across
// nself FREE plugins. MIT licensed. This module (free/shared) is the free
// counterpart of the internal plugins-pro/paid/shared package. The two
// intentionally carry duplicate copies of this utility so no MIT-licensed
// free plugin ever depends on Source-Available paid code (P6 ADR-P6-02;
// .claude/inbox 2026-08-31 paid-shared PCI).
//
// The central primitive is RequestIDMiddleware: it reads X-Request-ID from
// the incoming request (generating a new UUID v4 when absent), stores the ID
// on the request context, and echoes it back in the response header. Any
// outbound HTTP client built via httpclient.New will then pull the ID back
// out of context and attach it to downstream requests, so a single user
// request is traceable across ai -> claw -> mux -> google -> notify.
package httpmid

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// HeaderName is the canonical HTTP header used for request tracing.
const HeaderName = "X-Request-ID"

// contextKey is a private type so external packages cannot accidentally
// collide with this package's context key.
type contextKey struct{}

var requestIDKey = contextKey{}

// RequestIDMiddleware generates a new UUID v4 for every incoming request by
// default. The ID is stored on the request context (retrievable via
// FromContext) and echoed in the response header.
//
// Post-SIEGE (2026-04-16) policy: client-supplied X-Request-ID values are
// IGNORED unless they pass validateInternalRequestID (i.e. already look like
// a UUID that came from a trusted upstream plugin). This prevents a user
// from forging X-Request-ID to co-mingle their traces with another session's
// logs. Internal plugin-to-plugin calls via httpclient.New still propagate
// the ID correctly because httpclient writes a fresh UUID into the header.
//
// This middleware is idempotent — wrapping the same router twice is harmless
// because the inner invocation sees the UUID the outer one already issued.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderName)
		if !validateInternalRequestID(id) {
			id = uuid.NewString()
		}
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set(HeaderName, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validateInternalRequestID returns true when id parses as a UUID. Any
// other shape (including empty, short strings, or attacker-supplied
// ids) is rejected so a fresh UUID is issued instead.
func validateInternalRequestID(id string) bool {
	if id == "" {
		return false
	}
	_, err := uuid.Parse(id)
	return err == nil
}

// FromContext returns the request ID stored on ctx, or "" if none is set.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// WithRequestID returns a copy of ctx carrying id. Use this when you need to
// seed an ID onto a context that did not flow through an HTTP handler (e.g.,
// in a background job that spawns a traceable request).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}
