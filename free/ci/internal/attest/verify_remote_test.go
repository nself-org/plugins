package attest_test

// Purpose: tests of the runner-key verifier (P7-TRUST-20), over an in-memory ledger and over the real SQLite one: a document signed by the registered, eligible,
// lease-holding node verifies; every adversarial fixture is refused with its own reason; the signed half of
// invariant I13 (an unverifiable signature never satisfies a gate) over the shared world product.
// Inputs:  deterministic Ed25519 keys (fixed seeds), an in-memory Ledger, fixtures in testdata/remote.
// Outputs: pass or fail; set NSELF_UPDATE_GOLDEN=1 to rewrite the fixture files.
// Constraints: the Ticket's mutants run TestInvariant_I13*, TestVerifyRemote* and TestSign*; the fixture
//          keys are throwaway seeds that guard nothing.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
	_ "modernc.org/sqlite"
)

// seedSigner is a node key from a fixed seed.
type seedSigner struct{ priv ed25519.PrivateKey }

func newSeedSigner(b byte) seedSigner {
	return seedSigner{ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))}
}
func (s seedSigner) pub() ed25519.PublicKey { return s.priv.Public().(ed25519.PublicKey) }
func (s seedSigner) KeyID() string          { return nodekey.KeyIDOf(s.pub()) }
func (s seedSigner) Sign(m []byte) ([]byte, error) {
	return ed25519.Sign(s.priv, m), nil
}

var _ nodekey.Signer = seedSigner{}

// memLedger is an in-memory attest.Ledger.
type memLedger struct {
	attempts map[string]attest.AttemptFacts
	nodes    map[string][]attest.KeyNode
	revoked  map[string]bool
	uses     map[[3]string]attest.Use
	failOn   string
	// revokeOnConsume models a revocation that commits between the verifier's check and the write.
	revokeOnConsume bool
}

var errLedger = errors.New("ledger down")

func (l *memLedger) AttemptFacts(_ context.Context, id string) (attest.AttemptFacts, bool, error) {
	if l.failOn == "attempt" {
		return attest.AttemptFacts{}, false, errLedger
	}
	f, ok := l.attempts[id]
	return f, ok, nil
}
func (l *memLedger) NodesByKey(_ context.Context, id string) ([]attest.KeyNode, error) {
	if l.failOn == "nodes" {
		return nil, errLedger
	}
	return l.nodes[id], nil
}
func (l *memLedger) Revoked(_ context.Context, id string) (bool, error) {
	if l.failOn == "revoked" {
		return false, errLedger
	}
	return l.revoked[id], nil
}
func (l *memLedger) Replayed(_ context.Context, attempt, key, digest string) (bool, error) {
	for k := range l.uses {
		if k[1] == key && k[2] == digest && k[0] != attempt {
			return true, nil
		}
	}
	return false, nil
}
func (l *memLedger) Consume(_ context.Context, u attest.Use) error {
	if l.failOn == "consume" {
		return errLedger
	}
	if l.revokeOnConsume || l.revoked[u.KeyID] {
		return attest.ErrRevoked
	}
	if ok, _ := l.Replayed(nil, u.AttemptID, u.KeyID, u.SigDigest); ok {
		return attest.ErrReplay
	}
	l.uses[[3]string{u.AttemptID, u.KeyID, u.SigDigest}] = u
	return nil
}

const holderNode, otherNode = "node-1", "node-2"

var keyA, keyB = newSeedSigner(1), newSeedSigner(2)

// newLedger: attempts att-1 and att-2 were leased to node-1 at goodRevision; node-1 and node-2 pin their
// keys and are signing eligible.
func newLedger() *memLedger {
	return &memLedger{
		attempts: map[string]attest.AttemptFacts{"att-1": goodFacts(), "att-2": goodFacts()},
		nodes: map[string][]attest.KeyNode{
			keyA.KeyID(): {{NodeID: holderNode, Public: keyA.pub(), SigningEligible: true}},
			keyB.KeyID(): {{NodeID: otherNode, Public: keyB.pub(), SigningEligible: true}},
		},
		revoked: map[string]bool{}, uses: map[[3]string]attest.Use{},
	}
}

// goodFacts are the coordinator's records of the attempt goodEvidence describes.
func goodFacts() attest.AttemptFacts {
	return attest.AttemptFacts{RunnerID: holderNode, Revision: goodRevision, RunID: "run-1", RepoID: "repo-1", SourceTrust: "owner", PolicyDigest: "policy-1", SelectionMode: "full", Job: "build"}
}

func verifierOf(l attest.Ledger) *attest.RunnerVerifier {
	return &attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}
}

// attemptEvidence is goodEvidence as one attempt of job "build" records it: its own job only.
func attemptEvidence(attempt string) model.Evidence {
	ev := goodEvidence()
	ev.Attempt = &model.AttemptRef{ID: attempt, N: 1}
	ev.Job = strp("build")
	ev.Checks = append([]model.Check(nil), ev.Checks...)
	for i := range ev.Checks {
		ev.Checks[i].JobID = "build"
	}
	return ev
}

func mustSign(t testing.TB, ev model.Evidence, s nodekey.Signer) model.Evidence {
	t.Helper()
	out, err := attest.Sign(ev, s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// remoteCase is one fixture: a document, the ledger it is judged against and the reason it must give.
type remoteCase struct {
	name   string
	ev     model.Evidence
	ledger func(*memLedger)
	setup  func(*testing.T, *memLedger, attest.RemoteVerifier) // optional pre-step (a first, honest verification)
	reason attest.RemoteReason                                 // "" means the document verifies
}

func remoteCases(t testing.TB) []remoteCase {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	edited := good
	edited.Result = "fail"
	swapped := good
	swapped.Revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	otherPurpose := good
	id := "ci-audit-" + keyA.KeyID()
	otherPurpose.Signature.KeyID = &id
	withKeyID := func(k string) model.Evidence { e := good; e.Signature.KeyID = &k; return e }
	byB := mustSign(t, attemptEvidence("att-1"), keyB)
	replayed := mustSign(t, attemptEvidence("att-2"), keyA)
	replayed.Signature.Value = good.Signature.Value // att-1's signature on att-2's document
	noSig := good
	noSig.Signature = model.Signature{Kind: "runner-ed25519"}
	badEnc := good
	badEnc.Signature.Value = strp("not base64!")
	noAttempt := mustSign(t, goodEvidence(), keyA)
	unknownAttempt := mustSign(t, attemptEvidence("att-9"), keyA)
	emptyRev := attemptEvidence("att-1")
	emptyRev.Revision = ""
	noRevision := mustSign(t, emptyRev, keyA)
	forged := func(edit func(*model.Evidence)) model.Evidence {
		e := attemptEvidence("att-1")
		edit(&e)
		return mustSign(t, e, keyA)
	}
	return []remoteCase{
		{name: "valid", ev: good},
		{name: "edited-result", ev: edited, reason: attest.RemoteSignatureInvalid},
		{name: "swapped-revision", ev: swapped, reason: attest.RemoteRevisionMismatch},
		{name: "replayed-on-other-attempt", ev: replayed, reason: attest.RemoteReplay,
			setup: func(t *testing.T, l *memLedger, v attest.RemoteVerifier) {
				doc := marshal(t, good)
				if err := v.VerifyEvidence(doc, good); err != nil {
					t.Fatalf("the honest document did not verify: %v", err)
				}
			}},
		{name: "revoked-key", ev: good, ledger: func(l *memLedger) { l.revoked["ci-node-"+keyA.KeyID()] = true }, reason: attest.RemoteKeyRevoked},
		{name: "unknown-key", ev: good, ledger: func(l *memLedger) { delete(l.nodes, keyA.KeyID()) }, reason: attest.RemoteKeyUnknown},
		{name: "other-purpose-key", ev: otherPurpose, reason: attest.RemoteKeyPurpose},
		{name: "ineligible-node", ev: good, ledger: func(l *memLedger) { l.nodes[keyA.KeyID()][0].SigningEligible = false }, reason: attest.RemoteNodeNotEligible},
		{name: "other-eligible-node-did-not-lease", ev: byB, reason: attest.RemoteLeaseHolderMismatch},
		{name: "no-signature", ev: noSig, reason: attest.RemoteSignatureMissing},
		{name: "malformed-signature", ev: badEnc, reason: attest.RemoteSignatureMalformed},
		{name: "malformed-key-id", ev: withKeyID("ci node"), reason: attest.RemoteKeyMalformed},
		{name: "no-attempt", ev: noAttempt, reason: attest.RemoteAttemptUnknown},
		{name: "unknown-attempt", ev: unknownAttempt, reason: attest.RemoteAttemptUnknown},
		{name: "forged-repo", ev: forged(func(e *model.Evidence) { e.RepoID = "victim-protected" }), reason: attest.RemoteContextMismatch},
		{name: "forged-source-trust", ev: forged(func(e *model.Evidence) { e.SourceTrust = "untrusted" }), reason: attest.RemoteContextMismatch},
		{name: "forged-policy-digest", ev: forged(func(e *model.Evidence) { e.PolicyDigest = "policy-loose" }), reason: attest.RemoteContextMismatch},
		{name: "forged-selection-mode", ev: forged(func(e *model.Evidence) { e.Selection.Mode = "affected" }), reason: attest.RemoteContextMismatch},
		{name: "forged-run-id", ev: forged(func(e *model.Evidence) { e.RunID = "" }), reason: attest.RemoteContextMismatch},
		{name: "forged-job", ev: forged(func(e *model.Evidence) { e.Job = strp("deploy") }), reason: attest.RemoteContextMismatch},
		{name: "forged-no-job", ev: forged(func(e *model.Evidence) { e.Job = nil }), reason: attest.RemoteContextMismatch},
		{name: "forged-foreign-check", ev: forged(func(e *model.Evidence) {
			e.Checks = append([]model.Check(nil), e.Checks...)
			e.Checks[0].JobID = "lint"
		}), reason: attest.RemoteContextMismatch},
		{name: "revoked-during-verification", ev: good, ledger: func(l *memLedger) { l.revokeOnConsume = true }, reason: attest.RemoteKeyRevoked},
		{name: "empty-revision", ev: noRevision, reason: attest.RemoteRevisionMismatch},
		{name: "two-keys-under-one-id", ev: good, ledger: func(l *memLedger) {
			l.nodes[keyA.KeyID()] = append(l.nodes[keyA.KeyID()], attest.KeyNode{NodeID: otherNode, Public: keyB.pub(), SigningEligible: true})
		}, reason: attest.RemoteKeyUnknown},
		{name: "ledger-down", ev: good, ledger: func(l *memLedger) { l.failOn = "attempt" }, reason: attest.RemoteLedgerUnavailable},
	}
}

func runCase(t *testing.T, c remoteCase, doc []byte) {
	t.Helper()
	l := newLedger()
	v := verifierOf(l)
	if c.setup != nil {
		c.setup(t, l, v)
	}
	if c.ledger != nil {
		c.ledger(l)
	}
	err := v.VerifyEvidence(doc, c.ev)
	if got := attest.RemoteReasonOf(err); got != c.reason || (c.reason == "" && err != nil) {
		t.Fatalf("%s: reason %q (err %v), want %q", c.name, got, err, c.reason)
	}
}

func TestVerifyRemoteAdversarialFixtures(t *testing.T) {
	seen := map[attest.RemoteReason]string{}
	for _, c := range remoteCases(t) {
		t.Run(c.name, func(t *testing.T) { runCase(t, c, marshal(t, c.ev)) })
		if c.reason != "" {
			if prev, dup := seen[c.reason]; dup && c.name != "no-attempt" && c.name != "unknown-attempt" && c.name != "empty-revision" && !strings.HasPrefix(c.name, "forged-") && c.name != "revoked-during-verification" && c.name != "two-keys-under-one-id" {
				t.Errorf("%s and %s are refused with the same reason %s", prev, c.name, c.reason)
			}
			seen[c.reason] = c.name
		}
	}
}

func TestVerifyRemoteFixtureFiles(t *testing.T) {
	dir := filepath.Join("testdata", "remote")
	update := os.Getenv("NSELF_UPDATE_GOLDEN") == "1"
	var names []string
	for _, c := range remoteCases(t) {
		names = append(names, c.name)
		path := filepath.Join(dir, c.name+".json")
		doc := append(marshal(t, c.ev), '\n')
		if update {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, doc, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		onDisk, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%v (NSELF_UPDATE_GOLDEN=1 writes the fixtures)", err)
		}
		if !bytes.Equal(onDisk, doc) {
			t.Fatalf("%s is stale (NSELF_UPDATE_GOLDEN=1 rewrites it)", path)
		}
		var ev model.Evidence
		if err := json.Unmarshal(onDisk, &ev); err != nil {
			t.Fatal(err)
		}
		c.ev = ev
		t.Run(c.name, func(t *testing.T) { runCase(t, c, onDisk) })
	}
	entries, _ := os.ReadDir(dir)
	var have []string
	for _, e := range entries {
		have = append(have, e.Name())
	}
	sort.Strings(have)
	if len(have) != len(names) {
		t.Fatalf("testdata/remote holds %d files for %d cases: %v", len(have), len(names), have)
	}
}

func TestVerifyRemoteRefusesADocumentThatIsNotTheEvidence(t *testing.T) {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	// the signature is valid for the bytes; the decoded evidence the checks read says att-2 (also leased to
	// the node, same facts), so only the binding between the two catches it
	pairedWithOther := good
	pairedWithOther.Attempt = &model.AttemptRef{ID: "att-2", N: 1}
	swapped := good
	swapped.Revision = "dddddddddddddddddddddddddddddddddddddddd"
	for name, c := range map[string]struct {
		doc []byte
		ev  model.Evidence
	}{"other attempt": {marshal(t, good), pairedWithOther}, "other revision": {marshal(t, swapped), good}} {
		if err := verifierOf(newLedger()).VerifyEvidence(c.doc, c.ev); attest.RemoteReasonOf(err) != attest.RemoteSignatureInvalid {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestVerifyRemoteIgnoresReadTimeFreshness(t *testing.T) {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	reread := good
	reread.Freshness = model.Freshness{State: "stale"} // recomputed at read time by the caller, never signed
	if err := verifierOf(newLedger()).VerifyEvidence(marshal(t, good), reread); err != nil {
		t.Fatalf("a re-evaluated freshness broke the binding: %v", err)
	}
}

func TestVerifyRemoteIsIdempotentAndAcceptsBothEncodings(t *testing.T) {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	l := newLedger()
	v := verifierOf(l)
	for i := 0; i < 3; i++ {
		if err := v.VerifyEvidence(marshal(t, good), good); err != nil {
			t.Fatalf("verification %d of the same document: %v", i, err)
		}
	}
	if len(l.uses) != 1 {
		t.Fatalf("one document is %d uses", len(l.uses))
	}
	raw, _ := base64.RawStdEncoding.DecodeString(*good.Signature.Value)
	padded := good
	padded.Signature.Value = strp(base64.StdEncoding.EncodeToString(raw))
	if err := v.VerifyEvidence(marshal(t, padded), padded); err != nil {
		t.Fatalf("the padded text of the same signature: %v", err)
	}
	if len(l.uses) != 1 {
		t.Fatalf("padded and unpadded text of one signature made %d uses", len(l.uses))
	}
}

func TestVerifyRemoteUnknownFieldsIntegersAndFreshness(t *testing.T) {
	ev := attemptEvidence("att-1")
	ev.Producer.Kind = "signed-remote"
	var obj map[string]any
	dec := json.NewDecoder(bytes.NewReader(marshal(t, ev)))
	dec.UseNumber()
	if err := dec.Decode(&obj); err != nil {
		t.Fatal(err)
	}
	obj["x_future"] = map[string]any{"count": json.Number("12345678901234567890"), "tags": []any{"a", json.Number("7")}}
	doc, _ := json.Marshal(obj)
	raw, err := attest.SignDocumentRaw(doc, keyA)
	if err != nil {
		t.Fatal(err)
	}
	obj["signature"] = map[string]any{"kind": "runner-ed25519", "key_id": "ci-node-" + keyA.KeyID(), "value": base64.RawStdEncoding.EncodeToString(raw)}
	obj["freshness"] = map[string]any{"state": "stale"} // freshness is recomputed, never signed
	doc, _ = json.Marshal(obj)
	// a JSON round trip through the decoder that keeps numbers as written leaves the bytes signable
	var back map[string]any
	dec = json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	if err := dec.Decode(&back); err != nil {
		t.Fatal(err)
	}
	doc, _ = json.Marshal(back)
	var known model.Evidence
	if err := json.Unmarshal(doc, &known); err != nil {
		t.Fatal(err)
	}
	if err := verifierOf(newLedger()).VerifyEvidence(doc, known); err != nil {
		t.Fatalf("a document with an unknown field and a 20-digit integer: %v", err)
	}
	back["x_future"].(map[string]any)["count"] = json.Number("12345678901234567891")
	doc, _ = json.Marshal(back)
	if err := verifierOf(newLedger()).VerifyEvidence(doc, known); attest.RemoteReasonOf(err) != attest.RemoteSignatureInvalid {
		t.Fatalf("an edited unknown field: %v", err)
	}
}

func TestVerifyRemoteLedgerFailuresFailClosed(t *testing.T) {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	for _, where := range []string{"attempt", "nodes", "revoked", "consume"} {
		l := newLedger()
		l.failOn = where
		err := verifierOf(l).VerifyEvidence(marshal(t, good), good)
		if attest.RemoteReasonOf(err) != attest.RemoteLedgerUnavailable {
			t.Errorf("%s down: %v", where, err)
		}
	}
	if err := (&attest.RunnerVerifier{}).VerifyEvidence(marshal(t, good), good); attest.RemoteReasonOf(err) != attest.RemoteLedgerUnavailable {
		t.Errorf("no ledger: %v", err)
	}
	if err := verifierOf(nil).VerifyEvidence(marshal(t, good), good); attest.RemoteReasonOf(err) != attest.RemoteLedgerUnavailable {
		t.Errorf("nil ledger: %v", err)
	}
}

func TestVerifyRemoteGateAcceptsOnlyVerifiedEvidence(t *testing.T) {
	good := mustSign(t, attemptEvidence("att-1"), keyA)
	g := goodGate(good)
	g.RequiredJobs = []string{"build"}
	attest.RegisterRemoteVerifier(verifierOf(newLedger()))
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	if got := attest.GateAccept(good, g); len(got) != 0 {
		t.Fatalf("signed, verified evidence refused: %v", got)
	}
	edited := good
	edited.Counts.Pass = 3
	edited.Checks = append([]model.Check(nil), good.Checks...)
	edited.Checks[0].Excerpt = "edited"
	if got := attest.GateAccept(edited, g); !hasReason(got, attest.ReasonSignatureUnverifiable) {
		t.Fatalf("edited evidence accepted: %v", got)
	}
}

// TestVerifyRemoteInvariantI13Signed is the signed half of I13: in every world, signed-remote evidence is
// accepted by a gate only when its signature verifies and the rest of the gate agrees; a signature that
// does not verify (edited, other revision, revoked, unknown, replayed, ineligible, other lease holder)
// never satisfies it.
func TestVerifyRemoteInvariantI13Signed(t *testing.T) {
	var n, rejected, accepted int
	invariants.Check(t, func(w invariants.World) error {
		sc := build(w)
		if sc.producer != "signed-remote" {
			return nil
		}
		n++
		ev := sc.ev
		ev.Attempt = &model.AttemptRef{ID: "att-1", N: 1}
		// one attempt vouches for its own job only: every world is judged on the single required job "build"
		ev.Job = strp("build")
		ev.Checks = append([]model.Check(nil), ev.Checks...)
		for i := range ev.Checks {
			ev.Checks[i].JobID = "build"
		}
		sc.gate.RequiredJobs = []string{"build"}
		l := newLedger()
		facts := attest.AttemptFacts{RunnerID: holderNode, Revision: ev.Revision, RunID: ev.RunID, RepoID: ev.RepoID, SourceTrust: string(ev.SourceTrust),
			PolicyDigest: ev.PolicyDigest, SelectionMode: string(ev.Selection.Mode), Job: "build"}
		l.attempts["att-1"] = facts
		signer, mutation := nodekey.Signer(keyA), n%10
		ev = mustSign(t, ev, signer)
		switch mutation {
		case 1:
			ev.Result = "error"
		case 2:
			facts.Revision = "cccccccccccccccccccccccccccccccccccccccc"
			l.attempts["att-1"] = facts
		case 3:
			l.revoked["ci-node-"+keyA.KeyID()] = true
		case 4:
			delete(l.nodes, keyA.KeyID())
		case 5:
			l.uses[[3]string{"att-other", "ci-node-" + keyA.KeyID(), attest.SignatureDigest(mustRaw(t, ev))}] = attest.Use{}
		case 6:
			l.nodes[keyA.KeyID()][0].SigningEligible = false
		case 7:
			facts.RunnerID = otherNode
			l.attempts["att-1"] = facts
		case 8:
			facts.RepoID = "some-other-repo"
			l.attempts["att-1"] = facts
		}
		attest.RegisterRemoteVerifier(verifierOf(l))
		got := attest.GateAccept(ev, sc.gate)
		if mutation != 0 && mutation != 9 {
			rejected++
			if !hasReason(got, attest.ReasonSignatureUnverifiable) || len(got) == 0 {
				return errors.New("an unverifiable signature satisfied the gate")
			}
			return nil
		}
		if want := sc.shouldAccept(true); want != (len(got) == 0) {
			return errors.New("a verified signature changed the gate's decision")
		}
		if len(got) == 0 {
			accepted++
		}
		return nil
	})
	attest.RegisterRemoteVerifier(nil)
	if rejected == 0 || accepted == 0 {
		t.Fatalf("vacuous: %d rejected, %d accepted", rejected, accepted)
	}
}

func mustRaw(t testing.TB, ev model.Evidence) []byte {
	t.Helper()
	raw, err := base64.RawStdEncoding.DecodeString(*ev.Signature.Value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// repoIDOf is run.shortDigest: the first 8 bytes of the sha256 of the project, in hex.
func repoIDOf(project string) string {
	sum := sha256.Sum256([]byte(project))
	return hex.EncodeToString(sum[:8])
}

func TestVerifyRemoteOverTheRealLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	seed, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// repo id "repo-1" of goodEvidence is the project's digest in production; this test names the project
	// so that the derived id is what the evidence carries.
	ev := attemptEvidence("att-1")
	project := "widget"
	ev.RepoID = repoIDOf(project)
	exec := func(q string, a ...any) {
		t.Helper()
		if _, err := seed.Exec(q, a...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO pipeline(id,project,revision,trigger,source_trust,privacy_zone,policy_digest,input_digest,selection_mode,selection_reason,selection_digest,status,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1)`,
		ev.RunID, project, ev.Revision, "local", string(ev.SourceTrust), "any", ev.PolicyDigest, "in", string(ev.Selection.Mode), "r", "d", "running")
	exec(`INSERT INTO job(id,pipeline_id,name,kind,idempotent,infra_max,input_digest,required,status) VALUES('j1',?,'build','check',1,1,'in',1,'running')`, ev.RunID)
	exec(`INSERT INTO attempt(id,job_id,n,state) VALUES('att-1','j1',1,'passed')`)
	exec(`INSERT INTO sched_admitted(attempt_id,pipeline_id,project,user_id,team_id,provider,runner_id,world_digest,admitted_at) VALUES('att-1',?,?,'u','t','agent',?,'w',1)`, ev.RunID, project, holderNode)
	exec(`INSERT INTO pki_node_key(node_id,key_id,public,pinned_at) VALUES(?,?,?,1)`, holderNode, keyA.KeyID(), base64.StdEncoding.EncodeToString(keyA.pub()))
	exec(`INSERT INTO node(id,name,version,lifecycle,trust_json,deploy_host_json,state,created_at,updated_at) VALUES(?,?,1,'{}','{}','{}','online',1,1)`, holderNode, holderNode)
	exec(`INSERT INTO capability_snapshot(node_id,digest,doc_json,observed_at) VALUES(?,'d','{"separation":{"signing_eligible":{"value":true}}}',1)`, holderNode)
	exec(`INSERT INTO outbox(revision,context,seq,state,created_at) VALUES(?,'nself-ci@acme/widget',1,'posted',1)`, ev.Revision)
	_ = seed.Close()

	led := store.AttestLedger{S: st}
	v := &attest.RunnerVerifier{Ledger: func() attest.Ledger { return led }}
	good := mustSign(t, ev, keyA)
	if err := v.VerifyEvidence(marshal(t, good), good); err != nil {
		t.Fatalf("signed evidence over the real ledger: %v", err)
	}
	forged := ev
	forged.RepoID = "victim-protected"
	forged = mustSign(t, forged, keyA)
	if err := v.VerifyEvidence(marshal(t, forged), forged); attest.RemoteReasonOf(err) != attest.RemoteContextMismatch {
		t.Fatalf("forged repository: %v", err)
	}
	if q, _ := led.Quarantined(context.Background(), ev.RunID); q {
		t.Fatal("quarantined before the revocation")
	}

	rep := &fakeReposter{}
	var audited []auditRow
	r := attest.Revoker{Ledger: led, Reposter: rep, Audit: func(_ context.Context, actor, action, target, detail string) error {
		audited = append(audited, auditRow{actor, action, target, detail})
		return nil
	}}
	got, err := r.Revoke(context.Background(), holderNode, "cli:alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Keys) != 1 || len(got.Gates) != 1 || got.Gates[0].Job != "build" || got.Gates[0].RepoID != ev.RepoID {
		t.Fatalf("revocation set: %+v", got)
	}
	if len(rep.keys) != 1 || rep.keys[0].Context != "nself-ci@acme/widget" || rep.reason != "node.revoked" {
		t.Fatalf("repost: %+v", rep)
	}
	if q, _ := led.Quarantined(context.Background(), ev.RunID); !q {
		t.Fatal("the run was not quarantined")
	}
	if err := v.VerifyEvidence(marshal(t, good), good); attest.RemoteReasonOf(err) != attest.RemoteKeyRevoked {
		t.Fatalf("the same evidence after the revocation: %v", err)
	}
	if len(audited) == 0 {
		t.Fatal("no audit event")
	}
}
