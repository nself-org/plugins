package clientapi

// Purpose: tests of the user API: who is refused and with which E664 reason, role checks on built-in and
// registered routes, project scope, the audit rows, the event stream and the evidence bytes.
// Inputs:  the rig of rig_test.go. Outputs: pass or fail.
// Constraints: real SQLite, real pki, real TLS on 127.0.0.1; no sleeps beyond the 10 ms poll.

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const goodRev = "0123456789abcdef0123456789abcdef01234567"

func submitBody(project string) string {
	return `{"project":"` + project + `","revision":"` + goodRev + `"}`
}

func TestUserAPIRefusals(t *testing.T) {
	r := newRig(t)
	anon := r.client()
	t.Run("no credential", func(t *testing.T) {
		st, b := r.do(anon, "GET", PathRuns, "", "")
		refused(t, st, b, 401, ReasonNoCredential)
	})
	t.Run("node certificate", func(t *testing.T) {
		st, b := r.do(r.client(r.nodeCert("node-1")), "GET", PathRuns, "", "")
		refused(t, st, b, 403, ReasonNodeKind)
	})
	t.Run("node certificate with a good token", func(t *testing.T) {
		tok := r.token("bob", roles.Member, time.Hour, "acme/app")
		st, b := r.do(r.client(r.nodeCert("node-2")), "GET", PathRuns, tok, "")
		refused(t, st, b, 403, ReasonNodeKind) // the certificate wins; a token never rescues a node
	})
	t.Run("unknown token", func(t *testing.T) {
		st, b := r.do(anon, "GET", PathRuns, "not-a-token", "")
		refused(t, st, b, 401, ReasonTokenUnknown)
	})
	t.Run("user certificate without a grant", func(t *testing.T) {
		st, b := r.do(r.client(r.userCert("nobody")), "GET", PathRuns, "", "")
		refused(t, st, b, 403, ReasonNoGrant)
	})
	t.Run("expired token", func(t *testing.T) {
		tok := r.token("eve", roles.Member, time.Minute, "acme/app")
		r.shift = 2 * time.Minute
		defer func() { r.shift = 0 }()
		st, b := r.do(anon, "GET", PathRuns, tok, "")
		refused(t, st, b, 401, ReasonTokenExpired)
	})
	t.Run("revoked token", func(t *testing.T) {
		tok, tk, err := r.svc.IssueToken(context.Background(), r.op(), roles.Request{Principal: "rev", Role: roles.Member, Projects: []string{"acme/app"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = r.svc.RevokeToken(context.Background(), r.op(), tk.ID); err != nil {
			t.Fatal(err)
		}
		st, b := r.do(anon, "GET", PathRuns, tok, "")
		refused(t, st, b, 401, ReasonTokenRevoked)
	})
	t.Run("token that names no project", func(t *testing.T) {
		const clear = "scopeless-token-text"
		in, err := (&audit.Log{St: r.st}).Input(audit.Event{Actor: "operator:cli:alice", Action: "token.issue", Target: "zed", Detail: "test"})
		if err != nil {
			t.Fatal(err)
		}
		exp, sum := time.Now().Add(time.Hour).UnixNano(), sha256.Sum256([]byte(clear))
		tok := store.Token{ID: "tok-scopeless", Digest: hex.EncodeToString(sum[:]), Principal: "zed", Role: "member", Subject: "zed", IssuedBy: "operator:cli:alice", ExpiresAt: exp}
		if err = r.st.TokenPut(context.Background(), tok, in); err != nil {
			t.Fatal(err)
		}
		st, b := r.do(anon, "GET", PathRuns, clear, "")
		refused(t, st, b, 403, ReasonUnscopedToken)
	})
	t.Run("plain HTTP", func(t *testing.T) {
		api := New(Config{Backend: r.be, Roles: r.svc})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", PathRuns, nil)
		req.Header.Set("Authorization", "Bearer something")
		api.Handler().ServeHTTP(w, req)
		refused(t, w.Code, w.Body.Bytes(), 401, ReasonPlainHTTP)
	})
	t.Run("no body or header leaks the token", func(t *testing.T) {
		st, b := r.do(anon, "GET", PathRuns, "secret-token-text", "")
		if st != 401 || strings.Contains(string(b), "secret-token-text") {
			t.Fatalf("%d %s", st, b)
		}
	})
}

func TestRegisteredRouteRoles(t *testing.T) {
	serve := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		a, ok := ActorFrom(req.Context())
		if !ok {
			t.Error("no actor in the handler context")
		}
		_, _ = w.Write([]byte(a.ID()))
	})
	if err := RegisterRoute("GET", "/v1/test-member", "member", serve); err != nil {
		t.Fatal(err)
	}
	if err := RegisterRoute("GET", "/v1/test-release", "release", serve); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"/v1/{x", "/v1/a/{x}/{x}", "/v1/{$}x", "relative"} {
		if RegisterRoute("GET", bad, "member", serve) == nil {
			t.Errorf("pattern %q would panic the mux at serve time and must be refused at registration", bad)
		}
	}
	if RegisterRoute("GET", "/v1/test-member", "member", serve) == nil || RegisterRoute("GET", "/v1/x", "boss", serve) == nil || RegisterRoute("GET", "/v1/y", "member", nil) == nil {
		t.Fatal("a duplicate, an unknown role and a nil handler must be refused")
	}
	r := newRig(t)
	member := r.token("mia", roles.Member, time.Hour, "acme/app")
	release := r.token("ray", roles.Release, time.Hour, roles.AllProjects)
	anon := r.client()
	st, b := r.do(anon, "GET", "/v1/test-member", "", "")
	refused(t, st, b, 401, ReasonNoCredential)
	if st, b = r.do(anon, "GET", "/v1/test-member", member, ""); st != 200 || string(b) != "mia" {
		t.Fatalf("member on a member route: %d %s", st, b)
	}
	st, b = r.do(anon, "GET", "/v1/test-release", "", "")
	refused(t, st, b, 401, ReasonNoCredential)
	st, b = r.do(anon, "GET", "/v1/test-release", member, "")
	refused(t, st, b, 403, ReasonRole)
	if st, b = r.do(anon, "GET", "/v1/test-release", release, ""); st != 200 || string(b) != "ray" {
		t.Fatalf("release on a release route: %d %s", st, b)
	}
}

func TestSubmit(t *testing.T) {
	r := newRig(t)
	anon := r.client()
	app := r.token("mia", roles.Member, time.Hour, "acme/app")
	if st, b := r.do(anon, "POST", PathRuns, app, submitBody("acme/app")); st != 202 || !strings.Contains(string(b), `"id":"in-0001"`) {
		t.Fatalf("submit: %d %s", st, b)
	}
	if len(r.be.submits) != 1 || r.be.submits[0].Project != "acme/app" || r.be.submits[0].Revision != goodRev {
		t.Fatalf("backend saw %+v", r.be.submits)
	}
	st, b := r.do(anon, "POST", PathRuns, app, submitBody("acme/other"))
	refused(t, st, b, 403, ReasonProject)
	r.allow["acme/app"] = false
	st, b = r.do(anon, "POST", PathRuns, app, submitBody("acme/app"))
	refused(t, st, b, 403, ReasonProjectBlocked)
	r.allow["acme/app"] = true
	for name, body := range map[string]string{
		"short revision": `{"project":"acme/app","revision":"abc"}`, "unknown field": `{"project":"acme/app","revision":"` + goodRev + `","role":"admin"}`,
		"odd pipeline": `{"project":"acme/app","revision":"` + goodRev + `","pipeline":"a b/../c"}`, "not json": `nope`,
	} {
		if st, b = r.do(anon, "POST", PathRuns, app, body); st != 400 {
			t.Fatalf("%s: %d %s", name, st, b)
		}
	}
	if n := len(r.be.submits); n != 1 {
		t.Fatalf("refused submits reached the backend: %d", n)
	}
	var rows []string
	entries, _ := r.st.AuditEntries(context.Background(), 0, 0)
	for _, e := range entries {
		rows = append(rows, e.Action)
	}
	if !strings.Contains(strings.Join(rows, ","), "run.submit") {
		t.Fatalf("no run.submit audit row: %v", rows)
	}
}

func TestListGetCancelScope(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-b", "acme/other", "running"), item("run-a", "acme/app", "passed"), item("run-loose", "", "passed")}
	anon := r.client()
	app := r.token("mia", roles.Member, time.Hour, "acme/app")
	admin := r.token("ada", roles.Admin, time.Hour, roles.AllProjects)
	var list struct{ Runs []RunItem }
	st, b := r.do(anon, "GET", PathRuns, app, "")
	if err := json.Unmarshal(b, &list); err != nil || st != 200 || len(list.Runs) != 1 || list.Runs[0].ID != "run-a" {
		t.Fatalf("member list: %d %s", st, b)
	}
	if st, b = r.do(anon, "GET", PathRuns, admin, ""); st != 200 || !strings.Contains(string(b), "run-loose") || !strings.Contains(string(b), "run-b") {
		t.Fatalf("admin list: %d %s", st, b)
	}
	for _, id := range []string{"run-b", "run-loose", "run-none"} {
		st, b = r.do(anon, "GET", PathRunsPfx+id, app, "")
		refused(t, st, b, 404, ReasonNotFound) // other project, unowned run and missing run look alike
	}
	if st, b = r.do(anon, "GET", PathRunsPfx+"latest", app, ""); st != 200 || !strings.Contains(string(b), `"id":"run-a"`) {
		t.Fatalf("latest: %d %s", st, b)
	}
	st, b = r.do(anon, "POST", PathRunsPfx+"run-b/cancel", app, "")
	refused(t, st, b, 404, ReasonNotFound)
	if len(r.be.cancels) != 0 {
		t.Fatal("a cancel of an invisible run reached the backend")
	}
	if st, b = r.do(anon, "POST", PathRunsPfx+"run-a/cancel", app, ""); st != 200 || len(r.be.cancels) != 1 {
		t.Fatalf("cancel: %d %s %v", st, b, r.be.cancels)
	}
}

func TestEventsStreamResumesBySeq(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-e", "acme/app", "running")}
	r.be.evidence = json.RawMessage(`{"schema":"ci.evidence/v1","result":{"state":"pass"}}`)
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	go func() {
		time.Sleep(80 * time.Millisecond)
		r.be.set("run-e", "passed")
	}()
	st, b := r.do(r.client(), "GET", PathRunsPfx+"run-e/events", tok, "")
	if st != 200 {
		t.Fatalf("%d %s", st, b)
	}
	var seqs []int64
	var types []string
	for _, l := range lines(t, b) {
		rec, ok := detach.ParseLine([]byte(l))
		if !ok {
			t.Fatalf("not a ci.events record: %q", l)
		}
		seqs, types = append(seqs, rec.Seq), append(types, rec.Type)
	}
	if len(seqs) < 4 || types[0] != "queued" || types[len(types)-1] != "result" {
		t.Fatalf("records %v %v", types, seqs)
	}
	for i := range seqs {
		if seqs[i] != int64(i+1) {
			t.Fatalf("seq gap or duplicate: %v", seqs)
		}
	}
	// a reconnect from seq 3 gets exactly the records from 3 on
	st, b = r.do(r.client(), "GET", PathRunsPfx+"run-e/events?from=3", tok, "")
	rest := lines(t, b)
	if st != 200 || len(rest) != len(seqs)-2 {
		t.Fatalf("resume: %d, %d lines, want %d", st, len(rest), len(seqs)-2)
	}
	if rec, _ := detach.ParseLine([]byte(rest[0])); rec.Seq != 3 {
		t.Fatalf("resumed at seq %d, want 3", rec.Seq)
	}
}

func TestEvidenceAndLogs(t *testing.T) {
	r := newRig(t)
	r.be.runs = []RunItem{item("run-a", "acme/app", "passed")}
	raw := `{"schema":"ci.evidence/v1",  "signature":{"kind":"local","value":"sig=="}}`
	r.be.evidence = json.RawMessage(raw)
	r.be.lines = nil
	for i := int64(1); i <= 3; i++ {
		r.be.lines = append(r.be.lines, logs.Line{TS: "t", Stream: "out", Seq: i, Text: "line"})
	}
	tok := r.token("mia", roles.Member, time.Hour, "acme/app")
	st, b := r.do(r.client(), "GET", PathEvidence+"run-a", tok, "")
	if st != 200 || string(b) != raw {
		t.Fatalf("evidence changed in transit: %d %q", st, b)
	}
	st, b = r.do(r.client(), "GET", PathLogsPfx+"build?run=run-a&after=1", tok, "")
	if got := lines(t, b); st != 200 || len(got) != 2 || !strings.Contains(got[0], `"seq":2`) {
		t.Fatalf("logs: %d %q", st, b)
	}
	st, b = r.do(r.client(), "GET", PathLogsPfx+"nojob?run=run-a", tok, "")
	refused(t, st, b, 404, ReasonNotFound)
}

// A submit runs the commit's own pipeline config on the coordinator, so by default it needs the release
// role; a member-submit coordinator still reserves naming a pipeline for release.
func TestSubmitRoleAndNamedPipeline(t *testing.T) {
	r := newRig(t)
	anon := r.client()
	member := r.token("mia", roles.Member, time.Hour, "acme/app")
	release := r.token("ray", roles.Release, time.Hour, "acme/app")
	strict := New(Config{Backend: r.be, Roles: r.svc, Allow: func(string) bool { return true }}) // default SubmitRole
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", PathRuns, strings.NewReader(submitBody("acme/app")))
	req.TLS = &tls.ConnectionState{}
	req.Header.Set("Authorization", "Bearer "+member)
	strict.Handler().ServeHTTP(w, req)
	refused(t, w.Code, w.Body.Bytes(), 403, ReasonRole)
	if len(r.be.submits) != 0 {
		t.Fatal("a refused submit reached the backend")
	}
	named := `{"project":"acme/app","revision":"` + goodRev + `","pipeline":"release"}`
	st, b := r.do(anon, "POST", PathRuns, member, named) // this rig lets members submit
	refused(t, st, b, 403, ReasonRole)
	if st, b = r.do(anon, "POST", PathRuns, release, named); st != 202 {
		t.Fatalf("a release holder may name a pipeline: %d %s", st, b)
	}
	if len(r.be.submits) != 1 || r.be.submits[0].Pipeline != "release" {
		t.Fatalf("backend saw %+v", r.be.submits)
	}
}
