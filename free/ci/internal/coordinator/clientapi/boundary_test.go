package clientapi

// Purpose: tests that pin the exact edges of the user API: body size, list limits, run-by-commit lookup, the
// store-outage answer, the evidence in the final event record, who may read the logs of an unowned run, the
// limiter tables and the logs lookup of the store backend. Each one exists because a mutation of that edge
// (gremlins) survived the first test set.
// Inputs:  the rigs of rig_test.go. Outputs: pass or fail.

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestSubmitBodyLimitIsExact(t *testing.T) {
	r := newRig(t)
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	pad := func(n int) string { // a valid submit body of exactly n bytes
		core := submitBody("acme/app")
		return core + strings.Repeat(" ", n-len(core))
	}
	if st, b := r.do(r.client(), "POST", PathRuns, tok, pad(maxBody)); st != 202 {
		t.Fatalf("a body of exactly %d bytes must be accepted: %d %s", maxBody, st, b)
	}
	if st, b := r.do(r.client(), "POST", PathRuns, tok, pad(maxBody+1)); st != 400 {
		t.Fatalf("a body one byte over must be refused: %d %s", st, b)
	}
}

func TestListLimitEdges(t *testing.T) {
	r := newRig(t)
	for i := 0; i < 230; i++ {
		r.be.runs = append(r.be.runs, item(fmt.Sprintf("run-%03d", i), "acme/app", "passed"))
	}
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	count := func(q string) int {
		_, b := r.do(r.client(), "GET", PathRuns+q, tok, "")
		return strings.Count(string(b), `"id":"run-`)
	}
	for q, want := range map[string]int{"": 20, "?limit=0": 20, "?limit=-3": 20, "?limit=3": 3, "?limit=200": 200, "?limit=201": 20, "?limit=x": 20} {
		if got := count(q); got != want {
			t.Errorf("limit %q returned %d runs, want %d", q, got, want)
		}
	}
}

func TestRunByCommitFindsThatCommitsRunNotTheNewest(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{
		itemAt("run-new", "acme/app", "passed", strings.Repeat("b", 40)),
		itemAt("run-hidden", "acme/other", "passed", strings.Repeat("c", 40)),
		itemAt("run-old", "acme/app", "passed", strings.Repeat("a", 40)),
	}
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	st, b := r.do(r.client(), "GET", PathRunsPfx+strings.Repeat("a", 40), tok, "")
	if st != 200 || !strings.Contains(string(b), `"id":"run-old"`) {
		t.Fatalf("by commit: %d %s", st, b)
	}
	st, b = r.do(r.client(), "GET", PathRunsPfx+strings.Repeat("c", 40), tok, "")
	refused(t, st, b, 404, ReasonNotFound)
	st, b = r.do(r.client(), "GET", PathRunsPfx+"latest", tok, "")
	if st != 200 || !strings.Contains(string(b), `"id":"run-new"`) {
		t.Fatalf("latest: %d %s", st, b)
	}
}

// A store that cannot be read is an internal failure, said so and logged, never a role refusal or a missing run.
func TestStoreOutageIsAnInternalErrorNotARefusal(t *testing.T) {
	r := newRig(t)
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	var logged []string
	api := New(Config{Backend: r.be, Roles: r.svc, Logf: func(f string, a ...any) { logged = append(logged, fmt.Sprintf(f, a...)) }})
	if err := r.st.Close(); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", PathRuns, nil)
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+tok)
	api.Handler().ServeHTTP(w, req)
	refused(t, w.Code, w.Body.Bytes(), http.StatusInternalServerError, ReasonInternal)
	if len(logged) == 0 || strings.Contains(w.Body.String(), "closed") || strings.Contains(w.Body.String(), "sql") {
		t.Fatalf("the cause must go to the log, not the client: logged %v body %s", logged, w.Body.String())
	}
}

func TestFinalEventRecordCarriesTheEvidence(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-done", "acme/app", "passed")}
	r.be.evidence = []byte(`{"schema":"ci.evidence/v1","marker":"signed-doc"}`)
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	st, b := r.do(r.client(), "GET", PathRunsPfx+"run-done/events", tok, "")
	ls := lines(t, b)
	if st != 200 || len(ls) == 0 || !strings.Contains(ls[len(ls)-1], `"marker":"signed-doc"`) {
		t.Fatalf("the result record must hold the evidence: %d %s", st, b)
	}
}

func TestAdminReadsTheLogOfAnUnownedRun(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-local", "", "passed")}
	for i := int64(1); i <= 2; i++ {
		r.be.lines = append(r.be.lines, logs.Line{TS: "t", Stream: "out", Seq: i, Text: "x"})
	}
	admin := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	st, b := r.do(r.client(), "GET", PathLogsPfx+"build?run=run-local", admin, "")
	if st != 200 || len(lines(t, b)) != 2 {
		t.Fatalf("an all-project admin may read an unowned run's log: %d %s", st, b)
	}
	member := r.token("mia", roles.Member, time.Hour, "acme/app")
	st, b = r.do(r.client(), "GET", PathLogsPfx+"build?run=run-local", member, "")
	refused(t, st, b, 404, ReasonNotFound)
}

func TestFollowedLogEndsWhenTheRunIsGone(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-live", "acme/app", "running")}
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	req, _ := http.NewRequest("GET", r.srv.URL+PathLogsPfx+"build?run=run-live&follow=1", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := r.client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	buf := make([]byte, 512)
	if n, _ := resp.Body.Read(buf); n == 0 {
		t.Fatal("no first line")
	}
	r.be.mu.Lock()
	r.be.getFail = true
	r.be.mu.Unlock()
	done := make(chan struct{})
	go func() {
		for {
			if _, err := resp.Body.Read(buf); err != nil {
				close(done)
				return
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("a followed log outlived the run it follows")
	}
}

func TestLimiterTablesKeepSmallTablesAndBoundBigOnes(t *testing.T) {
	l := newLimiter()
	now := time.Unix(1_800_000_000, 0)
	l.now = func() time.Time { return now }
	for i := 0; i < 10; i++ {
		l.failed(fmt.Sprintf("10.0.0.%d", i))
		l.submitAllowed(fmt.Sprintf("p%d", i))
	}
	if len(l.fails) != 10 || len(l.submits) != 10 {
		t.Fatalf("a small table must not evict: %d fails, %d submits", len(l.fails), len(l.submits))
	}
	for i := 0; i < limiterEntries; i++ { // fill exactly to the cap, then one more source must still fit
		l.failed(fmt.Sprintf("h%d", i))
		l.submitAllowed(fmt.Sprintf("q%d", i))
	}
	if len(l.fails) > limiterEntries || len(l.submits) > limiterEntries {
		t.Fatalf("tables grew past the cap: %d, %d", len(l.fails), len(l.submits))
	}
	// exactly at the window edge the count still holds; one tick later it is gone
	l2 := newLimiter()
	l2.now = func() time.Time { return now }
	for i := 0; i < authFailMax; i++ {
		l2.failed("a")
	}
	now = now.Add(authFailWindow)
	if l2.allowed("a") {
		t.Fatal("at exactly the window the lock must still hold")
	}
	l2.failed("a") // counts into the same window
	if l2.allowed("a") {
		t.Fatal("a failure at the window edge must still count")
	}
	for i := 0; i < submitsPerMinute; i++ {
		l2.submitAllowed("s")
	}
	now = now.Add(time.Minute)
	if l2.submitAllowed("s") {
		t.Fatal("at exactly one minute the submit window must still hold")
	}
	// stream slots: the entry goes away when the last slot is released and stays while one is held
	a, _ := l2.openStream("x")
	b, _ := l2.openStream("x")
	a()
	if l2.streams["x"] != 1 {
		t.Fatalf("one slot still held, count %d", l2.streams["x"])
	}
	b()
	if _, held := l2.streams["x"]; held {
		t.Fatal("the entry of a principal with no stream must be removed")
	}
}

func TestStoreBackendLogsLookup(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	home := t.TempDir()
	b := &StoreBackend{Store: st, Home: home}
	p := store.PipelineRow{ID: "run-l", Project: "/tmp/c", Revision: goodRev, Trigger: "api", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}
	if err = st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-l", PipelineID: "run-l", Name: "build", Kind: "test", InfraMax: 2, InputDigest: "i", Idempotent: true}); err != nil {
		t.Fatal(err)
	}
	write := func(attempt string, text string) {
		w, e := logs.Open(logs.AttemptDir(home, "run-l", attempt), logs.Options{})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = w.Append(logs.Line{TS: "t", Stream: "out", Seq: 1, Text: text}); e != nil {
			t.Fatal(e)
		}
		_ = w.Close()
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "att-1", JobID: "job-l", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	write("att-1", "attempt 1")
	read := func(q LogQuery) (bool, string) {
		var got []string
		ok, e := b.Logs(ctx, q, func(l logs.Line) error { got = append(got, l.Text); return nil })
		if e != nil {
			t.Fatal(e)
		}
		return ok, strings.Join(got, "|")
	}
	for name, c := range map[string]struct {
		q    LogQuery
		ok   bool
		want string
	}{
		"newest attempt by job name":     {LogQuery{Run: "run-l", Job: "build"}, true, "attempt 1"},
		"attempt 1 by job id":            {LogQuery{Run: "run-l", Job: "job-l", Attempt: 1}, true, "attempt 1"},
		"after the only line":            {LogQuery{Run: "run-l", Job: "build", After: 1}, true, ""},
		"an attempt that does not exist": {LogQuery{Run: "run-l", Job: "build", Attempt: 2}, false, ""},
		"an unknown job":                 {LogQuery{Run: "run-l", Job: "nope"}, false, ""},
		"an unknown run":                 {LogQuery{Run: "run-zzz", Job: "build"}, false, ""},
	} {
		if ok, got := read(c.q); ok != c.ok || got != c.want {
			t.Errorf("%s: ok=%v %q, want ok=%v %q", name, ok, got, c.ok, c.want)
		}
	}
	// a job whose attempt has stored nothing has no log (and a follow of an ended attempt ends)
	if err = st.CreateJob(ctx, store.JobRow{ID: "job-m", PipelineID: "run-l", Name: "lint", Kind: "test", InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: "att-m", JobID: "job-m", N: 1}, ""); err != nil {
		t.Fatal(err)
	}
	if ok, _ := read(LogQuery{Run: "run-l", Job: "lint"}); ok {
		t.Fatal("an attempt with no stored log has none to show")
	}
}
