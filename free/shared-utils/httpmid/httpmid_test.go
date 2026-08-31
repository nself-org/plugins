package httpmid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestIDMiddleware_GeneratesWhenAbsent(t *testing.T) {
	var captured string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured == "" {
		t.Fatal("expected request ID to be generated on context")
	}
	if got := rec.Header().Get(HeaderName); got != captured {
		t.Fatalf("response header %q did not match context ID %q", got, captured)
	}
	if len(captured) < 16 {
		t.Fatalf("generated request ID looks too short: %q", captured)
	}
}

func TestRequestIDMiddleware_PreservesValidIncoming(t *testing.T) {
	const want = "550e8400-e29b-41d4-a716-446655440000" // valid UUID v4
	var captured string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderName, want)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured != want {
		t.Fatalf("FromContext = %q, want %q", captured, want)
	}
	if got := rec.Header().Get(HeaderName); got != want {
		t.Fatalf("response header = %q, want %q", got, want)
	}
}

// TestRequestIDMiddleware_RejectsInvalidIncoming locks in the SIEGE MEDIUM-1
// hardening: arbitrary attacker-supplied X-Request-ID values must be dropped
// and replaced with a freshly-minted UUID to prevent trace-poisoning /
// log-comingling attacks.
func TestRequestIDMiddleware_RejectsInvalidIncoming(t *testing.T) {
	const attacker = "abc-123-xyz"
	var captured string
	h := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set(HeaderName, attacker)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if captured == attacker {
		t.Fatalf("attacker-supplied non-UUID id was preserved: %q", captured)
	}
	if len(captured) < 32 {
		t.Fatalf("regenerated id looks short: %q", captured)
	}
}

func TestFromContext_Empty(t *testing.T) {
	if got := FromContext(nil); got != "" {
		t.Fatalf("FromContext(nil) = %q, want empty", got)
	}
}

func TestWithRequestID_RoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "seeded-id")
	if got := FromContext(ctx); got != "seeded-id" {
		t.Fatalf("FromContext = %q, want seeded-id", got)
	}
}
