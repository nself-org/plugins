package backup

// Purpose: the review-round-3 guarantees of restore (P7-NODE-20): a restore that fails after the swap on an
//          empty dir keeps its marker, a re-run still marks every node needs-reconfirm, serve and backup
//          refuse meanwhile, and token and grant revocations of the replaced dir survive a restore of an
//          older backup.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestFailedRestoreOnAnEmptyDirKeepsItsMarkerAndReconfirmsOnRetry(t *testing.T) {
	ctx := context.Background()
	r, arch := seeded(t)
	fresh := t.TempDir()
	if err := os.WriteFile(filepath.Join(fresh, "pki"), []byte("not a directory"), 0o600); err != nil { // the KEK install must fail
		t.Fatal(err)
	}
	o := r.opts()
	o.DataDir = fresh
	if _, err := Restore(ctx, o, bytes.NewReader(arch), r.kek); err == nil {
		t.Fatal("the first restore was meant to fail after the swap")
	}
	if !Pending(fresh) {
		t.Fatal("a failed restore must leave restore.pending")
	}
	if _, err := Create(ctx, o, &bytes.Buffer{}); ReasonOf(err) != ReasonIncomplete {
		t.Fatalf("a backup of a half-restored dir = %v, want incomplete_restore", err)
	}
	if err := os.Remove(filepath.Join(fresh, "pki")); err != nil {
		t.Fatal(err)
	}
	res, err := Restore(ctx, o, bytes.NewReader(arch), r.kek)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if !res.Empty || res.NodesReconfirm == 0 {
		t.Fatalf("retry result %+v: the dir was empty before the first run, every node must be needs-reconfirm", res)
	}
	if Pending(fresh) {
		t.Fatal("restore.pending must be gone after a restore finished")
	}
}

func TestRestoreRefusedWhileRunningLeavesNoMarker(t *testing.T) {
	ctx := context.Background()
	r, arch := seeded(t)
	if _, err := r.st.Register(ctx, os.Getpid(), "s", "b"); err != nil {
		t.Fatal(err)
	}
	_ = r.st.Close()
	o := r.opts()
	o.Alive = func(int) bool { return true }
	if _, err := Restore(ctx, o, bytes.NewReader(arch), r.kek); ReasonOf(err) != ReasonRunningCoordinator {
		t.Fatalf("restore over a running coordinator = %v", err)
	}
	if Pending(r.dir) {
		t.Fatal("a refused restore changed nothing and must leave no marker")
	}
}

func TestRestoreKeepsTokenAndGrantRevocations(t *testing.T) {
	ctx := context.Background()
	r, _ := seeded(t)
	put := func(id, who string) {
		t.Helper()
		if err := r.st.GrantPut(ctx, store.RoleGrant{Principal: who, Role: "admin", Subject: who, GrantedBy: "cli:test"}, store.AuditInput{Actor: "cli:test", Action: "t", Target: "x"}); err != nil {
			t.Fatal(err)
		}
		tok := store.Token{ID: id, Digest: "d-" + id, Principal: who, Role: "admin", Subject: who, IssuedBy: "cli:test", ExpiresAt: r.clk.now().Add(time.Hour).UnixNano()}
		if err := r.st.TokenPut(ctx, tok, store.AuditInput{Actor: "cli:test", Action: "t", Target: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	put("t-alice", "alice")
	put("t-bob", "bob")
	arch := r.archive() // taken while both are live
	if _, err := r.st.GrantRevoke(ctx, "alice", store.AuditInput{Actor: "cli:test", Action: "revoke", Target: "x"}); err != nil {
		t.Fatal(err)
	}
	if found, err := r.st.TokenRevoke(ctx, "t-bob", store.AuditInput{Actor: "cli:test", Action: "revoke", Target: "x"}); err != nil || !found {
		t.Fatalf("revoke bob's token: %v %v", found, err)
	}
	_ = r.st.Close()
	res := restoreInto(t, r, r.dir, arch)
	if res.TokensRevoked != 2 || res.GrantsRevoked != 1 {
		t.Fatalf("result %+v, want two tokens (bob's, and alice's ended by her grant) and one grant revoked again", res)
	}
	st2, _ := r.reopen(r.dir)
	if tok, ok, _ := st2.TokenByID(ctx, "t-bob"); !ok || tok.RevokedAt == 0 {
		t.Fatalf("bob's revoked token came back: %+v", tok)
	}
	if tok, ok, _ := st2.TokenByID(ctx, "t-alice"); !ok || tok.RevokedAt == 0 {
		t.Fatalf("alice's token survived her revoked grant: %+v", tok)
	}
	if _, live, _ := st2.GrantGet(ctx, "alice"); live {
		t.Fatal("alice's revoked grant came back")
	}
	if _, live, _ := st2.GrantGet(ctx, "bob"); !live {
		t.Fatal("bob's grant, still live in the replaced dir, was wrongly revoked")
	}
}

// TestRerunAfterAPostSwapFailureKeepsTheFirstRunsRevocations: the second run finds a half-restored dir; the
// revocations it merges are the ones the FIRST run found in the dir it replaced (kept in restore.pending).
func TestRerunAfterAPostSwapFailureKeepsTheFirstRunsRevocations(t *testing.T) {
	ctx := context.Background()
	r, _ := seeded(t)
	tok := store.Token{ID: "t-bob", Digest: "d-bob", Principal: "bob", Role: "admin", Subject: "bob", IssuedBy: "cli:test", ExpiresAt: r.clk.now().Add(time.Hour).UnixNano()}
	if err := r.st.TokenPut(ctx, tok, store.AuditInput{Actor: "cli:test", Action: "t", Target: "x"}); err != nil {
		t.Fatal(err)
	}
	arch := r.archive() // bob's token is live in the backup
	if found, err := r.st.TokenRevoke(ctx, "t-bob", store.AuditInput{Actor: "cli:test", Action: "revoke", Target: "x"}); err != nil || !found {
		t.Fatalf("revoke: %v %v", found, err)
	}
	_ = r.st.Close()
	pkiDir := filepath.Join(r.dir, "pki")
	if err := os.Chmod(pkiDir, 0o500); err != nil { // the KEK install after the swap must fail
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(pkiDir, 0o700) }()
	o := r.opts()
	if _, err := Restore(ctx, o, bytes.NewReader(arch), r.kek); err == nil {
		t.Skip("the platform let the restore write into a read-only directory")
	}
	if !Pending(r.dir) {
		t.Fatal("no marker after a post-swap failure")
	}
	if err := os.Chmod(pkiDir, 0o700); err != nil {
		t.Fatal(err)
	}
	r.clk.t = r.clk.t.Add(time.Minute)
	res, err := Restore(ctx, o, bytes.NewReader(arch), r.kek)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if res.TokensRevoked < 1 {
		t.Fatalf("retry result %+v: the first run's token revocation was lost", res)
	}
	st2, _ := r.reopen(r.dir)
	if tk, ok, _ := st2.TokenByID(ctx, "t-bob"); !ok || tk.RevokedAt == 0 {
		t.Fatalf("bob's token is live again after the retry: %+v", tk)
	}
}

func TestRollbackLineRemovesTheMarker(t *testing.T) {
	line := rollbackLine("/data", map[string]string{"/data/state.db": "/data/state.db.pre-restore-x"})
	if !strings.Contains(line, "restore.pending") {
		t.Fatalf("rollback line %q must remove the restore marker", line)
	}
}

func TestRestoreDoesNotRevokeGrantsOfAFreshDir(t *testing.T) {
	ctx := context.Background()
	r, _ := seeded(t)
	if err := r.st.GrantPut(ctx, store.RoleGrant{Principal: "alice", Role: "admin", Subject: "alice", GrantedBy: "cli:test"}, store.AuditInput{Actor: "cli:test", Action: "t", Target: "x"}); err != nil {
		t.Fatal(err)
	}
	arch := r.archive()
	// a second machine that already has a CA and nodes (serve ran once) but never held alice
	other := newRig(t)
	other.addNode("node-z")
	_ = other.st.Close()
	o := r.opts()
	o.DataDir = other.dir
	res, err := Restore(ctx, o, bytes.NewReader(arch), r.kek)
	if err != nil {
		t.Fatal(err)
	}
	if res.GrantsRevoked != 0 {
		t.Fatalf("result %+v: a grant the replaced dir never held is not evidence of a revocation", res)
	}
	st2, _ := other.reopen(other.dir)
	if _, live, _ := st2.GrantGet(ctx, "alice"); !live {
		t.Fatal("alice's grant was revoked by a restore onto a dir that never held it")
	}
}

func TestMarkerClearedWhenNothingChanged(t *testing.T) {
	ctx := context.Background()
	r, arch := seeded(t)
	_ = r.st.Close()
	if err := os.WriteFile(filepath.Join(r.dir, "state.db"), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, r.opts(), bytes.NewReader(arch), r.kek); err == nil {
		t.Fatal("a restore over an unreadable state.db was meant to fail")
	}
	if Pending(r.dir) {
		t.Fatal("a restore that failed before the swap must leave no marker")
	}
}
