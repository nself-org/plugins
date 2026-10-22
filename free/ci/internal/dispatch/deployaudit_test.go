package dispatch

// D-1249: a placement on an overridden deploy host (D21) is admitted only with
// its audit row, written in the admission transaction.

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

const deployAuditAction = "placement.deploy_host_override"

func admitOn(t *testing.T, st *store.Store, deploy trust.DeployHost) (string, error) {
	t.Helper()
	id := queueAttempt(t, st, "P", "J", "N")
	r := testRunner("R", "lan", 8)
	r.Decl.DeployHost = deploy
	_, err := Admit(context.Background(), st, Request{AttemptID: id, Placement: sched.Placement{JobKey: id, RunnerID: "R", CPU: 1, MemMB: 1}, Runner: r, Job: testJob(id), Local: true, Host: "h-" + id, CapacityCPU: 4, CapacityMemMB: 4096})
	return id, err
}

func auditActions(t *testing.T, path, action string) []string {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(db.Close)
	rows, err := db.Query("SELECT target FROM audit WHERE action=? ORDER BY seq", action)
	if err != nil {
		t.Fatal(err)
	}
	defer func(cleanup func() error) { _ = cleanup() }(rows.Close)
	var out []string
	for rows.Next() {
		var target string
		if err = rows.Scan(&target); err != nil {
			t.Fatal(err)
		}
		out = append(out, target)
	}
	return out
}

func TestDeployHostPlacementWritesAuditRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	id, err := admitOn(t, st, trust.DeployHost{Matched: true, Override: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := auditActions(t, path, deployAuditAction); len(got) != 1 || got[0] != id {
		t.Fatalf("audit rows %v, want one for %s", got, id)
	}
}

func TestOrdinaryPlacementWritesNoDeployAuditRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	if _, err := admitOn(t, st, trust.DeployHost{}); err != nil {
		t.Fatal(err)
	}
	if got := auditActions(t, path, deployAuditAction); len(got) != 0 {
		t.Fatalf("audit rows %v, want none", got)
	}
}

// TestDeployHostPlacementRefusedWithoutAuditRow: when the row cannot be written
// the placement is refused and the attempt stays queued, with no lease.
func TestDeployHostPlacementRefusedWithoutAuditRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st := openStoreAt(t, path)
	boom := errors.New("audit store down")
	st.SetAuditSealer(func([]byte, store.AuditRow) (store.AuditChain, error) { return store.AuditChain{}, boom })
	id, err := admitOn(t, st, trust.DeployHost{Matched: true, Override: true})
	var ref *Refusal
	if !errors.As(err, &ref) || ref.Reason != model.NodeDeployHost {
		t.Fatalf("err = %v, want a %s refusal", err, model.NodeDeployHost)
	}
	if state, _, serr := st.AttemptStateEpoch(context.Background(), id); serr != nil || state != "queued" {
		t.Fatalf("attempt state %q (%v), want queued", state, serr)
	}
}
