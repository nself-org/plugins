package main

// Purpose: tests of `nself ci serve admin audit checkpoint export|verify` (P7-TRUST-20): export writes a
// signed checkpoint and prints the key fingerprint; verify needs --pubkey; a chain rewritten with every hash
// recomputed passes `audit --verify` but fails the checkpoint; PERSONAL mode refuses export.
// Inputs:  a real SQLite store in t.TempDir() with an injected clock, the commands' own run functions.
// Outputs: pass or fail; no network.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

type checkpointRig struct {
	*adminRig
	dir, file string
}

func newCheckpointRig(t *testing.T) *checkpointRig {
	t.Helper()
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "team")
	r := &checkpointRig{adminRig: newAdminRig(t), dir: filepath.Join(t.TempDir(), "data")}
	r.file = filepath.Join(t.TempDir(), "audit.checkpoint")
	old := checkpointDir
	checkpointDir = func() string { return r.dir }
	t.Cleanup(func() { checkpointDir = old })
	svc, _, closeFn, err := r.env().Open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer closeFn()
	for _, a := range []string{"one", "two", "three", "four"} {
		if err = svc.Log.Append(context.Background(), audit.Event{Actor: "cli:alice", Action: "test." + a, Target: "t-" + a, Detail: "detail " + a}); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func (r *checkpointRig) export() string {
	r.t.Helper()
	if code := runAdminCheckpoint([]string{"export", r.file, "--json"}, r.env()); code != 0 {
		r.t.Fatalf("export exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	fp, _ := r.data()["fingerprint"].(string)
	if len(fp) != 64 {
		r.t.Fatalf("export printed no fingerprint: %v", r.data())
	}
	return fp
}

func TestCheckpointExportVerify(t *testing.T) {
	r := newCheckpointRig(t)
	fp := r.export()
	info, err := os.Stat(filepath.Join(r.dir, "audit-checkpoint.key"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("checkpoint key: %v %v", err, info)
	}
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", fp, r.file, "--json"}, r.env()); code != 0 {
		t.Fatalf("verify exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	if r.data()["ok"] != true {
		t.Fatalf("verify data: %v", r.data())
	}
	// rows written after the checkpoint do not break it
	svc, _, closeFn, _ := r.env().Open(context.Background())
	_ = svc.Log.Append(context.Background(), audit.Event{Actor: "cli:alice", Action: "test.later", Target: "t", Detail: "d"})
	closeFn()
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", "sha256:" + fp, r.file}, r.env()); code != 0 {
		t.Fatalf("verify after later rows exited %d: %s", code, r.errb.String())
	}
	// a second export reuses the key and prints the same fingerprint
	if again := r.export(); again != fp {
		t.Fatalf("second export changed the key: %s != %s", again, fp)
	}
	// the wrong pinned key, and no pinned key at all, are refusals
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", strings.Repeat("0", 64), r.file}, r.env()); code != 1 {
		t.Fatalf("verify with the wrong fingerprint exited %d", code)
	}
	if code := runAdminCheckpoint([]string{"verify", r.file}, r.env()); code != 1 || !strings.Contains(r.errb.String(), "--pubkey") {
		t.Fatalf("verify without --pubkey exited %d: %s", code, r.errb.String())
	}
}

func TestCheckpointPersonalModeRefusesExport(t *testing.T) {
	r := newCheckpointRig(t)
	t.Setenv("NSELF_CI_COORDINATOR_MODE", "")
	if code := runAdminCheckpoint([]string{"export", r.file}, r.env()); code != 1 || !strings.Contains(r.errb.String(), "PERSONAL") {
		t.Fatalf("export in PERSONAL mode exited %d: %s", code, r.errb.String())
	}
	if _, err := os.Stat(r.file); err == nil {
		t.Fatal("a checkpoint file was written in PERSONAL mode")
	}
	if _, err := os.Stat(filepath.Join(r.dir, "audit-checkpoint.key")); err == nil {
		t.Fatal("a checkpoint key was created in PERSONAL mode")
	}
}

// rewriteChain edits row seq and recomputes the hash of every sealed row from it on, the way a writer of
// the database would.
func rewriteChain(t *testing.T, path string, seq int64, detail string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`UPDATE audit SET detail=? WHERE seq=?`, detail, seq); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Query(`SELECT seq,ts,actor,action,target,detail,prev_head FROM audit WHERE seq>=? AND head IS NOT NULL ORDER BY seq`, seq)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		store.AuditRow
		prev []byte
	}
	var todo []row
	for rows.Next() {
		var x row
		if err = rows.Scan(&x.Seq, &x.TS, &x.Actor, &x.Action, &x.Target, &x.Detail, &x.prev); err != nil {
			t.Fatal(err)
		}
		todo = append(todo, x)
	}
	_ = rows.Close()
	var prev []byte
	if err = db.QueryRow(`SELECT head FROM audit WHERE seq<? AND head IS NOT NULL ORDER BY seq DESC LIMIT 1`, seq).Scan(&prev); err != nil && err != sql.ErrNoRows {
		t.Fatal(err)
	}
	for _, x := range todo {
		head, herr := audit.Hash(prev, x.AuditRow)
		if herr != nil {
			t.Fatal(herr)
		}
		if _, err = db.Exec(`UPDATE audit SET prev_head=?,head=? WHERE seq=?`, prev, head, x.Seq); err != nil {
			t.Fatal(err)
		}
		prev = head
	}
}

func TestCheckpointRewriteFails(t *testing.T) {
	r := newCheckpointRig(t)
	fp := r.export()
	rewriteChain(t, r.path, 2, "detail rewritten by a database writer")
	// the chain is internally consistent again: the hash chain alone cannot see the edit
	if code := runAdminAudit([]string{"--verify"}, r.env()); code != 0 {
		t.Fatalf("audit --verify of the rewritten chain exited %d: %s (the test must rewrite consistently)", code, r.errb.String())
	}
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", fp, r.file}, r.env()); code != 1 || !strings.Contains(r.errb.String(), "rewritten") {
		t.Fatalf("checkpoint verify of a rewritten chain exited %d: %s", code, r.errb.String())
	}
	// control: the same checkpoint verified before the rewrite is the one that fails now (fresh rig, no edit)
	c := newCheckpointRig(t)
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", c.export(), c.file}, c.env()); code != 0 {
		t.Fatalf("control exited %d: %s", code, c.errb.String())
	}
}

func TestCheckpointRemovedRowsFail(t *testing.T) {
	r := newCheckpointRig(t)
	fp := r.export()
	db, err := sql.Open("sqlite", r.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`DELETE FROM audit WHERE seq>=3`); err != nil {
		t.Fatal(err)
	}
	if code := runAdminCheckpoint([]string{"verify", "--pubkey", fp, r.file}, r.env()); code != 1 {
		t.Fatalf("verify after rows were removed exited %d: %s", code, r.errb.String())
	}
}

func TestCheckpointQuarantineSeamReadsTheStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	ctx := context.Background()
	ev := model.Evidence{RunID: "run-q"}
	if q, err := storeQuarantined(ctx, ev); err != nil || q {
		t.Fatalf("no store: %v %v", q, err)
	}
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	led := store.AttestLedger{S: st}
	if q, err := storeQuarantined(ctx, ev); err != nil || q {
		t.Fatalf("empty store: %v %v", q, err)
	}
	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO attest_quarantine(run_id,key_id,quarantined_at) VALUES('run-q','ci-node-0123456789abcdef',1)`); err != nil {
		t.Fatal(err)
	}
	if q, err := storeQuarantined(ctx, ev); err != nil || !q {
		t.Fatalf("quarantined run: %v %v", q, err)
	}
	if q, _ := led.Quarantined(ctx, "run-other"); q {
		t.Fatal("another run quarantined")
	}
}
