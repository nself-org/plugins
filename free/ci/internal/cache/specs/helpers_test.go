package specs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/detect"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const fakeSecret = "fake-registered-value-0123456789abcdef"

var (
	ownerFacts   = entries.Facts{Project: "/work/proj", Trust: "owner", Protection: "protected"}
	untrustFacts = entries.Facts{Project: "/work/proj", Trust: "untrusted", Protection: "unprotected", HeadRepo: "fork/example", PR: "7"}
	goFacts      = []detect.Fact{
		{Kind: "stack", Name: "go", Path: "go.mod", Confidence: detect.Certain},
		{Kind: "lockfile", Name: "go", Path: "go.sum", SHA256: strings.Repeat("a1", 32), Confidence: detect.Certain},
	}
)

type env struct {
	t     *testing.T
	ctx   context.Context
	st    *store.Store
	blobs *cas.Store
	es    *entries.Store
	eng   *Engine
	home  string
	scan  *redact.Redactor
	mu    sync.Mutex
	audit []store.AuditRow
}

func newEnv(t *testing.T) *env {
	t.Helper()
	e := &env{t: t, ctx: context.Background(), home: t.TempDir(), scan: redact.New()}
	var err error
	if e.st, err = store.Open(filepath.Join(e.home, "state.db"), store.Options{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.st.Close() })
	e.st.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		e.audit = append(e.audit, row)
		return store.AuditChain{Head: []byte("h"), Signature: []byte("s")}, nil
	})
	if e.blobs, err = cas.New(e.home); err != nil {
		t.Fatal(err)
	}
	e.es = entries.New(e.st, e.blobs, entries.Options{Home: e.home, Free: func() (int64, int64, error) { return 1 << 40, 1 << 41, nil },
		Project: func(string) authz.ProjectFacts { return authz.ProjectFacts{NoPrivateDeps: true} }})
	e.eng = &Engine{Entries: e.es, Blobs: e.blobs, Scanner: e.scan, Backend: e.st}
	return e
}

// running creates attempt id (running, launch epoch 2) of a pipeline for f.
func (e *env) running(id string, f entries.Facts) {
	e.t.Helper()
	ctx, s := e.ctx, e.st
	for i, err := range []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		s.Transition(ctx, id, "queued", "leased", store.TransitionDetail{Epoch: 0}),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
	} {
		if err != nil {
			e.t.Fatalf("attempt %s step %d: %v", id, i, err)
		}
	}
}

// finalizing moves id to finalizing and returns the epoch a finalize hook sees.
func (e *env) finalizing(id string) int64 {
	e.t.Helper()
	if err := e.st.Transition(e.ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}); err != nil {
		e.t.Fatal(err)
	}
	return 3
}

func probeGo(context.Context, []string) (string, error) {
	return "go version go1.27.2 linux/amd64\n", nil
}

// input is a BeginInput for attempt id with the Go fixture facts.
func (e *env) input(id string, f entries.Facts) BeginInput {
	return BeginInput{Engine: e.eng, Attempt: id, Epoch: 2, Facts: f, Root: filepath.Join(e.home, "cache-work", id), Detected: goFacts,
		Platform: "linux/amd64", Isolation: "process", Probe: probeGo, Getenv: func(string) string { return "" }, Warn: func(s string) { e.t.Log("warn: " + s) }}
}

func (e *env) begin(in BeginInput) *Session {
	e.t.Helper()
	s, err := Begin(e.ctx, in)
	if err != nil {
		e.t.Fatalf("Begin: %v", err)
	}
	return s
}

func (e *env) uses(attempt string) []entries.Use {
	e.t.Helper()
	u, err := e.es.Usage(e.ctx, attempt)
	if err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) auditActions(action string) (n int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, r := range e.audit {
		if r.Action == action {
			n++
		}
	}
	return n
}

func (e *env) entryCount() int {
	e.t.Helper()
	l, err := e.st.CacheEntries(e.ctx, "")
	if err != nil {
		e.t.Fatal(err)
	}
	return len(l)
}

func putFile(t *testing.T, p, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func getFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func resultsOf(us []entries.Use) (out []string) {
	for _, u := range us {
		out = append(out, u.Spec+":"+u.Result)
	}
	return out
}

func readIfExists(p string) ([]byte, error) { return os.ReadFile(p) }

func storeDetail(epoch int64) store.TransitionDetail { return store.TransitionDetail{Epoch: epoch} }

func casRealm(ns authz.Namespace) cas.Realm { return cas.CacheRealm(string(ns.Canonical())) }

const snapshotImage = "sha256:" + "9999999999999999999999999999999999999999999999999999999999999999"

// containerInput is input for a container-level attempt of the snapshot image.
func (e *env) containerInput(id string, f entries.Facts) BeginInput {
	in := e.input(id, f)
	in.Container, in.Isolation, in.Image = true, "container", snapshotImage
	return in
}

func (e *env) beginUntrustedContainer(id string) *Session {
	e.t.Helper()
	e.running(id, untrustFacts)
	return e.begin(e.containerInput(id, untrustFacts))
}

// seedSnapshot stores the owner's protected entry for the container-level key. It is a
// SYNTHETIC writer: no real attempt produces a container-level entry yet (a container
// job cannot populate a cache; debt), so the read-only paths are tested against it.
// with one file f.txt per dir holding the given content. A container job
// cannot populate a cache itself, so the entry goes in through the engine.
func (e *env) seedSnapshot(id string, contents map[string]string) {
	e.t.Helper()
	e.running(id, ownerFacts)
	ns, err := entries.NamespaceOf(ownerFacts)
	if err != nil {
		e.t.Fatal(err)
	}
	item, ok := (&Session{in: e.containerInput(id, ownerFacts), own: ns}).plan(e.ctx, mustGet(e.t, "go"))
	if !ok {
		e.t.Fatal("plan")
	}
	if err := e.es.RecordPlan(e.ctx, id, 2, ownerFacts, []string{item.Key}); err != nil {
		e.t.Fatal(err)
	}
	dirs := Dirs{}
	seed := filepath.Join(e.home, "seed-"+id)
	for name, content := range contents {
		dirs[name] = name
		putFile(e.t, filepath.Join(seed, name, "f.txt"), content)
	}
	base, err := os.OpenRoot(seed)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = base.Close() }()
	fin := e.finalizing(id)
	if err := e.es.RecordResult(e.ctx, id, fin, true); err != nil {
		e.t.Fatal(err)
	}
	if _, err := e.eng.Save(e.ctx, SaveRequest{Base: base, Attempt: id, Epoch: fin, Namespace: ns, Key: item.Key, Spec: item.Spec, Dirs: dirs}); err != nil {
		e.t.Fatal(err)
	}
	e.settlePassed(id)
}

func mustGet(t *testing.T, id string) Spec {
	t.Helper()
	s, ok := Get(id)
	if !ok {
		t.Fatalf("no spec %s", id)
	}
	return s
}

// dir is the host path of the first item's directory name.
func (s *Session) dir(name string) string { return s.host(s.items[0].Dirs[name]) }

// settlePassed moves a finalizing attempt to passed, as the run does after the finalize hooks.
func (e *env) settlePassed(id string) {
	e.t.Helper()
	if err := e.st.Transition(e.ctx, id, "finalizing", "passed", storeDetail(3)); err != nil {
		e.t.Fatal(err)
	}
}
