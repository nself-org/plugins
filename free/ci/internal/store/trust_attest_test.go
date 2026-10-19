package store

// Purpose: tests of the evidence-signing ledger (P7-TRUST-20): the migration, the attempt facts a verifier
// compares with, the nodes that pin a key and their signing_eligible fact, the replay set, and Revoke's
// transaction and re-evaluation set over rows seeded the way the coordinator writes them.
// Inputs:  a real SQLite store in a temp dir.
// Outputs: pass or fail; no network.
// Constraints: fixtures use throwaway keys; rows are seeded with SQL because the coordinator paths that
//          write them live in packages that import this one.

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
)

type attestRig struct {
	t   *testing.T
	s   *Store
	led AttestLedger
	ctx context.Context
}

func newAttestRig(t *testing.T) *attestRig {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return &attestRig{t: t, s: s, led: AttestLedger{S: s}, ctx: context.Background()}
}

func (r *attestRig) exec(q string, args ...any) {
	r.t.Helper()
	if _, err := r.s.writer.ExecContext(r.ctx, q, args...); err != nil {
		r.t.Fatalf("%s: %v", q, err)
	}
}

// attempt seeds pipeline p-<id> (revision rev), job j-<id> and attempt <id>.
func (r *attestRig) attempt(id, rev string) {
	r.exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		"p-"+id, "widget", rev, "local", "owner", "any", "pd", "in", "full", "r", "d", "running", 1)
	r.exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES(?,?,?,?,?,?,?,?,?)`, "j-"+id, "p-"+id, "build", "check", 1, 1, "in", 1, "running")
	r.exec(`INSERT INTO attempt(id,job_id,n,state) VALUES(?,?,?,?)`, id, "j-"+id, 1, "queued")
}

func (r *attestRig) pinKey(node string, seed byte, eligible *bool) (keyHex string) {
	pub := ed25519.NewKeyFromSeed([]byte(strings.Repeat(string([]byte{seed}), ed25519.SeedSize))).Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	keyHex = hex.EncodeToString(sum[:])[:16]
	r.exec(`INSERT INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES(?,?,?,?)`, node, keyHex, base64.StdEncoding.EncodeToString(pub), 1)
	if eligible != nil {
		doc := `{"separation":{"signing_eligible":{"value":false}}}`
		if *eligible {
			doc = `{"separation":{"signing_eligible":{"value":true,"source":"assigned"}}}`
		}
		r.exec(`INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,created_at,updated_at) VALUES(?,?,1,'{}','{}','{}','online',1,1)`, node, node)
		r.exec(`INSERT INTO capability_snapshot(node_id,digest,doc_json,observed_at) VALUES(?,?,?,1)`, node, "d", doc)
	}
	return keyHex
}

func ptr(b bool) *bool { return &b }

func TestTrustAttestMigrate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	for pass := 0; pass < 2; pass++ {
		s, err := Open(path, Options{})
		if err != nil {
			t.Fatal(err)
		}
		var tables, logged int
		if err = s.readers.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('attest_use','attest_revoked','attest_quarantine')").Scan(&tables); err != nil {
			t.Fatal(err)
		}
		if err = s.readers.QueryRow("SELECT count(*) FROM migration_log WHERE set_name='trust-attest'").Scan(&logged); err != nil {
			t.Fatal(err)
		}
		if tables != 3 || logged != 1 {
			t.Fatalf("pass %d: %d tables, %d logged", pass, tables, logged)
		}
		_ = s.Close()
	}
}

func TestAttestAttemptFactsReadLeaseThenAdmissionRecord(t *testing.T) {
	r := newAttestRig(t)
	rev := strings.Repeat("a", 40)
	r.attempt("att-live", rev)
	r.attempt("att-done", rev)
	r.attempt("att-none", rev)
	r.exec(`INSERT INTO lease(id,attempt_id,host,runner_id,expires_at,heartbeat_at,token_digest,epoch) VALUES('l1','att-live','h','node-live',9,1,'t',1)`)
	r.exec(`INSERT INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES('att-done','p-att-done','widget','u','t','agent','node-done','w',1)`)
	for id, want := range map[string]string{"att-live": "node-live", "att-done": "node-done"} {
		f, ok, err := r.led.AttemptFacts(r.ctx, id)
		if err != nil || !ok || f.RunnerID != want || f.Revision != rev || f.RunID != "p-"+id || f.Job != "build" || f.SourceTrust != "owner" || f.PolicyDigest != "pd" || f.SelectionMode != "full" {
			t.Fatalf("%s: %+v ok=%v err=%v", id, f, ok, err)
		}
	}
	for _, id := range []string{"att-none", "att-missing"} {
		if _, ok, err := r.led.AttemptFacts(r.ctx, id); ok || err != nil {
			t.Fatalf("%s: ok=%v err=%v", id, ok, err)
		}
	}
}

// TestAttestRepoIDMatchesTheRunDerivation pins the repo id the ledger derives to run.shortDigest's
// output: the first 8 bytes of the sha256 of the project, in hex (store cannot import run).
func TestAttestRepoIDMatchesTheRunDerivation(t *testing.T) {
	r := newAttestRig(t)
	r.attempt("att-1", strings.Repeat("a", 40)) // project "widget"
	r.exec(`INSERT INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES('att-1','p-att-1','widget','u','t','agent','node-1','w',1)`)
	f, ok, err := r.led.AttemptFacts(r.ctx, "att-1")
	sum := sha256.Sum256([]byte("widget"))
	if err != nil || !ok || f.RepoID != hex.EncodeToString(sum[:8]) || len(f.RepoID) != 16 {
		t.Fatalf("%+v %v %v", f, ok, err)
	}
}

func TestAttestNodesByKeyReadsPinAndEligibility(t *testing.T) {
	r := newAttestRig(t)
	a := r.pinKey("node-a", 1, ptr(true))
	b := r.pinKey("node-b", 2, ptr(false))
	c := r.pinKey("node-c", 3, nil) // pinned, no registry record: not eligible
	r.exec(`INSERT INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES('node-pending','0123456789abcdef','',1)`)
	r.exec(`INSERT INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES('node-liar','fedcba9876543210',?,1)`, base64.StdEncoding.EncodeToString(make([]byte, 32)))
	for key, want := range map[string]struct {
		node     string
		eligible bool
	}{a: {"node-a", true}, b: {"node-b", false}, c: {"node-c", false}} {
		got, err := r.led.NodesByKey(r.ctx, key)
		if err != nil || len(got) != 1 || got[0].NodeID != want.node || got[0].SigningEligible != want.eligible || len(got[0].Public) != 32 {
			t.Fatalf("%s: %+v %v", key, got, err)
		}
	}
	for _, key := range []string{"0123456789abcdef", "fedcba9876543210", "ffffffffffffffff"} {
		if got, err := r.led.NodesByKey(r.ctx, key); err != nil || len(got) != 0 {
			t.Fatalf("%s (pending pin, key that does not hash to its id, unknown): %+v %v", key, got, err)
		}
	}
}

func TestAttestReplaySetIsPerSignatureAcrossAttempts(t *testing.T) {
	r := newAttestRig(t)
	u := attest.Use{AttemptID: "att-1", KeyID: "ci-node-0123456789abcdef", SigDigest: "d1", NodeID: "node-1", RunID: "run-1", RepoID: "repo", Revision: strings.Repeat("a", 40), Job: "build"}
	for i := 0; i < 2; i++ {
		if err := r.led.Consume(r.ctx, u); err != nil {
			t.Fatalf("consume %d of the same triple: %v", i, err)
		}
	}
	if err := r.led.Consume(r.ctx, attest.Use{AttemptID: "att-2", KeyID: u.KeyID, SigDigest: "d1", NodeID: "node-1"}); !errors.Is(err, attest.ErrReplay) {
		t.Fatalf("the same signature under another attempt: %v", err)
	}
	if ok, err := r.led.Replayed(r.ctx, "att-2", u.KeyID, "d1"); err != nil || !ok {
		t.Fatalf("replayed under att-2: %v %v", ok, err)
	}
	if ok, _ := r.led.Replayed(r.ctx, "att-1", u.KeyID, "d1"); ok {
		t.Fatal("the attempt that owns the signature is not a replay")
	}
	if err := r.led.Consume(r.ctx, attest.Use{AttemptID: "att-2", KeyID: u.KeyID, SigDigest: "d2", NodeID: "node-1"}); err != nil {
		t.Fatalf("another signature under att-2: %v", err)
	}
	if err := r.led.Consume(r.ctx, attest.Use{AttemptID: "att-3"}); err == nil {
		t.Fatal("an empty use was recorded")
	}
}

func TestAttestRevokeNodeQuarantinesAndReturnsTheReevaluationSet(t *testing.T) {
	r := newAttestRig(t)
	rev := strings.Repeat("a", 40)
	keyHex := r.pinKey("node-1", 1, ptr(true))
	keyID := "ci-node-" + keyHex
	for _, id := range []string{"att-1", "att-2"} {
		r.attempt(id, rev)
		if err := r.led.Consume(r.ctx, attest.Use{AttemptID: id, KeyID: keyID, SigDigest: "dig-" + id, NodeID: "node-1", RunID: "run-" + id, RepoID: "repo-1", Revision: rev, Job: "build"}); err != nil {
			t.Fatal(err)
		}
	}
	// the key of another node signed too: it must not be touched
	r.led.Consume(r.ctx, attest.Use{AttemptID: "att-3", KeyID: "ci-node-ffffffffffffffff", SigDigest: "x", NodeID: "node-9", RunID: "run-att-3", RepoID: "repo-9", Revision: strings.Repeat("c", 40)})
	r.exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES(?,?,1,'posted',1)`, rev, "nself-ci@acme/widget")
	r.exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES(?,?,1,'pending',1)`, rev, "nself-ci@acme/pending-only")
	r.exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES(?,?,1,'posted',1)`, strings.Repeat("c", 40), "nself-ci@acme/other")
	if err := r.s.RecordRelease(r.ctx, ReleaseRow{RunID: "run-att-1", Project: "widget", Revision: rev, State: ReleasePublished, ManifestSHA256: strings.Repeat("ab", 32), KeyID: "ci-release-0123456789abcdef"}); err != nil {
		t.Fatal(err)
	}
	got, err := r.led.RevokeNode(r.ctx, "node-1", "node.revoked")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Keys, []string{keyID}) {
		t.Fatalf("keys %v", got.Keys)
	}
	wantGates := []attest.GateRef{{RepoID: "repo-1", Revision: rev, Job: "build"}}
	if !reflect.DeepEqual(got.Gates, wantGates) {
		t.Fatalf("gates %+v", got.Gates)
	}
	if !reflect.DeepEqual(got.Statuses, []attest.StatusKey{{Revision: rev, Context: "nself-ci@acme/widget"}}) {
		t.Fatalf("statuses %+v (only posted entries of the affected revision)", got.Statuses)
	}
	if len(got.Releases) != 1 || got.Releases[0].RunID != "run-att-1" || got.Releases[0].Project != "widget" {
		t.Fatalf("releases %+v (only published releases of quarantined runs)", got.Releases)
	}
	for _, c := range []struct {
		run  string
		want bool
	}{{"run-att-1", true}, {"run-att-2", true}, {"run-att-3", false}} {
		if q, _ := r.led.Quarantined(r.ctx, c.run); q != c.want {
			t.Errorf("quarantined(%s) = %v", c.run, q)
		}
	}
	if rv, _ := r.led.Revoked(r.ctx, keyID); !rv {
		t.Fatal("key not revoked")
	}
	if rv, _ := r.led.Revoked(r.ctx, "ci-node-ffffffffffffffff"); rv {
		t.Fatal("another node's key revoked")
	}
	again, err := r.led.RevokeNode(r.ctx, "node-1", "node.revoked")
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("a second revocation: %+v %v", again, err)
	}
	if _, err = r.led.RevokeNode(r.ctx, "", "x"); err == nil {
		t.Fatal("revoked no node")
	}
	// a key the node used and then rotated away is still revoked, through its uses
	r.exec(`UPDATE pki_node_key SET key_id='1111111111111111' WHERE node_id='node-1'`)
	rotated, err := r.led.RevokeNode(r.ctx, "node-1", "node.revoked")
	if err != nil || !reflect.DeepEqual(rotated.Keys, []string{"ci-node-1111111111111111", keyID}) {
		t.Fatalf("after a rotation: %+v %v", rotated.Keys, err)
	}
}

func TestAttestRevokedNodeAndKeyAreRefused(t *testing.T) {
	r := newAttestRig(t)
	keyHex := r.pinKey("node-1", 1, ptr(true))
	keyID := "ci-node-" + keyHex
	if got, _ := r.led.NodesByKey(r.ctx, keyHex); len(got) != 1 {
		t.Fatalf("control: %+v", got)
	}
	// a node revoked through the registry (nodes revoke) is not a signing node any more
	r.exec(`UPDATE node SET state='revoked' WHERE id='node-1'`)
	if got, _ := r.led.NodesByKey(r.ctx, keyHex); len(got) != 0 {
		t.Fatalf("a registry-revoked node still pins a key: %+v", got)
	}
	r.exec(`UPDATE node SET state='online' WHERE id='node-1'`)
	// a revocation that commits after the verifier's check refuses the use at the write
	u := attest.Use{AttemptID: "att-1", KeyID: keyID, SigDigest: "d1", NodeID: "node-1", RunID: "run-1"}
	if _, err := r.led.RevokeNode(r.ctx, "node-1", "node.revoked"); err != nil {
		t.Fatal(err)
	}
	if err := r.led.Consume(r.ctx, u); !errors.Is(err, attest.ErrRevoked) {
		t.Fatalf("consume after revoke: %v", err)
	}
	if got, _ := r.led.NodesByKey(r.ctx, keyHex); len(got) != 0 {
		t.Fatalf("a revoked node still pins a key: %+v", got)
	}
	// a node with no key yet is marked too: a key pinned for it later is refused
	late := r.pinKeyLater("node-late")
	if _, err := r.led.RevokeNode(r.ctx, "node-late", "node.revoked"); err != nil {
		t.Fatal(err)
	}
	r.exec(`UPDATE pki_node_key SET public=?,key_id=? WHERE node_id='node-late'`, late.public, late.keyHex)
	if got, _ := r.led.NodesByKey(r.ctx, late.keyHex); len(got) != 0 {
		t.Fatalf("a key pinned after the revocation verifies: %+v", got)
	}
}

type lateKey struct{ keyHex, public string }

// pinKeyLater leaves a pending pin (no key) for the node and returns the key it will pin later.
func (r *attestRig) pinKeyLater(node string) lateKey {
	pub := ed25519.NewKeyFromSeed([]byte(strings.Repeat("\x07", ed25519.SeedSize))).Public().(ed25519.PublicKey)
	sum := sha256.Sum256(pub)
	r.exec(`INSERT INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES(?,?,'',1)`, node, "0000000000000000")
	return lateKey{hex.EncodeToString(sum[:])[:16], base64.StdEncoding.EncodeToString(pub)}
}
