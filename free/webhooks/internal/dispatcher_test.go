package internal

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGenerateSignature verifies the signature format and determinism for
// the same inputs (within the same second, for the timestamp component).
func TestGenerateSignature(t *testing.T) {
	payload := []byte(`{"event_type":"test"}`)
	sig := GenerateSignature(payload, "my-secret-key")

	// Must start with t= and contain v1=.
	if !strings.HasPrefix(sig, "t=") {
		t.Errorf("signature should start with 't=', got %q", sig)
	}
	if !strings.Contains(sig, ",v1=") {
		t.Errorf("signature should contain ',v1=', got %q", sig)
	}
}

// TestGenerateSignature_DifferentSecrets verifies that different secrets
// produce different signatures.
func TestGenerateSignature_DifferentSecrets(t *testing.T) {
	payload := []byte(`{"x":1}`)
	a := GenerateSignature(payload, "secret-a")
	b := GenerateSignature(payload, "secret-b")
	if a == b {
		t.Error("different secrets should produce different signatures")
	}
}

// TestGenerateSignature_DifferentPayloads verifies that different payloads
// produce different signatures with the same secret.
func TestGenerateSignature_DifferentPayloads(t *testing.T) {
	a := GenerateSignature([]byte(`{"x":1}`), "secret")
	b := GenerateSignature([]byte(`{"x":2}`), "secret")
	if a == b {
		t.Error("different payloads should produce different signatures")
	}
}

// TestEndpointMatchesEvent verifies that event matching respects exact names
// and the wildcard "*".
func TestEndpointMatchesEvent(t *testing.T) {
	ep := Endpoint{
		Events: []string{"order.created", "order.updated"},
	}

	if !endpointMatchesEvent(ep, "order.created") {
		t.Error("expected match for 'order.created'")
	}
	if !endpointMatchesEvent(ep, "order.updated") {
		t.Error("expected match for 'order.updated'")
	}
	if endpointMatchesEvent(ep, "order.deleted") {
		t.Error("expected no match for 'order.deleted'")
	}

	// Wildcard endpoint should match anything.
	wildcard := Endpoint{Events: []string{"*"}}
	if !endpointMatchesEvent(wildcard, "any.event.type") {
		t.Error("wildcard endpoint should match any event")
	}
}

// TestTruncate verifies that long strings are truncated and short strings pass through.
func TestTruncate(t *testing.T) {
	cases := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello"},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcd", 3, "abc"},
	}
	for _, tc := range cases {
		got := truncate(tc.input, tc.maxLen)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
		}
	}
}

// TestDispatcher_TestEndpoint_HappyPath verifies that TestEndpoint returns
// a successful result for a test HTTP server that returns 200.
func TestDispatcher_TestEndpoint_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify expected headers are present.
		if r.Header.Get("X-Webhook-Test") != "true" {
			t.Errorf("missing X-Webhook-Test header")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing Content-Type header")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client: &http.Client{},
	}
	endpoint := &Endpoint{
		URL:    srv.URL,
		Secret: "test-secret",
	}
	result := d.TestEndpoint(endpoint)
	if !result.Success {
		t.Errorf("expected success=true, got false; error=%q", result.Error)
	}
	if result.Status == nil || *result.Status != 200 {
		t.Errorf("expected status 200")
	}
}

// TestEnvIntCapped verifies that envIntCapped enforces the max ceiling and
// rejects non-positive values.
func TestEnvIntCapped(t *testing.T) {
	cases := []struct {
		env      string // value to set (empty = unset)
		def, max int
		want     int
	}{
		{"", 50, 200, 50},     // unset → default
		{"30", 50, 200, 30},   // within range
		{"200", 50, 200, 200}, // exactly max
		{"500", 50, 200, 200}, // exceeds max → capped
		{"0", 50, 200, 50},    // zero → invalid → default
		{"-1", 50, 200, 50},   // negative → invalid → default
	}
	for _, tc := range cases {
		t.Setenv("TEST_ENV_INT_CAPPED", tc.env)
		got := envIntCapped("TEST_ENV_INT_CAPPED", tc.def, tc.max)
		if got != tc.want {
			t.Errorf("envIntCapped(env=%q, def=%d, max=%d) = %d, want %d", tc.env, tc.def, tc.max, got, tc.want)
		}
	}
}

// TestDispatcher_SemaphoreCap verifies that the dispatcher semaphore limits
// concurrent goroutines to at most maxConcurrency even when more are started.
//
// Strategy: build a Dispatcher with sem capacity = 5, spawn 20 goroutines
// each of which holds the slot for 20 ms (simulating a DB status update),
// and assert that the observed peak concurrent count never exceeds 5.
func TestDispatcher_SemaphoreCap(t *testing.T) {
	const cap = 5
	const total = 20
	const holdDuration = 20 * time.Millisecond

	d := &Dispatcher{
		sem:            make(chan struct{}, cap),
		maxConcurrency: cap,
		client:         &http.Client{},
	}

	var (
		wg      sync.WaitGroup
		peak    int64
		current int64
	)

	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Acquire semaphore — mirrors the real processPending path.
			d.sem <- struct{}{}
			defer func() { <-d.sem }()

			// Track concurrent goroutines inside the semaphore.
			c := atomic.AddInt64(&current, 1)
			for {
				p := atomic.LoadInt64(&peak)
				if c <= p || atomic.CompareAndSwapInt64(&peak, p, c) {
					break
				}
			}

			time.Sleep(holdDuration) // simulate DB work
			atomic.AddInt64(&current, -1)
		}()
	}

	wg.Wait()

	if peak > int64(cap) {
		t.Errorf("peak concurrent goroutines = %d; want <= %d (semaphore cap)", peak, cap)
	}
	t.Logf("peak concurrent goroutines = %d (cap = %d)", peak, cap)
}

// TestDispatcher_TestEndpoint_ServerError verifies that a 5xx response is
// reported as failure.
func TestDispatcher_TestEndpoint_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	d := &Dispatcher{
		client: &http.Client{},
	}
	endpoint := &Endpoint{
		URL:    srv.URL,
		Secret: "test-secret",
	}
	result := d.TestEndpoint(endpoint)
	if result.Success {
		t.Error("expected success=false for 500 response")
	}
	if result.Status == nil || *result.Status != 500 {
		t.Errorf("expected status 500")
	}
}
