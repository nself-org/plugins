//go:build integration

package coordclient

// Purpose: TestCoordinatorClient (P7-NODE-19): one subtest per acceptance line of the Ticket, run against
// a simulated TEAM coordinator: the real user API over TLS 1.3 with the real PKI, role service and store.
// Inputs:  INTEGRATION=1 NSELF_V15=1. Outputs: pass or fail.
// Constraints: 127.0.0.1 and unix sockets only; the coordinator's run loop is played by the test (it writes
//          the rows a dispatch writes); nothing here skips.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

func needIntegration(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" || os.Getenv("NSELF_V15") != "1" {
		t.Fatal("run with INTEGRATION=1 NSELF_V15=1 -tags integration")
	}
}

// grantMember gives principal the member role on projects.
func (s *sim) grantMember(principal string, projects ...string) {
	s.t.Helper()
	if err := s.svc.GrantRole(context.Background(), s.op(), roles.Request{Principal: principal, Role: roles.Member, Projects: projects}); err != nil {
		s.t.Fatal(err)
	}
}

// waitQueued waits for the submitted run to appear in the intake and returns its id.
func (s *sim) waitQueued() string {
	s.t.Helper()
	for end := time.Now().Add(10 * time.Second); time.Now().Before(end); time.Sleep(10 * time.Millisecond) {
		if rows, _ := s.st.IntakeList(context.Background(), 5); len(rows) > 0 && rows[0].State == store.IntakeQueued {
			return rows[0].ID
		}
	}
	s.t.Fatal("no trigger reached the intake")
	return ""
}

// runOnce plays a whole run on the coordinator: start, one attempt with a stored log, finish.
func (s *sim) runOnce(pid string, live func()) (intakeID string) {
	s.t.Helper()
	intakeID = s.waitQueued()
	s.start(pid)
	ctx := context.Background()
	if err := s.st.CreateAttempt(ctx, store.AttemptRow{ID: "att-" + pid, JobID: "job-" + pid, N: 1}, ""); err != nil {
		s.t.Fatal(err)
	}
	w, err := logs.Open(logs.AttemptDir(s.dir, pid, "att-"+pid), logs.Options{})
	if err != nil {
		s.t.Fatal(err)
	}
	for i, l := range []string{"compiling", "tests ok"} {
		if _, err = w.Append(logs.Line{TS: "2026-10-19T00:00:00Z", Stream: "out", Seq: int64(i + 1), Text: l}); err != nil {
			s.t.Fatal(err)
		}
	}
	_ = w.Close()
	storetest.Lease(s.t, s.st, "att-"+pid)
	for i, to := range []model.JobState{"running", "finalizing", "passed"} {
		from := []model.JobState{"leased", "running", "finalizing"}[i]
		if err = s.st.Transition(ctx, "att-"+pid, from, to, store.TransitionDetail{Epoch: int64(i + 1)}); err != nil {
			s.t.Fatal(err)
		}
	}
	if live != nil {
		live()
	}
	s.finish(intakeID, pid, "passed", goodDoc(s.t))
	return intakeID
}

func TestCoordinatorClient(t *testing.T) {
	needIntegration(t)
	schemas, err := model.Schemas()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("nself ci --coordinator against a sim TEAM coordinator streams events that validate against contract:ci.events; runs, logs, cancel and evidence return the remote data and the evidence verifies", func(t *testing.T) {
		s := newSim(t)
		s.grantMember("mia", "acme/app")
		cl := s.client(s.userFiles("mia"))
		var out, errw bytes.Buffer
		done := make(chan error, 1)
		var res RunResult
		go func() {
			var e error
			res, e = cl.Run(context.Background(), RunInput{Project: "acme/app", Revision: testRev, JSON: true, Out: &out, Err: &errw})
			done <- e
		}()
		s.runOnce("run-1", func() { time.Sleep(60 * time.Millisecond) })
		if err := <-done; err != nil || res.Exit != 0 || res.Result != "passed" {
			t.Fatalf("run: %+v %v", res, err)
		}
		recs := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(recs) < 4 {
			t.Fatalf("too few records:\n%s", out.String())
		}
		for i, l := range recs {
			rec, ok := detach.ParseLine([]byte(l))
			if !ok || rec.Seq != int64(i+1) && !rec.Final() {
				t.Fatalf("record %d is not the next ci.events record: %q", i+1, l)
			}
			if rec.Final() {
				if i != len(recs)-1 {
					t.Fatalf("the result is not last")
				}
				continue
			}
			var env struct{ Data json.RawMessage }
			if json.Unmarshal([]byte(l), &env) != nil || model.ValidateJSON(schemas["ci-events.v1.schema.json"], env.Data) != nil {
				t.Fatalf("record %d does not validate against contract:ci.events: %s", i+1, l)
			}
		}
		ctx := context.Background()
		list, err := cl.List(ctx, 10)
		if err != nil || len(list) != 1 || list[0].ID != "run-1" || list[0].Status != "passed" || list[0].Project != "acme/app" {
			t.Fatalf("runs: %+v %v", list, err)
		}
		if it, ok, err := cl.Get(ctx, "run-1"); err != nil || !ok || len(it.Jobs) != 1 {
			t.Fatalf("show: %+v %v %v", it, ok, err)
		}
		var lines []string
		ok, err := cl.Logs(ctx, clientapi.LogQuery{Run: "run-1", Job: "build"}, func(l logs.Line) error { lines = append(lines, l.Text); return nil })
		if err != nil || !ok || strings.Join(lines, "|") != "compiling|tests ok" {
			t.Fatalf("logs: %v %v %v", lines, ok, err)
		}
		doc, err := cl.Evidence(ctx, "run-1")
		if err != nil || !bytes.Equal(doc, goodDoc(t)) {
			t.Fatalf("evidence changed in transit: %v", err)
		}
		ev, err := attest.Verify(doc)
		if err != nil || ev.Signature.Kind != "coordinator-recorded" {
			t.Fatalf("the coordinator's evidence must verify with its signature kind: %+v %v", ev.Signature, err)
		}
		// cancel reaches a live attempt of a run that is still going
		sub, err := cl.Submit(ctx, clientapi.SubmitRequest{Project: "acme/app", Revision: strings.Repeat("c", 40)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = cl.Cancel(ctx, sub.ID); !reasonIs(err, clientapi.ReasonConflict) {
			t.Fatalf("a run that has not started cannot be cancelled: %v", err)
		}
		s.start("run-2")
		if err = s.st.CreateAttempt(ctx, store.AttemptRow{ID: "att-run-2", JobID: "job-run-2", N: 1}, ""); err != nil {
			t.Fatal(err)
		}
		if n, err := cl.Cancel(ctx, "run-2"); err != nil || n != 1 {
			t.Fatalf("cancel asked %d attempts: %v", n, err)
		}
	})

	t.Run("with a local ci serve running, a second nself ci submits over serve.sock and never opens the DB for writing", func(t *testing.T) {
		s := newSim(t)
		home, err := os.MkdirTemp("", "ci-sock") // short path: sun_path is 104 bytes on macOS
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(home) })
		release, err := AcquireServeLock(home)
		if err != nil {
			t.Fatal(err)
		}
		defer release()
		ln, err := clientapi.ListenUnix(SocketPath(home))
		if err != nil {
			t.Fatal(err)
		}
		api := clientapi.New(clientapi.Config{Roles: roles.New(s.st, &audit.Log{St: s.st}), Local: true, Operator: "cli:alice", Poll: 10 * time.Millisecond,
			Backend: &clientapi.StoreBackend{Store: s.st, Home: s.dir, Enqueue: s.coord.Enqueue}})
		ctx, stop := context.WithCancel(context.Background())
		defer stop()
		go func() { _ = clientapi.Serve(ctx, ln, api.Handler()) }()
		info, err := os.Stat(SocketPath(home))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("the socket must be 0600: %v %v", info, err)
		}
		target, _, err := Resolve(Inputs{Getenv: env(), Home: home})
		if err != nil || target.Kind != LocalServe {
			t.Fatalf("a held lock must resolve to the local serve: %+v %v", target, err)
		}
		cl, err := New(target)
		if err != nil {
			t.Fatal(err)
		}
		var out, errw bytes.Buffer
		done := make(chan error, 1)
		var res RunResult
		go func() {
			var e error
			res, e = cl.Run(ctx, RunInput{Project: "any/project", Revision: testRev, Out: &out, Err: &errw})
			done <- e
		}()
		s.runOnce("run-local", nil)
		if err = <-done; err != nil || res.Exit != 0 || !strings.Contains(out.String(), "run run-local: passed") {
			t.Fatalf("local run: %+v %v\n%s", res, err, out.String())
		}
		entries, _ := os.ReadDir(home)
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		if strings.Join(names, ",") != "serve.lock,serve.sock" {
			t.Fatalf("the client left %v in the CI home; a state.db or journal there means it opened a database", names)
		}
	})

	t.Run("a node certificate, an unscoped token and an expired token are refused on the user API with E664 and reason keys", func(t *testing.T) {
		s := newSim(t)
		ctx, root := context.Background(), s.rootFile()
		if _, _, err := s.client(s.nodeFiles("node-3")).Get(ctx, "latest"); !reasonIs(err, clientapi.ReasonNodeKind) {
			t.Fatalf("node certificate: %v", err)
		}
		const clear = "token-without-projects"
		sum := sha256.Sum256([]byte(clear))
		in, _ := (&audit.Log{St: s.st}).Input(audit.Event{Actor: "operator:cli:alice", Action: "token.issue", Target: "zed", Detail: "test"})
		if err := s.st.TokenPut(ctx, store.Token{ID: "tok-none", Digest: hex.EncodeToString(sum[:]), Principal: "zed", Role: "member", Subject: "zed",
			IssuedBy: "operator:cli:alice", ExpiresAt: time.Now().Add(time.Hour).UnixNano()}, in); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(t.TempDir(), "tok")
		_ = os.WriteFile(p, []byte(clear), 0o600)
		if _, _, err := s.client(Creds{CA: root, TokenFile: p}).Get(ctx, "latest"); !reasonIs(err, clientapi.ReasonUnscopedToken) {
			t.Fatalf("unscoped token: %v", err)
		}
		tf := s.tokenFile("eve", roles.Member, time.Minute, "acme/app")
		s.svc.Now = func() time.Time { return time.Now().Add(2 * time.Minute) }
		_, _, err := s.client(Creds{CA: root, TokenFile: tf}).Get(ctx, "latest")
		s.svc.Now = time.Now
		if !reasonIs(err, clientapi.ReasonTokenExpired) {
			t.Fatalf("expired token: %v", err)
		}
	})

	t.Run("a ci.coordinator value in the committed .nself/ci.yaml is ignored with one warning line", func(t *testing.T) {
		repo, home := t.TempDir(), t.TempDir()
		_ = os.MkdirAll(filepath.Join(repo, ".nself"), 0o755)
		_ = os.WriteFile(filepath.Join(repo, ".nself", "ci.yaml"), []byte("version: 1\nci:\n  coordinator: https://attacker.example\n"), 0o644)
		var warn bytes.Buffer
		r := &Resolver{Warn: &warn, Inputs: func() Inputs { return Inputs{Getenv: env(), Home: home, RepoRoot: repo} }}
		for i := 0; i < 2; i++ {
			if tg, err := r.Target(); err != nil || tg.Kind != InProcess {
				t.Fatalf("%+v %v", tg, err)
			}
		}
		if lineCount(warn.String()) != 1 || !strings.HasPrefix(warn.String(), "warning: ignoring ci.coordinator") {
			t.Fatalf("warning output: %q", warn.String())
		}
	})

	t.Run("an events stream interrupted mid-run reconnects by seq with no duplicate and no missing event", func(t *testing.T) {
		s := newSim(t)
		s.grantMember("mia", "acme/app")
		var cuts atomic.Int32
		inner := s.srv.Config.Handler
		s.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/events") && cuts.Add(1) == 1 {
				w = &cutWriter{ResponseWriter: w, limit: 1} // the first connection dies after `queued`
			}
			inner.ServeHTTP(w, r)
		})
		cl := s.client(s.userFiles("mia"))
		var out, errw bytes.Buffer
		done := make(chan error, 1)
		go func() {
			_, e := cl.Run(context.Background(), RunInput{Project: "acme/app", Revision: testRev, JSON: true, Out: &out, Err: &errw})
			done <- e
		}()
		s.runOnce("run-cut", func() { time.Sleep(150 * time.Millisecond) })
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if cuts.Load() < 2 {
			t.Fatalf("the stream was asked for %d time(s); the cut never happened", cuts.Load())
		}
		var seqs []int64
		for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			rec, ok := detach.ParseLine([]byte(l))
			if !ok {
				t.Fatalf("not a record: %q", l)
			}
			seqs = append(seqs, rec.Seq)
		}
		for i := range seqs { // the result record carries the next seq too
			if seqs[i] != int64(i+1) {
				t.Fatalf("seqs %v: a duplicate or a gap at position %d", seqs, i)
			}
		}
	})

	t.Run("a client without ci.coordinator_ca, or facing a server leaf that is not coordinator kind, refuses with E664 server_identity", func(t *testing.T) {
		s := newSim(t)
		c := s.userFiles("mia")
		noCA := c
		noCA.CA = ""
		if _, err := New(s.target(noCA)); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("no pinned root: %v", err)
		}
		other := newSim(t)
		wrong := c
		wrong.CA = other.rootFile()
		cl, err := New(s.target(wrong))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = cl.Get(context.Background(), "latest"); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("foreign root: %v", err)
		}
		node := s.nodeFiles("node-9") // a node certificate served as the coordinator
		pair, err := tls.LoadX509KeyPair(node.Cert, node.Key)
		if err != nil {
			t.Fatal(err)
		}
		fake := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("{}")) }))
		fake.Listener = tls.NewListener(fake.Listener, &tls.Config{Certificates: []tls.Certificate{pair}, MinVersion: tls.VersionTLS13})
		fake.Start()
		defer fake.Close()
		cl, err = New(Target{Kind: Remote, URL: "https://" + fake.Listener.Addr().String(), Creds: c})
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = cl.Get(context.Background(), "latest"); !reasonIs(err, clientapi.ReasonServerIdentity) {
			t.Fatalf("a leaf that is not coordinator kind: %v", err)
		}
	})

	t.Run("a route registered with role member is refused for an unauthenticated client and a lower role, and served for a member", func(t *testing.T) {
		s := newSim(t)
		serve := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("ok")) })
		if err := clientapi.RegisterRoute("GET", "/v1/it-release-only", "release", serve); err != nil {
			t.Fatal(err)
		}
		if err := clientapi.RegisterRoute("GET", "/v1/it-member-only", "member", serve); err != nil {
			t.Fatal(err)
		}
		// the sim built its Handler before these routes existed: build a second server from the registry
		api := clientapi.New(clientapi.Config{Roles: s.svc, Backend: &clientapi.StoreBackend{Store: s.st, Home: s.dir, Enqueue: s.coord.Enqueue}})
		h := api.Handler()
		s.srv.Config.Handler = h
		root := s.rootFile()
		member := s.client(Creds{CA: root, TokenFile: s.tokenFile("mia", roles.Member, time.Hour, "acme/app")})
		get := func(cl *Client, path string) error {
			resp, err := cl.do(context.Background(), "GET", path, nil)
			if err == nil {
				_ = resp.Body.Close()
			}
			return err
		}
		if err := get(member, "/v1/it-member-only"); err != nil {
			t.Fatalf("a member must be served: %v", err)
		}
		if err := get(member, "/v1/it-release-only"); !reasonIs(err, clientapi.ReasonRole) {
			t.Fatalf("a member on a release route: %v", err)
		}
		anon := &Client{base: s.url, hc: member.hc}
		if err := get(anon, "/v1/it-member-only"); !reasonIs(err, clientapi.ReasonNoCredential) {
			t.Fatalf("an unauthenticated client: %v", err)
		}
	})
}

// cutWriter ends the connection after limit lines were written (a dropped network).
type cutWriter struct {
	http.ResponseWriter
	limit, lines int
}

func (c *cutWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *cutWriter) Write(p []byte) (int, error) {
	if c.lines >= c.limit {
		if conn, _, err := http.NewResponseController(c.ResponseWriter).Hijack(); err == nil {
			_ = conn.Close()
		}
		return 0, net.ErrClosed
	}
	c.lines += bytes.Count(p, []byte("\n"))
	return c.ResponseWriter.Write(p)
}

var _ = store.IntakeQueued
