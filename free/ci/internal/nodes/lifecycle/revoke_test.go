package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// seedLease inserts a running attempt with a live lease of the node, straight into the tables.
func seedLease(t *testing.T, dbPath, node string, ttl time.Duration) string {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err = db.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	if _, err = db.Exec("INSERT INTO attempt(id,job_id,n,state,epoch) VALUES('att-1','job-1',1,'running',1)"); err != nil {
		t.Skipf("attempt table shape changed: %v", err)
	}
	if _, err = db.Exec("INSERT INTO lease(id,attempt_id,host,runner_id,expires_at,heartbeat_at,token_digest,epoch) VALUES('lease-1','att-1','h',?,?,?,'d',1)", node, now+int64(ttl), now); err != nil {
		t.Skipf("lease table shape changed: %v", err)
	}
	return "lease-1"
}

func leaseExpiry(t *testing.T, dbPath, id string) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var e int64
	if err = db.QueryRow("SELECT expires_at FROM lease WHERE id=?", id).Scan(&e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestRevokeStateLeasesAndAudit(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rv1")
	lease := seedLease(t, k.dbPath, "rv1", time.Hour)
	before := leaseExpiry(t, k.dbPath, lease)
	rep, err := k.d.Revoke(ctx, "rv1")
	if err != nil {
		t.Fatal(err)
	}
	if rep.LeasesExpired != 1 || !rep.PinRemoved || len(rep.Remaining) != 0 || !strings.Contains(rep.Note, "authorized_keys") {
		t.Fatalf("report = %+v", rep)
	}
	if after := leaseExpiry(t, k.dbPath, lease); after >= before || after > time.Now().UnixNano() {
		t.Fatalf("lease not expired: before=%d after=%d", before, after)
	}
	n, err := k.d.Registry.Get(ctx, "rv1")
	if err != nil || n.Record.State != "revoked" || n.Capability.Lifecycle.RevokedAt == nil {
		t.Fatalf("node = %+v err=%v", n.Record, err)
	}
	if p, _ := k.d.pins().Lookup(pinAlias("rv1")); len(p) != 0 {
		t.Fatal("pin survived revoke")
	}
	if k.auditCount("node.revoke") != 1 {
		t.Fatalf("revoke audit rows = %d", k.auditCount("node.revoke"))
	}
	errCode(t, Gate(ctx, k.d.Registry, "rv1", "proj-1"), CodeRevoked)
	for name, fn := range map[string]func() error{
		"drain":  func() error { _, e := k.d.Drain(ctx, "rv1"); return e },
		"enable": func() error { _, e := k.d.Enable(ctx, "rv1"); return e },
		"test":   func() error { _, e := k.d.Test(ctx, "rv1", nil); return e },
	} {
		if e := fn(); CodeOf(e) != CodeRevoked {
			t.Errorf("%s of a revoked node: %v", name, e)
		}
	}
}

func TestRevokeFailingRevokerRollsBackEverything(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rv2")
	lease := seedLease(t, k.dbPath, "rv2", time.Hour)
	before := leaseExpiry(t, k.dbPath, lease)
	t.Cleanup(resetExtensions)
	ran := 0
	RegisterRevoker(func(context.Context, *store.RevokeTx, string) error { ran++; return nil })
	RegisterRevoker(func(context.Context, *store.RevokeTx, string) error { return errBoom })
	_, err := k.d.Revoke(ctx, "rv2")
	if err == nil || !errors.Is(err, errBoom) || ran != 1 {
		t.Fatalf("err=%v ran=%d", err, ran)
	}
	n, gerr := k.d.Registry.Get(ctx, "rv2")
	if gerr != nil || n.Record.State == "revoked" || n.Capability.Lifecycle.RevokedAt != nil {
		t.Fatalf("state changed despite the rollback: %+v", n.Record)
	}
	if leaseExpiry(t, k.dbPath, lease) != before {
		t.Fatal("lease expired despite the rollback")
	}
	if k.auditCount("node.revoke") != 0 {
		t.Fatal("audit row survived the rollback")
	}
	if p, _ := k.d.pins().Lookup(pinAlias("rv2")); len(p) != 1 {
		t.Fatal("pin removed although the revoke failed")
	}
	if Gate(ctx, k.d.Registry, "rv2", "proj-1") != nil {
		t.Fatal("rolled-back node is no longer leasable")
	}
}

func TestRevokerWritesInTheSameTransaction(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rv3")
	t.Cleanup(resetExtensions)
	RegisterRevoker(func(ctx context.Context, tx *store.RevokeTx, id string) error {
		return tx.AppendAudit("cli:test", "deny.add", id, "deny list")
	})
	if _, err := k.d.Revoke(ctx, "rv3"); err != nil {
		t.Fatal(err)
	}
	if k.auditCount("deny.add") != 1 || k.auditCount("node.revoke") != 1 {
		t.Fatal("hook audit row or revoke row missing")
	}
}

func TestSessionCloserFailureIsReportedNotHidden(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rv4")
	t.Cleanup(resetExtensions)
	closed := 0
	RegisterSessionCloser(func(context.Context, string) error { closed++; return nil })
	RegisterSessionCloser(func(context.Context, string) error { return errBoom })
	rep, err := k.d.Revoke(ctx, "rv4")
	if CodeOf(err) != CodeRevoked || err.(*Error).Reason != ReasonPartial {
		t.Fatalf("err = %v", err)
	}
	if rep.SessionsShut != 1 || closed != 1 || len(rep.Remaining) != 1 || !strings.Contains(rep.Remaining[0], "session closer 2") {
		t.Fatalf("report = %+v", rep)
	}
	n, _ := k.d.Registry.Get(ctx, "rv4")
	if n.Record.State != "revoked" {
		t.Fatal("a failing closer must not undo the committed revoke")
	}
}

func TestRemoveDeletesRecordKeepsAudit(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rm1")
	rep, err := k.d.Remove(ctx, "rm1")
	if err != nil || !rep.Removed || !rep.PinRemoved {
		t.Fatalf("rep=%+v err=%v", rep, err)
	}
	if _, err = k.d.Registry.Get(ctx, "rm1"); CodeOf(err) != CodeNotFound {
		t.Fatalf("node still there: %v", err)
	}
	if k.auditCount("node.add") != 1 || k.auditCount("node.remove") != 1 {
		t.Fatalf("audit rows lost: add=%d remove=%d", k.auditCount("node.add"), k.auditCount("node.remove"))
	}
	if _, err = k.d.Revoke(ctx, "rm1"); CodeOf(err) != CodeNotFound {
		t.Fatalf("revoke of a removed node: %v", err)
	}
}

func TestNodeLifecycleStoreNeedsAnActor(t *testing.T) {
	k := newKit(t)
	k.addNode("rv5")
	for _, actor := range []string{"", "agent", "probe"} {
		if _, err := k.st.NodeRevoke(context.Background(), "rv5", actor, nil); err == nil && actor == "" {
			t.Errorf("revoke with actor %q accepted", actor)
		}
	}
	if _, err := k.st.NodeRevoke(context.Background(), "nope", "cli:test", nil); CodeOf(err) != CodeNotFound {
		t.Fatalf("unknown node: %v", err)
	}
}

func TestGateAuthorization(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("gt1")
	if err := Gate(ctx, k.d.Registry, "gt1", "proj-1"); err != nil {
		t.Fatalf("authorized project refused: %v", err)
	}
	for _, p := range []string{"proj-2", ""} {
		err := Gate(ctx, k.d.Registry, "gt1", p)
		errCode(t, err, CodeRevoked)
		if err.(*Error).Reason != ReasonNotAuthorized {
			t.Errorf("project %q reason = %s", p, err.(*Error).Reason)
		}
	}
	errCode(t, Gate(ctx, k.d.Registry, "ghost", "proj-1"), CodeNotFound)
}

// D-1212: revoke moves the attempt epoch of the node's running attempts, so an epoch-gated call the
// revoked node makes at its old epoch is refused at once, not at the lease sweep.
func TestRevokeFencesTheAttemptEpoch(t *testing.T) {
	k := newKit(t)
	ctx := context.Background()
	k.addNode("rv7")
	seedLease(t, k.dbPath, "rv7", time.Hour)
	if err := k.st.ArtifactDeclare(ctx, "att-1", 1, nil); err != nil {
		t.Fatalf("declare at the live epoch: %v", err)
	}
	if _, err := k.d.Revoke(ctx, "rv7"); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+k.dbPath+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var epoch int64
	if err = db.QueryRow("SELECT epoch FROM attempt WHERE id='att-1'").Scan(&epoch); err != nil || epoch != 2 {
		t.Fatalf("attempt epoch = %d (%v), want 2", epoch, err)
	}
	if err = k.st.ArtifactDeclare(ctx, "att-1", 1, nil); err == nil {
		t.Fatal("the revoked node's declare at its old epoch was accepted")
	}
}
