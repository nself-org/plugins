package store

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}
func seed(t *testing.T, s *Store, id string) {
	t.Helper()
	ctx := context.Background()
	p := PipelineRow{ID: "p" + id, Revision: "rev", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := s.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, JobRow{ID: "j" + id, PipelineID: p.ID, Name: id, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: id, JobID: "j" + id, N: 1}, ""); err != nil {
		t.Fatal(err)
	}
}
func codeIs(err error, code string) bool { var e *Error; return errors.As(err, &e) && e.Code == code }

func TestTransitionTableExhaustive(t *testing.T) {
	states := model.EnumValues("JobState")
	allowed := map[string]bool{"queued>leased": true, "queued>cancelled": true, "leased>running": true, "leased>lost": true, "leased>cancelled": true, "running>finalizing": true, "running>failed": true, "running>lost": true, "running>cancelled": true, "finalizing>passed": true, "finalizing>failed": true, "finalizing>cancelled": true, "lost>retryable": true, "lost>needs-operator": true, "retryable>queued": true, "needs-operator>queued": true, "needs-operator>failed": true, "needs-operator>cancelled": true, "failed>queued": true}
	for _, from := range states {
		for _, to := range states {
			key := from + ">" + to
			if Allowed(model.JobState(from), model.JobState(to)) != allowed[key] {
				t.Fatalf("incorrect pair %s", key)
			}
		}
	}
}

func TestFinalizingAutomatic(t *testing.T) {
	ctx := context.Background()
	advance := func(t *testing.T, s *Store) {
		t.Helper()
		for _, step := range []struct {
			from, to model.JobState
			epoch    int64
		}{{"queued", "leased", 0}, {"leased", "running", 1}, {"running", "finalizing", 2}} {
			if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Run("no declared artifacts pass without operator", func(t *testing.T) {
		s := testStore(t)
		seed(t, s, "a")
		advance(t, s)
		if err := s.Transition(ctx, "a", "finalizing", "passed", TransitionDetail{Epoch: 3}); err != nil {
			t.Fatal(err)
		}
		state, _, err := s.AttemptStateEpoch(ctx, "a")
		if err != nil || state != "passed" {
			t.Fatalf("state = %s, err = %v", state, err)
		}
	})
	t.Run("uncommitted artifact fails infra without operator", func(t *testing.T) {
		s := testStore(t)
		seed(t, s, "a")
		if err := s.DeclareArtifacts(ctx, "a", []model.ArtifactDecl{{Name: "out@sha256:" + strings.Repeat("a", 64), Path: "out"}}); err != nil {
			t.Fatal(err)
		}
		advance(t, s)
		if err := s.Transition(ctx, "a", "finalizing", "passed", TransitionDetail{Epoch: 3}); err != nil {
			t.Fatal(err)
		}
		var state, class string
		if err := s.readers.QueryRowContext(ctx, "SELECT state,failure_class FROM attempt WHERE id='a'").Scan(&state, &class); err != nil || state != "failed" || class != "infra" {
			t.Fatalf("state = %s, class = %s, err = %v", state, class, err)
		}
	})
	t.Run("needs operator requires authority and audit", func(t *testing.T) {
		s := testStore(t)
		seed(t, s, "a")
		if err := s.Transition(ctx, "a", "queued", "leased", TransitionDetail{Epoch: 0}); err != nil {
			t.Fatal(err)
		}
		if err := s.Transition(ctx, "a", "leased", "lost", TransitionDetail{Epoch: 1}); err != nil {
			t.Fatal(err)
		}
		if err := s.Transition(ctx, "a", "lost", "needs-operator", TransitionDetail{Epoch: 2}); err != nil {
			t.Fatal(err)
		}
		var before, after int
		if err := s.readers.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&before); err != nil {
			t.Fatal(err)
		}
		if err := s.Transition(ctx, "a", "needs-operator", "failed", TransitionDetail{Epoch: 3}); !codeIs(err, "E609") {
			t.Fatalf("without operator = %v", err)
		}
		if err := s.Transition(ctx, "a", "needs-operator", "failed", TransitionDetail{Epoch: 3, Operator: true, Actor: "operator"}); err != nil {
			t.Fatal(err)
		}
		if err := s.readers.QueryRowContext(ctx, "SELECT count(*) FROM audit").Scan(&after); err != nil || after != before+1 {
			t.Fatalf("audit rows before = %d, after = %d, err = %v", before, after, err)
		}
	})
}

func TestTransitionCASAndCancel(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	if err := s.Transition(ctx, "a", "queued", "leased", TransitionDetail{Epoch: 0}); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "a", "leased", "running", TransitionDetail{Epoch: 0}); !codeIs(err, "E605") {
		t.Fatalf("stale CAS: %v", err)
	}
	if err := s.RequestCancel(ctx, "a", "stop", 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Transition(ctx, "a", "leased", "running", TransitionDetail{Epoch: 1}); !codeIs(err, "E605") {
		t.Fatalf("cancel lost: %v", err)
	}
	if err := s.Transition(ctx, "a", "leased", "cancelled", TransitionDetail{Epoch: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestArtifactsAndAuditRollback(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	digest := strings.Repeat("a", 64)
	name := "out@sha256:" + digest
	if err := s.DeclareArtifacts(ctx, "a", []model.ArtifactDecl{{Name: name, Path: "out"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}, {"running", "finalizing", 2}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.MarkArtifactCommitted(ctx, "a", 2, name, digest, 1); !codeIs(err, "E605") {
		t.Fatalf("stale epoch: %v", err)
	}
	if err := s.MarkArtifactCommitted(ctx, "a", 3, name, digest, 1); err != nil {
		t.Fatal(err)
	}
	s.SetAuditSealer(func([]byte, AuditRow) (AuditChain, error) { return AuditChain{}, errors.New("sealer down") })
	if err := s.AppendAudit(ctx, "operator", "test", "a", "decision"); err == nil {
		t.Fatal("sealer failure accepted")
	}
	var n int
	if err := s.readers.QueryRow("SELECT count(*) FROM audit").Scan(&n); err != nil || n != 0 {
		t.Fatalf("audit rollback: %d %v", n, err)
	}
	s.SetAuditSealer(nil)
	if err := s.Transition(ctx, "a", "finalizing", "passed", TransitionDetail{Epoch: 3, Operator: true, Actor: "operator"}); err != nil {
		t.Fatal(err)
	}
}

func TestUncommittedArtifactFails(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	digest := strings.Repeat("b", 64)
	if err := s.DeclareArtifacts(ctx, "a", []model.ArtifactDecl{{Name: "out@sha256:" + digest, Path: "out"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}, {"running", "finalizing", 2}, {"finalizing", "passed", 3}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch, Operator: step.from == "finalizing", Actor: "operator"}); err != nil {
			t.Fatal(err)
		}
	}
	state, _, err := s.AttemptStateEpoch(ctx, "a")
	if err != nil || state != "failed" {
		t.Fatalf("uncommitted artifact state %s: %v", state, err)
	}
}

func TestBackupRestore(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	c, err := s.RegisterCoordinator(ctx, 1, "start", "boot")
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err = s.Backup(ctx, &b); err != nil {
		t.Fatal(err)
	}
	restored, err := s.Restore(ctx, bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = restored.Close() })
	rows, err := restored.PipelinesByRevision(ctx, "rev")
	if err != nil || len(rows) != 1 {
		t.Fatalf("restored rows %d %v", len(rows), err)
	}
	var incarnation int64
	if err = restored.readers.QueryRow("SELECT incarnation FROM coordinator WHERE id=?", c.ID).Scan(&incarnation); err != nil || incarnation != c.Incarnation+1 {
		t.Fatalf("incarnation %d %v", incarnation, err)
	}
}

func TestStaleSweepLeavesLive(t *testing.T) {
	now := time.Now()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{TTL: time.Second, Clock: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	seed(t, s, "a")
	ctx := context.Background()
	c, err := s.RegisterCoordinator(ctx, 1, "start", "boot")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.writer.Exec("UPDATE attempt SET coordinator_id=?,state='running' WHERE id='a'", c.ID)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.SweepStale(ctx, "boot")
	if err != nil || n != 0 {
		t.Fatalf("live touched %d %v", n, err)
	}
	now = now.Add(4 * time.Second)
	n, err = s.SweepStale(ctx, "boot")
	if err != nil || n != 1 {
		t.Fatalf("stale untouched %d %v", n, err)
	}
}

func TestBoundedRetry(t *testing.T) {
	s := testStore(t)
	seed(t, s, "a")
	ctx := context.Background()
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a2", JobID: "ja", N: 2}, ""); !codeIs(err, "E604") {
		t.Fatalf("premature retry: %v", err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "running", 1}, {"running", "lost", 2}, {"lost", "retryable", 3}, {"retryable", "queued", 4}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a2", JobID: "ja", N: 2}, ""); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a3", JobID: "ja", N: 3}, ""); !codeIs(err, "E604") {
		t.Fatalf("unbounded retry: %v", err)
	}
}

func TestAtMostOnceNeedsOperator(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	p := PipelineRow{ID: "p", Project: "p", Revision: "r", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only", PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: "queued"}
	if err := s.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateJob(ctx, JobRow{ID: "j", PipelineID: "p", Name: "release", Kind: "release", Idempotent: false, InfraMax: 3, InputDigest: "i"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "a", JobID: "j", N: 1}, "digest"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "b", JobID: "j", N: 2}, "digest"); !codeIs(err, "E608") && !codeIs(err, "E609") {
		t.Fatalf("duplicate key accepted: %v", err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "lost", 1}} {
		if err := s.Transition(ctx, "a", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Transition(ctx, "a", "lost", "retryable", TransitionDetail{Epoch: 2}); !codeIs(err, "E604") {
		t.Fatalf("release auto retry: %v", err)
	}
	if err := s.Transition(ctx, "a", "lost", "needs-operator", TransitionDetail{Epoch: 2}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "b", JobID: "j", N: 2}, "new-digest"); !codeIs(err, "E609") {
		t.Fatalf("retry without audit: %v", err)
	}
	if err := s.OperatorRetry(ctx, "operator", "a", "approved"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "b", JobID: "j", N: 2}, "new-digest"); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		from, to model.JobState
		epoch    int64
	}{{"queued", "leased", 0}, {"leased", "lost", 1}, {"lost", "needs-operator", 2}} {
		if err := s.Transition(ctx, "b", step.from, step.to, TransitionDetail{Epoch: step.epoch}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateAttempt(ctx, AttemptRow{ID: "c", JobID: "j", N: 3}, "third-digest"); !codeIs(err, "E609") {
		t.Fatalf("old approval reused: %v", err)
	}
}

func TestOpenModesAndSkew(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dir", "state.db")
	s, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Mode().Perm() != 0600 {
		t.Fatalf("file mode %v %v", st, err)
	}
	st, err = os.Stat(filepath.Dir(path))
	if err != nil || st.Mode().Perm() != 0700 {
		t.Fatalf("dir mode %v %v", st, err)
	}
	_, err = s.writer.Exec("UPDATE schema_meta SET min_reader_version=2")
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	_, err = Open(path, Options{ReaderVersion: 1})
	if !codeIs(err, "E606") {
		t.Fatalf("version skew: %v", err)
	}
}

func TestHostReservationAcrossStores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	a, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Close() }()
	b, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = b.Close() }()
	seed(t, a, "a")
	seed(t, a, "b")
	stores := []*Store{a, b}
	var wg sync.WaitGroup
	results := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := []string{"a", "b"}[i]
			results[i] = stores[i].AdmitAndLease(context.Background(), "host", Demand{AttemptID: id, RunnerID: "runner", LeaseID: "lease" + id, TokenDigest: "digest", CPU: 1, MemMB: 1, CapacityCPU: 1, CapacityMemMB: 1, Epoch: 0})
		}(i)
	}
	wg.Wait()
	accepted := 0
	for _, err := range results {
		if err == nil {
			accepted++
		} else if !codeIs(err, "E604") {
			t.Fatalf("unexpected admission error: %v", err)
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted %d reservations", accepted)
	}
	var n int
	if err := a.readers.QueryRow("SELECT count(*) FROM host_reservation WHERE host='host'").Scan(&n); err != nil || n != 1 {
		t.Fatalf("reservations %d: %v", n, err)
	}
}

func TestAuditSealerOrder(t *testing.T) {
	s := testStore(t)
	var previous []byte
	calls := 0
	s.SetAuditSealer(func(prev []byte, row AuditRow) (AuditChain, error) {
		if !bytes.Equal(prev, previous) {
			t.Fatalf("wrong previous head")
		}
		calls++
		previous = []byte{byte(calls)}
		return AuditChain{Head: previous, Signature: []byte{byte(row.Seq)}}, nil
	})
	ctx := context.Background()
	if err := s.AppendAudit(ctx, "operator", "one", "a", "decision"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendAudit(ctx, "operator", "two", "b", "decision"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("sealer calls %d", calls)
	}
}
