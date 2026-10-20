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

func TestRefusedReadsDoNotCountAsFailedAuthentication(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-other", "acme/other", "passed")}
	member := r.token("mia", roles.Member, time.Hour, "acme/app")
	for i := 0; i < authFailMax+5; i++ {
		st, b := r.do(r.client(), "GET", PathRunsPfx+"run-other", member, "")
		refused(t, st, b, 404, ReasonNotFound)
	}
}

func TestSubmitsAreCappedPerPrincipalWhetherAllowedOrNot(t *testing.T) {
	l := newLimiter()
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < submitsPerMinute; i++ {
		if !l.submitAllowed("mia") {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	if l.submitAllowed("mia") || !l.submitAllowed("bob") {
		t.Fatal("the cap is per principal and exact")
	}
	now = now.Add(time.Minute + time.Second)
	if !l.submitAllowed("mia") {
		t.Fatal("the window must reset")
	}
}

func TestIPv6SourcesShareTheirSlash64(t *testing.T) {
	key := func(addr string) string {
		r, _ := http.NewRequest("GET", "http://x", nil)
		r.RemoteAddr = addr
		return sourceOf(r)
	}
	if key("[2001:db8:1:2::1]:4000") != key("[2001:db8:1:2:ffff::9]:5000") {
		t.Fatal("addresses of one /64 must share a count")
	}
	if key("[2001:db8:1:2::1]:4000") == key("[2001:db8:1:3::1]:4000") {
		t.Fatal("another /64 must have its own count")
	}
	if key("192.0.2.7:1") != "192.0.2.7" {
		t.Fatalf("ipv4 key %q", key("192.0.2.7:1"))
	}
}

// An event stream must end when the token behind it is revoked, and a long stream must not write an audit row
// per poll (an admin following a run no project owns would otherwise write two a second).
func TestEventsEndOnRevocationAndWriteNoAuditRowPerPoll(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-live", "acme/app", "running")}
	tok, tk, err := r.svc.IssueToken(context.Background(), r.op(), roles.Request{Principal: "mia", Role: roles.Member, Projects: []string{"acme/app"}})
	if err != nil {
		t.Fatal(err)
	}
	count := func() int {
		rows, _ := r.st.AuditEntries(context.Background(), 0, 0)
		return len(rows)
	}
	req, _ := http.NewRequest("GET", r.srv.URL+PathRunsPfx+"run-live/events", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := r.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 2048)
	if n, _ := resp.Body.Read(buf); n == 0 {
		t.Fatal("no first record")
	}
	before := count()
	time.Sleep(300 * time.Millisecond) // ~30 polls at 10 ms
	if got := count(); got != before {
		t.Fatalf("the stream wrote %d audit rows while polling", got-before)
	}
	if _, err = r.svc.RevokeToken(context.Background(), r.op(), tk.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() { _, _ = io.Copy(io.Discard, resp.Body); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the event stream outlived the revoked token")
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

// D-1408: every refused call to an admin route is an audited, chained deny row, so one member could grow
// the audit chain without bound. After deniesPerMinute refusals the caller gets 429 and no further row.
func TestRoleDenialsAreCappedPerActorAndStopWritingAuditRows(t *testing.T) {
	r := nodeRig(t)
	count := func() int {
		rows, _ := r.st.AuditEntries(context.Background(), 0, 0)
		return len(rows)
	}
	eve := r.token("eve", roles.Member, time.Hour, roles.AllProjects)
	before := count()
	for i := 0; i < deniesPerMinute; i++ {
		st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/drain", eve, ``)
		refused(t, st, b, 403, roles.ReasonRole)
	}
	rows := count() - before
	for i := 0; i < 20; i++ {
		st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/drain", eve, ``)
		refused(t, st, b, 429, ReasonRateLimited)
	}
	if got := count() - before; got != rows {
		t.Fatalf("%d audit rows after the cap, %d at the cap", got, rows)
	}
	bob := r.token("bob", roles.Member, time.Hour, roles.AllProjects)
	if st, b := r.do(r.client(), "POST", PathNodesPfx+"n1/drain", bob, ``); st != 403 {
		t.Fatalf("another actor must keep its own count: %d %s", st, b)
	}
}

func TestDenyCountResetsAfterTheWindow(t *testing.T) {
	l := newLimiter()
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < deniesPerMinute; i++ {
		if !l.denyAllowed("eve") {
			t.Fatalf("refusal %d blocked early", i+1)
		}
		l.denied("eve")
	}
	if l.denyAllowed("eve") || !l.denyAllowed("bob") {
		t.Fatal("the cap is per actor and exact")
	}
	now = now.Add(time.Minute + time.Second)
	if !l.denyAllowed("eve") {
		t.Fatal("the window must reset")
	}
}
