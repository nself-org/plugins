package clientapi

// Purpose: tests of the abuse limits: failed authentications are capped per source before any store
// lookup, role refusals do not count, and open streams are capped per principal and given back.
// Inputs:  the rig of rig_test.go and a limiter with an injected clock. Outputs: pass or fail.

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/roles"
)

func TestFailedAuthenticationsAreCappedPerSource(t *testing.T) {
	r := newRig(t)
	anon := r.client()
	for i := 0; i < authFailMax; i++ {
		st, b := r.do(anon, "GET", PathRuns, "guess", "")
		refused(t, st, b, 401, ReasonTokenUnknown)
	}
	st, b := r.do(anon, "GET", PathRuns, "guess", "")
	refused(t, st, b, 429, ReasonRateLimited)
	good := r.token("mia", roles.Member, time.Hour, "acme/app")
	st, b = r.do(anon, "GET", PathRuns, good, "")
	refused(t, st, b, 429, ReasonRateLimited) // the source stays locked for the window, even with a real token
}

func TestRoleRefusalsDoNotCountAsFailedAuthentication(t *testing.T) {
	r := newRig(t)
	member := r.token("mia", roles.Member, time.Hour, "acme/app")
	for i := 0; i < authFailMax+5; i++ {
		st, b := r.do(r.client(), "POST", PathRuns, member, submitBody("acme/other"))
		refused(t, st, b, 403, ReasonProject)
	}
}

func TestLimiterWindowAndBound(t *testing.T) {
	l := newLimiter()
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < authFailMax; i++ {
		l.failed("10.0.0.1")
	}
	if l.allowed("10.0.0.1") || !l.allowed("10.0.0.2") {
		t.Fatal("only the failing source is locked")
	}
	now = now.Add(authFailWindow + time.Second)
	if !l.allowed("10.0.0.1") {
		t.Fatal("the lock must end with the window")
	}
	for i := 0; i < limiterEntries+50; i++ {
		l.failed(string(rune('a'+i%26)) + time.Duration(i).String())
	}
	if len(l.fails) > limiterEntries {
		t.Fatalf("the table holds %d sources, cap %d", len(l.fails), limiterEntries)
	}
}

func TestStreamSlotsPerPrincipal(t *testing.T) {
	l := newLimiter()
	var rel []func()
	for i := 0; i < streamsPerPrincipal; i++ {
		f, ok := l.openStream("mia")
		if !ok {
			t.Fatalf("slot %d refused", i)
		}
		rel = append(rel, f)
	}
	if _, ok := l.openStream("mia"); ok {
		t.Fatal("slot beyond the cap granted")
	}
	if _, ok := l.openStream("bob"); !ok {
		t.Fatal("another principal must have its own slots")
	}
	rel[0]()
	rel[0]() // a second release is harmless
	if _, ok := l.openStream("mia"); !ok {
		t.Fatal("a released slot must be reusable")
	}
}

func TestOpenEventStreamsAreCapped(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-live", "acme/app", "running")}
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := r.client()
	open := func() *http.Response {
		req, _ := http.NewRequestWithContext(ctx, "GET", r.srv.URL+PathRunsPfx+"run-live/events", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}
	for i := 0; i < streamsPerPrincipal; i++ {
		resp := open()
		defer func() { _ = resp.Body.Close() }()
	}
	st, b := r.do(c, "GET", PathRunsPfx+"run-live/events", tok, "")
	refused(t, st, b, 429, ReasonRateLimited)
}

// A followed log must end when the credential behind it is revoked, not run to the stream's end.
func TestFollowedLogEndsWhenTheTokenIsRevoked(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-live", "acme/app", "running")}
	tok, tk, err := r.svc.IssueToken(context.Background(), r.op(), roles.Request{Principal: "mia", Role: roles.Member, Projects: []string{"acme/app"}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("GET", r.srv.URL+PathLogsPfx+"build?run=run-live&follow=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := r.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 4096)
	if n, _ := resp.Body.Read(buf); n == 0 {
		t.Fatal("no first log line")
	}
	if _, err = r.svc.RevokeToken(context.Background(), r.op(), tk.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, resp.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("the followed log outlived the revoked token")
	}
}
