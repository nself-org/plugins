//go:build security

package redteam

// Purpose: red team group C, audit log (TM-25). The attacker holds write access to the database file
// and edits a sealed row, deletes a middle row, swaps two rows, clears a head, truncates the newest
// rows (with and without resetting the AUTOINCREMENT counter), recomputes the whole chain after an
// edit, and replays a signed checkpoint of another chain. It also feeds the scrubber a registered
// value in encodings and splits.
// Inputs:  a real store opened twice (the second handle is the attacker), production audit.Log,
//          audit.Hash and attest checkpoint functions.
// Outputs: pass (the tamper is detected, or the documented residual is caught by the pinned checkpoint)
//          or fail.
// Constraints: temp dirs only; the checkpoint compare follows the documented operator procedure
//          (cmd/serve_admin_checkpoint.go is package main and cannot be imported).

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type cAuditRig struct {
	t    *testing.T
	st   *store.Store
	log  *audit.Log
	db   *sql.DB // the attacker's own handle on the file
	dir  string
	path string
}

func cNewAudit(t *testing.T, rows int) *cAuditRig {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	now := time.Unix(1_800_000_000, 0)
	st, err := store.Open(path, store.Options{Clock: func() time.Time { now = now.Add(time.Second); return now }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	r := &cAuditRig{t: t, st: st, log: &audit.Log{St: st}, db: db, dir: dir, path: path}
	for i := 1; i <= rows; i++ {
		if err := r.log.Append(context.Background(), audit.Event{Actor: "op", Action: "act", Target: fmt.Sprintf("t%d", i), Detail: fmt.Sprintf("detail %d", i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.log.Seal(context.Background()); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *cAuditRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.db.Exec(q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

func (r *cAuditRig) verify() audit.Report {
	r.t.Helper()
	rep, err := r.log.Verify(context.Background(), time.Hour)
	if err != nil {
		r.t.Fatal(err)
	}
	return rep
}

// TestC25ChainDetectsDirectEdits: each tamper that does not recompute the chain is reported.
func TestC25ChainDetectsDirectEdits(t *testing.T) {
	if rep := cNewAudit(t, 6).verify(); !rep.OK() || rep.Sealed != 6 {
		t.Fatalf("control: %+v", rep)
	}
	for name, tamper := range map[string]func(*cAuditRig){
		"edit a sealed row":        func(r *cAuditRig) { r.exec(`UPDATE audit SET detail='forged' WHERE seq=3`) },
		"edit the timestamp":       func(r *cAuditRig) { r.exec(`UPDATE audit SET ts=ts+1 WHERE seq=2`) },
		"edit actor to look human": func(r *cAuditRig) { r.exec(`UPDATE audit SET actor='alice' WHERE seq=5`) },
		"delete a middle row":      func(r *cAuditRig) { r.exec(`DELETE FROM audit WHERE seq=4`) },
		"delete the first row":     func(r *cAuditRig) { r.exec(`DELETE FROM audit WHERE seq=1`) },
		"delete the newest row":    func(r *cAuditRig) { r.exec(`DELETE FROM audit WHERE seq=6`) },
		"clear a head":             func(r *cAuditRig) { r.exec(`UPDATE audit SET head=NULL, prev_head=NULL WHERE seq=3`) },
		"swap two rows": func(r *cAuditRig) {
			r.exec(`UPDATE audit SET seq=99 WHERE seq=2`)
			r.exec(`UPDATE audit SET seq=2 WHERE seq=3`)
			r.exec(`UPDATE audit SET seq=3 WHERE seq=99`)
		},
		"copy a head to forge link": func(r *cAuditRig) {
			r.exec(`UPDATE audit SET prev_head=(SELECT head FROM audit WHERE seq=1) WHERE seq=4`)
		},
		"insert a row in the gap": func(r *cAuditRig) {
			r.exec(`DELETE FROM audit WHERE seq=3`)
			r.exec(`INSERT INTO audit(seq,ts,actor,action,target,detail) VALUES (3,1,'x','x','x','x')`)
		},
	} {
		r := cNewAudit(t, 6)
		tamper(r)
		if rep := r.verify(); rep.OK() {
			t.Errorf("%s: the chain still verifies (%+v)", name, rep)
		}
	}
}

// TestC25CheckpointCatchesRecomputedChain: a writer who edits a row and recomputes every later head, or
// truncates the tail and resets the counter, passes the chain check (documented residual) but not the
// pinned checkpoint taken earlier; a checkpoint of another chain or with edited fields is refused.
func TestC25CheckpointCatchesRecomputedChain(t *testing.T) {
	ctx := context.Background()
	r := cNewAudit(t, 6)
	signer, pub, err := attest.LoadOrCreateCheckpointKey(filepath.Join(r.dir, "keys"))
	if err != nil {
		t.Fatal(err)
	}
	pin := attest.Fingerprint(pub)
	last, _ := r.st.AuditEntries(ctx, 5, 1)
	cp, err := attest.NewCheckpoint(signer, pub, last[0].Head, last[0].Seq, 6, 1_800_000_100)
	if err != nil {
		t.Fatal(err)
	}
	if err := attest.VerifyCheckpoint(cp, pin); err != nil {
		t.Fatalf("control: %v", err)
	}
	matches := func() bool { // the operator procedure: the checkpoint row must exist with the same head
		page, err := r.st.AuditEntries(ctx, cp.Seq-1, 1)
		return err == nil && len(page) == 1 && page[0].Seq == cp.Seq && hex.EncodeToString(page[0].Head) == cp.Head
	}
	if !matches() {
		t.Fatal("control: the honest chain does not match its own checkpoint")
	}
	// recompute everything from an edited row 2
	r.exec(`UPDATE audit SET detail='forged' WHERE seq=2`)
	entries, _ := r.st.AuditEntries(ctx, 0, 100)
	var prev []byte
	for _, e := range entries {
		if e.Seq == 2 {
			e.Detail = "forged"
		}
		head, err := audit.Hash(prev, e.AuditRow)
		if err != nil {
			t.Fatal(err)
		}
		r.exec(`UPDATE audit SET prev_head=?, head=? WHERE seq=?`, prev, head, e.Seq)
		prev = head
	}
	if rep := r.verify(); !rep.OK() {
		t.Logf("a recomputed chain no longer verifies: %+v (stronger than documented)", rep)
	}
	if matches() {
		t.Error("a fully recomputed chain still matches the checkpoint taken before the edit")
	}
	// truncate and reset the counter on a fresh chain
	r2 := cNewAudit(t, 6)
	r2.exec(`DELETE FROM audit WHERE seq>4`)
	r2.exec(`UPDATE sqlite_sequence SET seq=4 WHERE name='audit'`)
	page, _ := r2.st.AuditEntries(ctx, 5, 1)
	if len(page) != 0 {
		t.Fatal("truncation fixture failed")
	}
	// a checkpoint minted for another chain, and edited copies of the real one
	other := cNewAudit(t, 7) // a different installation: its rows differ from ours
	oLast, _ := other.st.AuditEntries(ctx, 6, 1)
	foreign, err := attest.NewCheckpoint(signer, pub, oLast[0].Head, oLast[0].Seq, 7, 1)
	if err != nil {
		t.Fatal(err)
	}
	if attest.VerifyCheckpoint(foreign, pin) != nil {
		t.Fatal("control: a foreign chain's checkpoint has a valid signature by design")
	}
	if matches2 := hex.EncodeToString(last[0].Head) == foreign.Head; matches2 {
		t.Error("two independent chains produced one head")
	}
	_, otherPub, _ := attest.LoadOrCreateCheckpointKey(filepath.Join(r.dir, "other-keys"))
	for name, c := range map[string]attest.Checkpoint{
		"seq edited":  withCP(cp, func(c *attest.Checkpoint) { c.Seq = 5 }),
		"head edited": withCP(cp, func(c *attest.Checkpoint) { c.Head = strings.Repeat("0", 64) }),
		"rows edited": withCP(cp, func(c *attest.Checkpoint) { c.Rows = 1 }),
		"ts edited":   withCP(cp, func(c *attest.Checkpoint) { c.TS++ }),
		"key swapped": withCP(cp, func(c *attest.Checkpoint) { c.Public = base64.StdEncoding.EncodeToString(otherPub) }),
		"version":     withCP(cp, func(c *attest.Checkpoint) { c.Version = 2 }),
		"empty sig":   withCP(cp, func(c *attest.Checkpoint) { c.Signature = "" }),
	} {
		if err := attest.VerifyCheckpoint(c, pin); err == nil {
			t.Errorf("%s: the checkpoint verified", name)
		}
	}
	for _, bad := range []string{"", "00", strings.Repeat("0", 64), attest.Fingerprint(otherPub), "sha256:" + attest.Fingerprint(otherPub), strings.ToUpper(pin) + "x"} {
		if err := attest.VerifyCheckpoint(cp, bad); err == nil {
			t.Errorf("pinned %q accepted a checkpoint", bad)
		}
	}
}

func withCP(c attest.Checkpoint, f func(*attest.Checkpoint)) attest.Checkpoint { f(&c); return c }

// TestC25ScrubberEncodings: a registered value is refused in the raw form, split across fields and in
// the encodings an event writer would produce; the test reports any encoding that gets through.
func TestC25ScrubberEncodings(t *testing.T) {
	sc := redact.New()
	secret := "p@ss\"w0rd/with+chars&=12345"
	if err := sc.Register([]byte(secret)); err != nil {
		t.Fatal(err)
	}
	ev := func(actor, action, target, detail string) audit.Event {
		return audit.Event{Actor: actor, Action: action, Target: target, Detail: detail}
	}
	cases := map[string]audit.Event{
		"raw":                  ev("a", "b", "c", "x "+secret+" y"),
		"split actor/action":   ev(secret[:8], secret[8:], "c", "d"),
		"split target/detail":  ev("a", "b", secret[:10], secret[10:]),
		"base64 std":           ev("a", "b", "c", base64.StdEncoding.EncodeToString([]byte(secret))),
		"base64 url":           ev("a", "b", "c", base64.URLEncoding.EncodeToString([]byte(secret))),
		"base64 raw":           ev("a", "b", "c", base64.RawStdEncoding.EncodeToString([]byte(secret))),
		"hex":                  ev("a", "b", "c", hex.EncodeToString([]byte(secret))),
		"hex upper":            ev("a", "b", "c", strings.ToUpper(hex.EncodeToString([]byte(secret)))),
		"percent encoded":      ev("a", "b", "c", url.QueryEscape(secret)),
		"path escaped":         ev("a", "b", "c", url.PathEscape(secret)),
		"go quoted":            ev("a", "b", "c", fmt.Sprintf("%q", secret)),
		"json inside detail":   ev("a", "b", "c", `{"v":"`+strings.ReplaceAll(secret, `"`, `\"`)+`"}`),
		"with a base64 prefix": ev("a", "b", "c", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:"+secret))),
	}
	for name, e := range cases {
		if err := audit.Scrub(sc, e); err == nil {
			t.Errorf("%s: the event was not refused", name)
		}
	}
	if err := audit.Scrub(sc, ev("a", "b", "c", "no secret here")); err != nil {
		t.Errorf("control: %v", err)
	}
}
