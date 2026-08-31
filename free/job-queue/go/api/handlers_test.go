package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"
)

// newTestServer returns a minimal Server with a real logger for unit tests
// that do not require Redis or Postgres.
func newTestServer() *Server {
	return &Server{
		log: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
	}
}

// TestDispatch_UnknownJobType verifies that dispatch returns a clear error
// for unregistered job types rather than silently returning nil (which would
// cause jobs to be falsely marked "completed" without any work being done).
func TestDispatch_UnknownJobType(t *testing.T) {
	s := newTestServer()

	err := s.dispatch(context.Background(), "unknown_job_type", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("dispatch must return an error for an unregistered job type; got nil")
	}
	if !strings.Contains(err.Error(), "unknown_job_type") {
		t.Errorf("error should mention the job type; got: %v", err)
	}
}

// TestDispatch_SilentSuccessRegression ensures dispatch never returns nil for
// arbitrary job types, preventing the regression where all jobs were
// completing silently without handler execution.
func TestDispatch_SilentSuccessRegression(t *testing.T) {
	s := newTestServer()

	jobTypes := []string{"email", "ai", "media", "default", "some_future_type"}
	for _, jt := range jobTypes {
		err := s.dispatch(context.Background(), jt, json.RawMessage(`{}`))
		if err == nil {
			t.Errorf("dispatch(%q) returned nil — job would be silently completed without handler", jt)
		}
	}
}
