//go:build integration

package remote_test

// Purpose: the simulated TEAM coordinator the sign tests run against: a real git repository, the real
// coordinator running a test job and a release job on a TEAM coordinator (Team: true, so the release
// is awaiting-signature and the coordinator's secret resolver has no key and counts its calls), the
// real run store and blob store, the real user API over TLS 1.3 with a real PKI, role service and
// scoped tokens, and the two release routes registered through clientapi.RegisterRoute.
// Constraints: a missing prerequisite (git) is t.Fatal, never t.Skip; 127.0.0.1 only; every key is
// generated per test; the only fakes are the release-inputs digester seam (until internal/evidence
// registers it, D-1074) and the secret store behind each resolver.

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/compat"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/coordinator"
	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/pki"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	"github.com/nself-org/plugins/free/ci/internal/release/publish"
	"github.com/nself-org/plugins/free/ci/internal/release/remote"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/secrets"
	"github.com/nself-org/plugins/free/ci/internal/signing"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const project = "acme/app" // the owner's name for the project (the intake row); the run's own project is its checkout path

// fakeRes is a secret resolver that counts its calls.
type fakeRes struct {
	vals  map[string][]byte
	calls atomic.Int32
}

func (r *fakeRes) Probe(context.Context) error { return nil }
func (r *fakeRes) Resolve(_ context.Context, _ string, refs []secrets.Ref) ([]secrets.Value, error) {
	r.calls.Add(1)
	var out []secrets.Value
	for _, ref := range refs {
		v, ok := r.vals[ref.Raw]
		if !ok {
			return nil, &secrets.Error{Code: "E676", Message: "not found", Cause: "E541"}
		}
		out = append(out, secrets.Value{Ref: ref, Bytes: append([]byte(nil), v...)})
	}
	return out, nil
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@example.test", "-c", "user.name=t", "-c", "commit.gpgsign=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func newRepo(t *testing.T, readme string) (dir, rev string) {
	t.Helper()
	dir = t.TempDir()
	gitIn(t, dir, "init", "-q", "-b", "main")
	for name, body := range map[string]string{".gitignore": "out/\n", "README.md": readme} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, dir, "add", ".")
	gitIn(t, dir, "commit", "-q", "-m", "fixture")
	return dir, gitIn(t, dir, "rev-parse", "HEAD")
}

func yes() *bool { b := true; return &b }
func no() *bool  { b := false; return &b }

const goodRelease = `mkdir -p out && printf app-bytes > out/app && printf '{"sbom":"cdx"}' > out/app.cdx.json && printf '{"sbom":"spdx"}' > out/app.spdx.json`

// team is a TEAM coordinator that ran one release run.
type team struct {
	t         *testing.T
	repo, rev string
	home      string // the coordinator's CI home
	st        *store.Store
	runID     string
	keyValue  []byte
	coordRes  *fakeRes // the coordinator's resolver: no key, must never be called
	opRes     *fakeRes // the operator's resolver: holds the release key
	blobs     *cas.Store
}

func newTeam(t *testing.T) *team {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Fatal("INTEGRATION=1 is required for the TEAM signing tests")
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	if !compat.V15() {
		t.Fatal("the V15 engine is not enabled")
	}
	tm := &team{t: t, home: t.TempDir(), coordRes: &fakeRes{vals: map[string][]byte{}}}
	tm.repo, tm.rev = newRepo(t, "release fixture\n")
	_, value, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	tm.keyValue = value
	tm.opRes = &fakeRes{vals: map[string][]byte{signing.KeyRef: value}}
	tm.runRelease()
	return tm
}

func (tm *team) pipeline() *plan.Plan {
	jobs := map[string]model.Job{
		"build": {Kind: "test", Run: []string{"sh", "-c", "true"}, Required: yes()},
		"publish": {Kind: "release", Run: []string{"sh", "-c", goodRelease}, Deps: []string{"build"}, Required: yes(), Idempotent: no(),
			Artifacts: []model.ArtifactDecl{{Path: "out/app", Name: "app", Kind: "binary"},
				{Path: "out/app.cdx.json", Name: "app.cdx.json", Kind: "sbom"}, {Path: "out/app.spdx.json", Name: "app.spdx.json", Kind: "sbom"}}},
	}
	pl, err := plan.Build(plan.Input{Config: model.PipelineConfig{Version: 1, Jobs: jobs}, World: plan.World{Platform: runtime.GOOS + "/" + runtime.GOARCH, CPU: 8, MemMB: 16384}})
	if err != nil {
		tm.t.Fatalf("plan: %v", err)
	}
	return pl
}

// coordDeps are the coordinator's publication dependencies: Team is set, so it records
// awaiting-signature and never calls the signer (and the resolver behind it has no key).
func (tm *team) coordDeps(st *store.Store) publish.Deps {
	blobs, err := cas.New(records.StoreDir(tm.home))
	if err != nil {
		tm.t.Fatal(err)
	}
	signer := remote.NewLocalSigner(st, blobs, tm.coordRes, tm.repo)
	return publish.Deps{Src: st, Blobs: blobs, Ledger: st, Signer: signer, Home: tm.home, Root: tm.repo, Capture: report.Capture, Team: true}
}

// runRelease runs the pipeline on the TEAM coordinator and links the run to an intake row of project.
func (tm *team) runRelease() {
	t := tm.t
	oldInputs, oldPub := run.ReleaseInputs, run.ReleasePublisher
	t.Cleanup(func() { run.ReleaseInputs, run.ReleasePublisher = oldInputs, oldPub })
	run.ReleaseInputs = func(ctx context.Context, root, rev string) (model.InputDigests, model.InputDigests, error) {
		d, err := evidence.InputsAt(ctx, report.Capture, root, rev)
		return d, d, err
	}
	bound, err := evidence.InputsAt(context.Background(), report.Capture, tm.repo, tm.rev)
	if err != nil {
		t.Fatalf("inputs: %v", err)
	}
	st, err := store.Open(filepath.Join(tm.home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	tm.st = st
	run.ReleasePublisher = func(ctx context.Context, r run.ReleasePublishRequest) (run.ReleaseOutcome, error) {
		rel, err := publish.Run(ctx, tm.coordDeps(r.Store), r.RunID)
		if err == publish.ErrNotRelease {
			return run.ReleaseOutcome{}, run.ErrNothingToPublish
		}
		return run.ReleaseOutcome{State: rel.State, Dir: rel.Dir, Message: rel.Message}, err
	}
	var out bytes.Buffer
	c := &run.Coordinator{Opts: run.Options{Root: tm.repo, Home: tm.home, NoStatus: true, Full: true}, Plan: tm.pipeline(), ConfigDigest: bound.Config, Out: &out, Poll: 10 * time.Millisecond}
	res, err := c.Run(context.Background())
	if err != nil || res.Exit != 0 {
		t.Fatalf("Run: exit %d: %v\n%s", res.Exit, err, out.String())
	}
	tm.runID = res.RunID
	row, ok, _ := st.ReleaseFor(context.Background(), tm.runID)
	if !ok || row.State != store.ReleaseAwaiting {
		t.Fatalf("the TEAM coordinator recorded %+v, want awaiting-signature", row)
	}
	// the user API names a run's project through its intake row
	co, err := coordinator.New(coordinator.Config{Store: st, PolicyDigest: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = co.Enqueue(context.Background(), coordinator.Delivery{Source: "manual", Project: project, Revision: tm.rev, TriggerClass: "change"}); err != nil {
		t.Fatal(err)
	}
	in, ok, err := st.IntakeClaim(context.Background(), "coord-sim")
	if err != nil || !ok {
		t.Fatalf("claim: %v", err)
	}
	if err = st.IntakeFinish(context.Background(), in.ID, "coord-sim", tm.runID, "ok", true); err != nil {
		t.Fatal(err)
	}
	if tm.blobs, err = cas.New(records.StoreDir(tm.home)); err != nil {
		t.Fatal(err)
	}
}

// recorder keeps every request line and body the coordinator saw and every byte it answered.
type recorder struct {
	mu       sync.Mutex
	requests []string
	bodies   [][]byte
	answered bytes.Buffer
}

func (r *recorder) wrap(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body bytes.Buffer
		if req.Body != nil {
			_, _ = body.ReadFrom(req.Body)
		}
		req.Body = nopCloser{bytes.NewReader(body.Bytes())}
		r.mu.Lock()
		r.requests = append(r.requests, req.Method+" "+req.URL.Path)
		r.bodies = append(r.bodies, body.Bytes())
		r.mu.Unlock()
		h.ServeHTTP(&tee{ResponseWriter: w, r: r}, req)
	})
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

type tee struct {
	http.ResponseWriter
	r *recorder
}

func (t *tee) Write(b []byte) (int, error) {
	t.r.mu.Lock()
	t.r.answered.Write(b)
	t.r.mu.Unlock()
	return t.ResponseWriter.Write(b)
}

var (
	routesOnce sync.Once
	current    atomic.Pointer[remote.Deps]
)

// sim is the coordinator's user API over TLS 1.3.
type sim struct {
	t   *testing.T
	tm  *team
	ca  *pki.CA
	svc *roles.Service
	srv *httptest.Server
	url string
	rec *recorder
}

// newSim serves the user API over tm's store. blobs replaces the coordinator's blob store (nil: the real one).
func newSim(t *testing.T, tm *team, blobs signing.Blobs) *sim {
	t.Helper()
	routesOnce.Do(func() {
		h := remote.Handlers{Deps: func() (remote.Deps, bool) {
			d := current.Load()
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
	if blobs == nil {
		blobs = tm.blobs
	}
	backend := &clientapi.StoreBackend{Store: tm.st, Home: tm.home}
	current.Store(&remote.Deps{Runs: backend, Store: tm.st, Blobs: blobs})
	t.Cleanup(func() { current.Store(nil) })
	s := &sim{t: t, tm: tm, rec: &recorder{}}
	var err error
	if s.ca, err = pki.New(context.Background(), pki.Options{Store: tm.st, DataDir: tm.home}); err != nil {
		t.Fatal(err)
	}
	s.svc = roles.New(tm.st, &audit.Log{St: tm.st})
	s.svc.Self = "alice"
	leaf, err := s.ca.IssueServer("coord-test", []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")}, 0)
	if err != nil {
		t.Fatal(err)
	}
	api := clientapi.New(clientapi.Config{Roles: s.svc, SubmitRole: roles.Member, Allow: func(p string) bool { return p == project }, Backend: backend})
	s.srv = httptest.NewUnstartedServer(s.rec.wrap(api.Handler()))
	s.srv.Listener = tls.NewListener(s.srv.Listener, clientapi.TLSConfig(s.ca, func() (tls.Certificate, error) { return leaf, nil }, pki.NewGuard(tm.st)))
	s.srv.Start()
	s.url = "https://" + s.srv.Listener.Addr().String()
	t.Cleanup(s.srv.Close)
	return s
}

func (s *sim) rootFile() string {
	p := filepath.Join(s.t.TempDir(), "root.pem")
	if err := os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: s.ca.RootDER()}), 0o600); err != nil {
		s.t.Fatal(err)
	}
	return p
}

// client is a coordinator client holding a scoped token of role on projects.
func (s *sim) client(role roles.Role, projects ...string) *coordclient.Client {
	s.t.Helper()
	op := s.svc.Operator("cli:alice")
	tok, _, err := s.svc.IssueToken(context.Background(), op, roles.Request{Principal: "u-" + string(role), Role: role, Projects: projects, TTL: time.Hour})
	if err != nil {
		s.t.Fatal(err)
	}
	tf := filepath.Join(s.t.TempDir(), "token")
	if err = os.WriteFile(tf, []byte(tok+"\n"), 0o600); err != nil {
		s.t.Fatal(err)
	}
	c, err := coordclient.New(coordclient.Target{Kind: coordclient.Remote, URL: s.url, Creds: coordclient.Creds{CA: s.rootFile(), TokenFile: tf}})
	if err != nil {
		s.t.Fatal(err)
	}
	return c
}

// params builds the operator's sign request; the operator's checkout and CI home are fresh.
func (s *sim) params(c remote.Fetcher, out string) remote.Params {
	home := s.t.TempDir()
	return remote.Params{Fetcher: c, Run: s.tm.runID, Out: out, Cwd: s.t.TempDir(), Root: s.tm.repo, OperatorHome: home, Capture: report.Capture, Coordinator: s.url,
		NewSigner: func(src signing.Source, blobs *cas.Store, project string) publish.Signer {
			return remote.NewLocalSigner(src, blobs, s.tm.opRes, project)
		}}
}
