package reliability

// Purpose: the coordinator side of the sim: one real store and lease engine behind an injected clock,
//          fixture jobs, grants on named runners and the fixture job's side-effect file.
// Inputs:  a *testing.T; every time moves through world.advance.
// Outputs: helpers that fail the test on any unexpected store error.
// Constraints: no sleeps; times start at time.Unix(1, 0); the side-effect file lives in the test's TempDir.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/lease"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	coordTTL  = 45 * time.Second
	heartbeat = 10 * time.Second
)

// world is one coordinator store with its lease engine and an injected clock.
type world struct {
	t       *testing.T
	ctx     context.Context
	mu      sync.Mutex
	now     time.Time
	down    bool // the coordinator process is not running
	store   *store.Store
	engine  *lease.Engine
	effects string
	audits  []store.AuditRow
}

func newWorld(t *testing.T) *world {
	t.Helper()
	w := &world{t: t, ctx: context.Background(), now: time.Unix(1, 0), effects: t.TempDir()}
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{Clock: w.clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	w.store = s
	s.SetAuditSealer(func(_ []byte, row store.AuditRow) (store.AuditChain, error) {
		w.mu.Lock()
		w.audits = append(w.audits, row)
		w.mu.Unlock()
		return store.AuditChain{}, nil
	})
	w.engine = w.coordinator()
	return w
}

// retryReasons lists the reasons of the retries queued for an attempt (audit action retry-queued).
func (w *world) retryReasons(attemptID string) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	var out []string
	for _, row := range w.audits {
		if row.Action == "retry-queued" && row.Target == attemptID {
			out = append(out, row.Detail)
		}
	}
	return out
}

// coordinator builds a fresh lease engine over the same store: a restarted coordinator process.
func (w *world) coordinator() *lease.Engine {
	return lease.New(w.store, nil, lease.Config{Clock: w.clock, Heartbeat: heartbeat, TTL: coordTTL})
}

func (w *world) clock() time.Time {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.now
}

func (w *world) advance(d time.Duration) {
	w.mu.Lock()
	w.now = w.now.Add(d)
	w.mu.Unlock()
}

// job creates a pipeline, a job and its first attempt "<id>.a1".
func (w *world) job(id, kind string, idempotent bool, infraMax int) string {
	w.t.Helper()
	p := store.PipelineRow{ID: "p-" + id, Project: "project", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := w.store.CreatePipeline(w.ctx, p); err != nil {
		w.t.Fatal(err)
	}
	if err := w.store.CreateJob(w.ctx, store.JobRow{ID: id, PipelineID: p.ID, Name: id, Kind: kind, Idempotent: idempotent, InfraMax: infraMax, InputDigest: "i"}); err != nil {
		w.t.Fatal(err)
	}
	if err := w.store.CreateAttempt(w.ctx, store.AttemptRow{ID: id + ".a1", JobID: id, N: 1}, ""); err != nil {
		w.t.Fatal(err)
	}
	return id + ".a1"
}

// grant leases a queued attempt to runner (one host per runner) and starts it; it returns the lease epoch.
func (w *world) grant(attemptID, leaseID, runner string, incarnation int64) int64 {
	w.t.Helper()
	d := store.Demand{AttemptID: attemptID, LeaseID: leaseID, RunnerID: runner, CPU: 1, MemMB: 1, CapacityCPU: 100, CapacityMemMB: 100, TTL: coordTTL}
	epoch, err := w.engine.Grant(w.ctx, "host-"+runner, d, incarnation)
	if err != nil {
		w.t.Fatalf("grant %s on %s: %v", attemptID, runner, err)
	}
	_, attemptEpoch := w.state(attemptID)
	if err := w.store.Transition(w.ctx, attemptID, "leased", "running", store.TransitionDetail{Epoch: attemptEpoch}); err != nil {
		w.t.Fatal(err)
	}
	return epoch
}

func (w *world) state(attemptID string) (string, int64) {
	w.t.Helper()
	st, epoch, err := w.store.AttemptStateEpoch(w.ctx, attemptID)
	if err != nil {
		w.t.Fatal(err)
	}
	return string(st), epoch
}

func (w *world) attempts(jobID string) []store.AttemptView {
	w.t.Helper()
	rows, err := w.store.AttemptsByJob(w.ctx, jobID)
	if err != nil {
		w.t.Fatal(err)
	}
	return rows
}

// active counts attempts of a job that hold or may hold a lease.
func (w *world) active(jobID string) int {
	n := 0
	for _, a := range w.attempts(jobID) {
		if a.State == "leased" || a.State == "running" || a.State == "finalizing" {
			n++
		}
	}
	return n
}

// wantAttempts checks the evidence: the state and failure_class of every attempt in order.
func (w *world) wantAttempts(jobID string, want ...string) []store.AttemptView {
	w.t.Helper()
	rows := w.attempts(jobID)
	got := make([]string, len(rows))
	for i, a := range rows {
		got[i] = a.State + "/" + a.FailureClass
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		w.t.Fatalf("job %s attempts = %v, want %v", jobID, got, want)
	}
	return rows
}

func (w *world) previousRunner(attemptID string) string {
	w.t.Helper()
	r, err := w.store.LeasePreviousRunner(w.ctx, attemptID)
	if err != nil {
		w.t.Fatal(err)
	}
	return r
}

// effect is the fixture job's side effect: one line per execution, stamped with the world clock.
func (w *world) effect(jobID, runner string) {
	w.t.Helper()
	f, err := os.OpenFile(filepath.Join(w.effects, jobID), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		w.t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "%s %d\n", runner, w.clock().Unix()); err != nil {
		w.t.Fatal(err)
	}
}

// effects returns the lines of a job's side-effect file.
func (w *world) effectLines(jobID string) []string {
	w.t.Helper()
	b, err := os.ReadFile(filepath.Join(w.effects, jobID))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		w.t.Fatal(err)
	}
	return strings.Fields(strings.ReplaceAll(strings.TrimSpace(string(b)), " ", "@"))
}

// wantEffect asserts the side-effect file holds exactly one line, written by runner.
func (w *world) wantEffect(jobID, runner string) {
	w.t.Helper()
	lines := w.effectLines(jobID)
	if len(lines) != 1 || !strings.HasPrefix(lines[0], runner+"@") {
		w.t.Fatalf("side effect of %s = %v, want one write by %s", jobID, lines, runner)
	}
}

func hasCode(err error, code string) bool {
	var coded *store.Error
	return errors.As(err, &coded) && coded.Code == code
}

func wantCode(t *testing.T, what string, err error, code string) {
	t.Helper()
	if !hasCode(err, code) {
		t.Fatalf("%s: got %v, want %s", what, err, code)
	}
}
