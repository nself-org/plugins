//go:build cachesec

package cachesec

// Purpose: a simulated TEAM coordinator for group G4 (P7-CACHE-11), rebuilt from
// internal/release/remote/rig_integration_test.go: the real user API over TLS 1.3 with a real PKI, the real role
// service and scoped tokens, and the two release routes (remote.Handlers) registered through
// clientapi.RegisterRoute. The run store is an in-memory view of one finished release run (as in serve_test.go).
// Inputs:  newWorld (store, cas). Outputs: g4Team (client per role/project, run id, blob digest).
// Constraints: 127.0.0.1 only; keys and tokens are generated per test; the route registry is process-wide, so the
//          routes are registered once and the Deps they see are swapped per test (cases do not run in parallel).

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"encoding/pem"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	g4TeamRun     = "run-team-1"
	g4TeamProject = "acme/app"  // the owner's name for the project (the intake row)
	g4TeamRoot    = "/srv/proj" // the run's own project: its checkout path
	g4TeamRev     = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	g4TeamBytes   = "app-bytes-SYNTHETIC"
)

var (
	g4RoutesOnce sync.Once
	g4Current    atomic.Pointer[remote.Deps]
)

// g4TeamStore is one finished release run in memory.
type g4TeamStore struct{ recs []records.Record }

func (f *g4TeamStore) PipelineByID(_ context.Context, id string) (store.PipelineRow, bool, error) {
	return store.PipelineRow{ID: g4TeamRun, Project: g4TeamRoot, Revision: g4TeamRev, InputDigest: "in", Status: "passed"}, id == g4TeamRun, nil
}
func (f *g4TeamStore) PipelineJobs(context.Context, string) ([]store.RunJob, error) {
	return []store.RunJob{{ID: "r", Name: "publish", Kind: "release"}}, nil
}
func (f *g4TeamStore) PipelineAttempts(context.Context, string) ([]store.AttemptView, error) {
	return []store.AttemptView{{ID: "r.a1", JobID: "r", N: 1, State: "passed"}}, nil
}
func (f *g4TeamStore) EvidenceByRevision(context.Context, string, string) ([]store.EvidenceView, error) {
	return []store.EvidenceView{{ID: "e", PipelineID: g4TeamRun, Body: []byte(`{"run_id":"run-team-1"}`)}}, nil
}
func (f *g4TeamStore) ArtifactList(context.Context, records.Filter) ([]records.Record, error) {
	return f.recs, nil
}
func (f *g4TeamStore) AttemptCoordinator(context.Context, string) (string, error) { return "c1", nil }

type g4TeamRuns struct{}

func (g4TeamRuns) Get(_ context.Context, ref string) (clientapi.RunItem, bool, error) {
	it := clientapi.RunItem{Project: g4TeamProject}
	it.ID = g4TeamRun
	return it, ref == g4TeamRun, nil
}

// g4Team is the simulated TEAM coordinator.
type g4Team struct {
	w   *world
	svc *roles.Service
	ca  *pki.CA
	url string
	app string // digest of the release binary
}

func g4RegisterRoutes(t *testing.T) {
	g4RoutesOnce.Do(func() {
		h := remote.Handlers{Deps: func() (remote.Deps, bool) {
			d := g4Current.Load()
			if d == nil {
				return remote.Deps{}, false
			}
			return *d, true
		}}
		for pattern, handler := range map[string]http.Handler{remote.PatternInputs: h.InputsHandler(), remote.PatternBlob: h.BlobHandler()} {
			if err := clientapi.RegisterRoute(http.MethodGet, pattern, "member", handler); err != nil {
				t.Fatal(err)
			}
		}
	})
}

// newG4Team serves one finished release run over the TEAM user API.
func newG4Team(w *world) *g4Team {
	t := w.t
	g4RegisterRoutes(t)
	put := func(b []byte) string {
		d, _, err := w.Blobs.Put(w.ctx, cas.ArtifactRealm(g4TeamRoot), bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	rec := func(name string, kind records.Kind, b []byte) records.Record {
		return records.Record{ID: "r-" + name, Project: g4TeamRoot, Run: g4TeamRun, Attempt: "r.a1", Revision: g4TeamRev, Name: name, Kind: kind,
			SHA256: put(b), Size: int64(len(b)), State: records.StateCommitted}
	}
	app := rec("app", records.KindBinary, []byte(g4TeamBytes))
	fs := &g4TeamStore{recs: []records.Record{app, rec("app.cdx.json", records.KindSBOM, []byte("{}"))}}
	g4Current.Store(&remote.Deps{Runs: g4TeamRuns{}, Store: fs, Blobs: w.Blobs})
	t.Cleanup(func() { g4Current.Store(nil) })
	tm := &g4Team{w: w, app: app.SHA256}
	var err error
	if tm.ca, err = pki.New(w.ctx, pki.Options{Store: w.St, DataDir: w.Home}); err != nil {
		t.Fatal(err)
	}
	tm.svc = roles.New(w.St, &audit.Log{St: w.St})
	tm.svc.Self = "alice"
	leaf, err := tm.ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	api := clientapi.New(clientapi.Config{Roles: tm.svc, SubmitRole: roles.Member, Allow: func(p string) bool { return p == g4TeamProject }})
	srv := httptest.NewUnstartedServer(api.Handler())
	srv.Listener = tls.NewListener(srv.Listener, clientapi.TLSConfig(tm.ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(w.St)))
	srv.Start()
	t.Cleanup(srv.Close)
	tm.url = "https://" + srv.Listener.Addr().String()
	return tm
}

// client is a coordinator client holding a scoped token of role on projects.
func (tm *g4Team) client(role roles.Role, projects ...string) *coordclient.Client {
	t := tm.w.t
	t.Helper()
	tok, _, err := tm.svc.IssueToken(tm.w.ctx, tm.svc.Operator("cli:alice"), roles.Request{Principal: "u-" + string(role), Role: role, Projects: projects, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	tf, root := filepath.Join(dir, "token"), filepath.Join(dir, "root.pem")
	if err = os.WriteFile(tf, []byte(tok+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tm.ca.RootDER()}), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := coordclient.New(coordclient.Target{Kind: coordclient.Remote, URL: tm.url, Creds: coordclient.Creds{CA: root, TokenFile: tf}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// get fetches path as c and returns the status, the raw body and the decoded refusal body (zero when 2xx).
func (tm *g4Team) get(c *coordclient.Client, path string) (int, []byte, clientapi.Body) {
	t := tm.w.t
	t.Helper()
	resp, err := c.GetRaw(tm.w.ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	var body clientapi.Body
	if resp.StatusCode >= 400 {
		if err = json.Unmarshal(b, &body); err != nil {
			t.Fatalf("refusal body is not a user-API body: %s", b)
		}
	}
	return resp.StatusCode, b, body
}
