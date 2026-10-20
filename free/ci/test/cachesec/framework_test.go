//go:build cachesec

package cachesec

// Purpose: the shared frame of the cache, artifact and release security suite (P7-CACHE-11). A case
// is a function registered under its matrix.yaml id; it drives the real production packages and
// returns what it observed (Observed). TestCacheSecMatrix compares that with the matrix row.
// Inputs:  none. Outputs: Observed, register, newWorld (a real store with an audit sink, a cas store
//          and a clock), and the small assertion helpers every case uses.
// Constraints: a case never weakens a production check to pass and never reaches the network except
//          through the fabric sim; documentation addresses only (192.0.2.0/24, 2001:db8::/32). A refused
//          case reports the byte count that LANDED in the store it attacked: bytes reachable through a
//          lookup, an artifact record or a release directory afterwards (an unreferenced staging blob
//          is not reachable and is swept by cas.Sweep, so it does not count; the case says so when it
//          Puts one).

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/storetest"
)

const (
	Refused  = "refused"
	Admitted = "admitted"
)

// Observed is what one case saw. Outcome is Refused or Admitted; Code is the fabric code ("E711"),
// Reason its reason key, Audit the audit action recorded for the refusal, Landed the bytes that
// reached the attacked store.
type Observed struct {
	Outcome string
	Code    string
	Reason  string
	Audit   string
	Landed  int64
}

var registry = map[string]func(t *testing.T) Observed{}

// register binds a matrix.yaml case id to its function. Call it from init() of the case file.
func register(id string, fn func(t *testing.T) Observed) {
	if _, dup := registry[id]; dup {
		panic("cachesec: case registered twice: " + id)
	}
	registry[id] = fn
}

func registered() []string {
	out := make([]string, 0, len(registry))
	for id := range registry {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// coder is any error that carries a fabric code (entries.Refused, authz.Denied, records and cas errors).
type coder interface{ Code() string }

// refusal turns an error into the Observed code and reason keys. It fails the test when err is nil
// (an unexpected pass) or carries no code. reasonOf extracts the reason key from the typed error.
func refusal(t *testing.T, err error, reasonOf func(error) string) Observed {
	t.Helper()
	if err == nil {
		t.Fatal("the operation was admitted; the matrix expects a refusal")
	}
	var c coder
	if !errors.As(err, &c) {
		t.Fatalf("the refusal carries no fabric code: %v", err)
	}
	return Observed{Outcome: Refused, Code: c.Code(), Reason: reasonOf(err)}
}

// world is a real coordinator-side store with an audit sink, a cas store and a manual clock.
type world struct {
	t     *testing.T
	ctx   context.Context
	Home  string
	St    *store.Store
	Blobs *cas.Store
	Es    *entries.Store
	clock atomic.Int64
	mu    sync.Mutex
	audit []store.AuditRow
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t, ctx: context.Background(), Home: t.TempDir()}
	w.clock.Store(time.Now().UnixNano())
	now := func() time.Time { return time.Unix(0, w.clock.Load()) }
	var err error
	if w.St, err = store.Open(filepath.Join(w.Home, "state.db"), store.Options{Clock: now}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.St.Close() })
	w.St.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		w.mu.Lock()
		defer w.mu.Unlock()
		w.audit = append(w.audit, row)
		return store.AuditChain{Head: []byte("h"), Signature: []byte("s")}, nil
	})
	if w.Blobs, err = cas.New(w.Home); err != nil {
		t.Fatal(err)
	}
	w.Es = entries.New(w.St, w.Blobs, entries.Options{Home: w.Home, Clock: now, Getenv: func(string) string { return "" }})
	return w
}

// Audit returns the audit rows recorded with the given action (all rows when action is "").
func (w *world) Audit(action string) (out []store.AuditRow) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, r := range w.audit {
		if action == "" || r.Action == action {
			out = append(out, r)
		}
	}
	return out
}

// AuditActions lists the distinct audit actions seen so far, sorted.
func (w *world) AuditActions() []string {
	seen := map[string]bool{}
	for _, r := range w.Audit("") {
		seen[r.Action] = true
	}
	out := make([]string, 0, len(seen))
	for a := range seen {
		out = append(out, a)
	}
	sort.Strings(out)
	return out
}

// Attempt creates attempt id for facts f, records the plan while running, moves it to finalizing and
// records the result (the shape every cache save goes through). It returns the finalizing epoch.
func (w *world) Attempt(id string, f entries.Facts, keys []string, passed bool) int64 {
	w.t.Helper()
	ctx, s := w.ctx, w.St
	steps := []error{
		s.CreatePipeline(ctx, store.PipelineRow{ID: "p" + id, Revision: "rev", Project: f.Project, Trigger: "local", SourceTrust: f.Trust, PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i"}),
		s.CreateJob(ctx, store.JobRow{ID: "j" + id, PipelineID: "p" + id, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}),
		s.CreateAttempt(ctx, store.AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""),
		storetest.Admit(ctx, s, id),
		s.Transition(ctx, id, "leased", "running", store.TransitionDetail{Epoch: 1}),
		w.Es.RecordPlan(ctx, id, 2, f, keys),
		s.Transition(ctx, id, "running", "finalizing", store.TransitionDetail{Epoch: 2}),
		w.Es.RecordResult(ctx, id, 3, passed),
	}
	for i, err := range steps {
		if err != nil {
			w.t.Fatalf("attempt %s step %d: %v", id, i, err)
		}
	}
	return 3
}

var _ = authz.ModeNone

// NS is the cache namespace of facts f.
func (w *world) NS(f entries.Facts) authz.Namespace {
	w.t.Helper()
	ns, err := entries.NamespaceOf(f)
	if err != nil {
		w.t.Fatal(err)
	}
	return ns
}

// Key is the cache key of facts f for the "go" spec; extra varies it.
func (w *world) Key(f entries.Facts, extra string) string {
	w.t.Helper()
	k, err := entries.Key(entries.KeyInputs{Namespace: w.NS(f), SpecID: "go", SpecVersion: "1", Platform: "linux/amd64", Isolation: "container", Extras: []string{extra}})
	if err != nil {
		w.t.Fatal(err)
	}
	return k
}

// Manifest builds a manifest for key from freshly Put blobs in the cache realm of f's namespace.
func (w *world) Manifest(f entries.Facts, key string, contents ...string) entries.Manifest {
	w.t.Helper()
	ns := w.NS(f)
	m := entries.Manifest{Namespace: ns.Canonical(), Key: key, Spec: "go"}
	for i, c := range contents {
		digest, size, err := w.Blobs.Put(w.ctx, cas.CacheRealm(string(ns.Canonical())), strings.NewReader(c))
		if err != nil {
			w.t.Fatal(err)
		}
		m.Blobs = append(m.Blobs, entries.Blob{Path: fmt.Sprintf("part%02d.tar.gz", i), SHA256: digest, Size: size})
	}
	return m
}

// EntryBytes is the total size of every cache entry visible in the store: the bytes that landed.
func (w *world) EntryBytes() (n int64) {
	w.t.Helper()
	list, err := w.St.CacheEntries(w.ctx, "")
	if err != nil {
		w.t.Fatal(err)
	}
	for _, e := range list {
		n += e.Bytes()
	}
	return n
}

// AuditOf returns action when the world recorded at least one audit row with it, "" otherwise.
func (w *world) AuditOf(action string) string {
	if len(w.Audit(action)) == 0 {
		return ""
	}
	return action
}
