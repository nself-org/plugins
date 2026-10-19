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
		if err := r.st.GrantPut(ctx, store.Grant{Principal: who, Role: "admin", Subject: who, GrantedBy: "cli:test"}, store.AuditInput{Actor: "cli:test", Action: "t", Target: "x"}); err != nil {
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
