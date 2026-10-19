package job_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/release/job"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const (
	jobRow  = "run-1.publish-1a2b3c4d"
	attempt = jobRow + ".a1"
)

type auditLog struct {
	mu   sync.Mutex
	rows []store.AuditRow
}

func (a *auditLog) seal(_ []byte, row store.AuditRow) (store.AuditChain, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rows = append(a.rows, row)
	return store.AuditChain{Head: []byte("h")}, nil
}

func (a *auditLog) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.rows)
}

func (a *auditLog) last() store.AuditRow {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.rows[len(a.rows)-1]
}

// seed stores a run with one release job and one attempt that reached `state`.
// It returns the store and the audit log, which starts empty after the seeding.
func seed(t *testing.T, kind, state string) (*store.Store, *auditLog) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(st.CreatePipeline(ctx, store.PipelineRow{ID: "run-1", Revision: revision, Project: "proj", Trigger: "local", SourceTrust: "owner",
		PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "running"}))
	must(st.CreateJob(ctx, store.JobRow{ID: jobRow, PipelineID: "run-1", Name: "publish", Kind: kind, Idempotent: kind != "release", InputDigest: "i"}))
	must(st.CreateAttempt(ctx, store.AttemptRow{ID: attempt, JobID: jobRow, N: 1}, ""))
	path := map[string][]model.JobState{
		"queued":         {},
		"running":        {"leased", "running"},
		"finalizing":     {"leased", "running", "finalizing"},
		"passed":         {"leased", "running", "finalizing", "passed"},
		"needs-operator": {"leased", "lost", "needs-operator"},
	}[state]
	from := model.JobState("queued")
	for _, to := range path {
		_, epoch, err := st.AttemptStateEpoch(ctx, attempt)
		must(err)
		must(st.Transition(ctx, attempt, from, to, store.TransitionDetail{Epoch: epoch, FailureClass: "lost", Operator: false}))
		from = to
	}
	log := &auditLog{}
	st.SetAuditSealer(log.seal)
	return st, log
}

func state(t *testing.T, st *store.Store, id string) string {
	t.Helper()
	s, _, err := st.AttemptStateEpoch(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return string(s)
}

func wantE716(t *testing.T, err error, reason string) {
	t.Helper()
	var e *job.Error
	if !errors.As(err, &e) || e.Code != job.CodeResolveRefused || e.Reason != reason {
		t.Fatalf("err = %v, want E716 %s", err, reason)
	}
}

func resolve(st *store.Store, id, key string, a job.Action) (job.Result, error) {
	return job.Resolve(context.Background(), st, job.Request{Attempt: id, Key: key, Action: a, Actor: "operator:test"})
}

// TestResolveKeyMismatch: a wrong key changes nothing (E716 key_mismatch).
func TestResolveKeyMismatch(t *testing.T) {
	st, log := seed(t, "release", "needs-operator")
	for _, key := range []string{"", "wrong", strings.Repeat("0", 64), job.IdempotencyKey(attempt) + "0", strings.ToUpper(job.IdempotencyKey(attempt)), job.IdempotencyKey(jobRow + ".a2")} {
		for _, a := range []job.Action{job.Rerun, job.Abandon} {
			_, err := resolve(st, attempt, key, a)
			wantE716(t, err, job.KeyMismatch)
		}
	}
	if log.count() != 0 || state(t, st, attempt) != "needs-operator" {
		t.Fatalf("a refused resolution wrote %d audit rows, state %s", log.count(), state(t, st, attempt))
	}
}

// TestResolveRerunApprovesAttemptNPlusOne: the right key writes one audit row
// and the store then accepts attempt n+1 (and refused it before).
func TestResolveRerunApprovesAttemptNPlusOne(t *testing.T) {
	st, log := seed(t, "release", "needs-operator")
	ctx := context.Background()
	next := jobRow + ".a2"
	if err := st.CreateAttempt(ctx, store.AttemptRow{ID: next, JobID: jobRow, N: 2}, ""); err == nil || !strings.Contains(err.Error(), "E609") {
		t.Fatalf("attempt n+1 before the operator decision: %v", err)
	}
	res, err := resolve(st, attempt, job.IdempotencyKey(attempt), job.Rerun)
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt != attempt || res.Action != job.Rerun || res.State != "needs-operator" || res.Next != next {
		t.Fatalf("result %+v", res)
	}
	if log.count() != 1 || log.last().Action != "operator-retry" || log.last().Target != attempt || log.last().Actor != "operator:test" {
		t.Fatalf("audit rows %d, last %+v", log.count(), log.last())
	}
	if strings.Contains(log.last().Detail, job.IdempotencyKey(attempt)) {
		t.Fatal("the key reached the audit trail")
	}
	if err = st.CreateAttempt(ctx, store.AttemptRow{ID: next, JobID: jobRow, N: 2}, ""); err != nil {
		t.Fatalf("attempt n+1 after the operator decision: %v", err)
	}
	// once n+1 exists the attempt is resolved
	_, err = resolve(st, attempt, job.IdempotencyKey(attempt), job.Rerun)
	wantE716(t, err, job.AlreadyResolved)
	_, err = resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon)
	wantE716(t, err, job.AlreadyResolved)
	if log.count() != 1 {
		t.Fatalf("a refused repeat wrote audit rows: %d", log.count())
	}
}

func TestResolveAbandonFailsTheAttemptWithOneAuditRow(t *testing.T) {
	st, log := seed(t, "release", "needs-operator")
	res, err := job.Resolve(context.Background(), st, job.Request{Attempt: attempt, Key: job.IdempotencyKey(attempt), Action: job.Abandon, Actor: "operator:test", Reason: "publish never started"})
	if err != nil {
		t.Fatal(err)
	}
	if res.State != "needs-operator" || res.Next != "" || state(t, st, attempt) != "failed" {
		t.Fatalf("result %+v state %s", res, state(t, st, attempt))
	}
	if log.count() != 1 || log.last().Actor != "operator:test" || !strings.Contains(log.last().Detail, "abandoned: publish never started") {
		t.Fatalf("audit rows %d, last %+v", log.count(), log.last())
	}
	if err = st.CreateAttempt(context.Background(), store.AttemptRow{ID: jobRow + ".a2", JobID: jobRow, N: 2}, ""); err == nil {
		t.Fatal("an abandoned release attempt was retried")
	}
	_, err = resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon)
	wantE716(t, err, job.NotResolvable)
}

func TestResolveDefaultsTheActorAndReason(t *testing.T) {
	st, log := seed(t, "release", "needs-operator")
	if _, err := job.Resolve(context.Background(), st, job.Request{Attempt: attempt, Key: job.IdempotencyKey(attempt), Action: job.Abandon}); err != nil {
		t.Fatal(err)
	}
	if log.last().Actor != "operator" || !strings.Contains(log.last().Detail, "resolved by `nself ci runs resolve`") {
		t.Fatalf("audit %+v", log.last())
	}
}

func TestResolveFinalizingReleaseAttemptCanOnlyBeAbandoned(t *testing.T) {
	st, log := seed(t, "release", "finalizing")
	_, err := resolve(st, attempt, job.IdempotencyKey(attempt), job.Rerun)
	wantE716(t, err, job.NotResolvable)
	if state(t, st, attempt) != "finalizing" || log.count() != 0 {
		t.Fatal("a refused rerun changed the attempt")
	}
	if _, err = resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon); err != nil {
		t.Fatal(err)
	}
	if state(t, st, attempt) != "failed" || log.count() != 1 {
		t.Fatalf("state %s audit %d", state(t, st, attempt), log.count())
	}
}

func TestResolveRefusesWhatIsNotStuck(t *testing.T) {
	for _, s := range []string{"queued", "running", "passed"} {
		st, log := seed(t, "release", s)
		for _, a := range []job.Action{job.Rerun, job.Abandon} {
			_, err := resolve(st, attempt, job.IdempotencyKey(attempt), a)
			wantE716(t, err, job.NotResolvable)
		}
		if log.count() != 0 || state(t, st, attempt) != s {
			t.Errorf("%s: changed (%s, %d audit rows)", s, state(t, st, attempt), log.count())
		}
	}
	// finalizing is only resolvable for a release attempt
	st, _ := seed(t, "test", "finalizing")
	_, err := resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon)
	wantE716(t, err, job.NotResolvable)
	// a non-release at-most-once attempt that needs an operator is resolvable
	st, _ = seed(t, "test", "needs-operator")
	if _, err = resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon); err != nil {
		t.Fatal(err)
	}
}

func TestResolveUnknownAttemptAndBadAction(t *testing.T) {
	st, _ := seed(t, "release", "needs-operator")
	for _, id := range []string{"", "nope", "run-1.other.a1", "run-1." + "x", "run-10.publish.a1"} {
		_, err := resolve(st, id, job.IdempotencyKey(id), job.Rerun)
		wantE716(t, err, job.NotResolvable)
	}
	_, err := resolve(st, attempt, job.IdempotencyKey(attempt), "retry")
	wantE716(t, err, job.NotResolvable)
}

func TestResolveFindsTheAttemptAmongManyRuns(t *testing.T) {
	st, _ := seed(t, "release", "needs-operator")
	ctx := context.Background()
	for i := 0; i < 130; i++ { // more runs than one history page
		id := "other-" + strings.Repeat("x", i%5) + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if err := st.CreatePipeline(ctx, store.PipelineRow{ID: id, Revision: revision, Project: "p", Trigger: "local", SourceTrust: "owner",
			PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "passed"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := resolve(st, attempt, job.IdempotencyKey(attempt), job.Abandon); err != nil {
		t.Fatalf("the attempt of an older run was not found: %v", err)
	}
}

// TestResolveMapsALostCompareAndSetToAlreadyResolved: a concurrent resolution
// (E605 epoch moved, E604 state moved) is E716 already_resolved, anything else
// passes through untouched.
func TestResolveMapsALostCompareAndSetToAlreadyResolved(t *testing.T) {
	for _, code := range []string{"E605", "E604"} {
		wantE716(t, job.MapConflict(&store.Error{Code: code, Message: "moved"}), job.AlreadyResolved)
	}
	other := &store.Error{Code: "E609", Message: "operator action required"}
	if got := job.MapConflict(other); got != error(other) {
		t.Fatalf("E609 was rewritten: %v", got)
	}
	plain := errors.New("disk full")
	if got := job.MapConflict(plain); got != plain {
		t.Fatalf("a plain error was rewritten: %v", got)
	}
	if job.MapConflict(nil) != nil {
		t.Fatal("nil became an error")
	}
}
