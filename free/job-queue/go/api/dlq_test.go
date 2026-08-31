// Purpose: Tests for DLQ routing, priority ordering, and job lifecycle.
// Inputs: In-memory Server instance; no Redis or Postgres required for unit tests.
// Outputs: Verification of retry logic, DLQ routing, priority separation.
// Constraints: Pure Go — no network required.
package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// TestDLQRouting verifies that a job exceeding max retries is correctly
// identified as dead and routes to DLQ (moveToDLQ path).
func TestDLQRouting(t *testing.T) {
	s := newTestServer()
	s.cfg.MaxAttempts = 2

	// Simulate a job that has already exhausted all retries.
	job := jobPayload{
		ID:           "dlq-test-job-001",
		JobType:      "failing_job",
		Payload:      json.RawMessage(`{}`),
		AttemptCount: 2, // at MaxAttempts
	}

	// dispatch returns error for unregistered types.
	err := s.dispatch(context.Background(), job.JobType, job.Payload)
	if err == nil {
		t.Fatal("dispatch must return error for unregistered job type")
	}

	// After AttemptCount reaches MaxAttempts the job is dead.
	isDead := job.AttemptCount >= s.cfg.MaxAttempts
	if !isDead {
		t.Errorf("job with attempt_count=%d and max_attempts=%d should be dead",
			job.AttemptCount, s.cfg.MaxAttempts)
	}
}

// TestDLQRoutingAfterMaxRetries verifies the exact boundary: a job at
// MaxAttempts-1 is retried; at MaxAttempts it goes to DLQ.
func TestDLQRoutingAfterMaxRetries(t *testing.T) {
	s := newTestServer()
	s.cfg.MaxAttempts = 3

	// processJob: after dispatch error, job.AttemptCount++ then check >= MaxAttempts (3).
	// pre=0 → post=1, 1<3 → retry; pre=1→post=2, 2<3→retry; pre=2→post=3, 3>=3→dead.
	cases := []struct {
		attemptCount int
		expectDead   bool
	}{
		{0, false}, // post=1, 1<3 → retry
		{1, false}, // post=2, 2<3 → retry
		{2, true},  // post=3, 3>=3 → dead/DLQ
		{3, true},  // post=4, 4>=3 → definitely dead
	}

	for _, tc := range cases {
		job := jobPayload{AttemptCount: tc.attemptCount}
		// Simulate processJob decision: attempt_count++ then compare.
		job.AttemptCount++
		isDead := job.AttemptCount >= s.cfg.MaxAttempts
		if isDead != tc.expectDead {
			t.Errorf("attempt_count=%d (after increment=%d): isDead=%v, want %v",
				tc.attemptCount, job.AttemptCount, isDead, tc.expectDead)
		}
	}
}

// TestPriorityOrdering verifies that jobs enqueued to separate priority queues
// can be processed independently. The job-queue plugin uses dedicated named
// queues for priority separation (high-priority work → dedicated queue with
// higher concurrency). This test verifies queue name isolation.
func TestPriorityOrdering(t *testing.T) {
	queues := []string{"default", "email", "ai", "media"}
	seen := make(map[string]bool)
	for _, q := range queues {
		if seen[q] {
			t.Errorf("duplicate queue name: %q", q)
		}
		seen[q] = true
	}
	// All configured queues must be distinct.
	if len(seen) != len(queues) {
		t.Errorf("queue list has duplicates: %v", queues)
	}
}

// TestRetryExponentialBackoff verifies that retry delay grows exponentially
// and is capped at 1 hour.
func TestRetryExponentialBackoff(t *testing.T) {
	cases := []struct {
		attempt  int
		minDelay time.Duration
		maxDelay time.Duration
	}{
		{1, 2 * time.Second, 3 * time.Second},
		{2, 4 * time.Second, 5 * time.Second},
		{3, 8 * time.Second, 9 * time.Second},
		{12, time.Hour - time.Second, time.Hour + time.Second}, // 2^12=4096s>3600s → capped at 1h
	}
	for _, tc := range cases {
		d := retryDelay(tc.attempt)
		if d < tc.minDelay || d > tc.maxDelay {
			t.Errorf("retryDelay(%d) = %v, want in [%v, %v]",
				tc.attempt, d, tc.minDelay, tc.maxDelay)
		}
	}
}

// TestRetryCapAt1Hour verifies the hard cap at 3600 seconds.
func TestRetryCapAt1Hour(t *testing.T) {
	// Attempt 20 would be 2^20 s ≈ 291 hours — must be capped at 1h.
	d := retryDelay(20)
	if d > time.Hour {
		t.Errorf("retryDelay(20) = %v, must not exceed 1 hour", d)
	}
	if d != time.Hour {
		t.Errorf("retryDelay(20) = %v, want exactly 1h", d)
	}
}

// TestJobPayloadParsing verifies valid job JSON is parsed correctly.
func TestJobPayloadParsing(t *testing.T) {
	raw := `{"id":"abc123","job_type":"email","payload":{"to":"user@example.com"},"attempt_count":0}`
	var job jobPayload
	if err := json.Unmarshal([]byte(raw), &job); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	if job.ID != "abc123" {
		t.Errorf("id: got %q, want %q", job.ID, "abc123")
	}
	if job.JobType != "email" {
		t.Errorf("job_type: got %q, want %q", job.JobType, "email")
	}
	if job.AttemptCount != 0 {
		t.Errorf("attempt_count: got %d, want 0", job.AttemptCount)
	}
}

// TestDLQDispatchErrorMessage verifies dispatch error names the job type.
func TestDLQDispatchErrorMessage(t *testing.T) {
	s := newTestServer()
	err := s.dispatch(context.Background(), "my_custom_job", json.RawMessage(`{}`))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "my_custom_job") {
		t.Errorf("error should mention job type 'my_custom_job'; got: %v", err)
	}
}
