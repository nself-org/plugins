//go:build security

package security

// Purpose: evidence integrity and publication (I12, I13; TM-26, TM-27, TM-28, TM-36).
//   - stale evidence never satisfies a current-revision gate: a moved head, changed config, lockfile or
//     generated input, an unknown current state, another policy;
//   - forged evidence is refused: self-reported where it is not admitted, an unsigned or edited
//     remote document, a signature from a node that did not hold the lease, a revoked key, a replay
//     on another attempt, a context the coordinator does not recognise;
//   - a signature binds the lease holder: a valid, eligible node that did not lease the attempt cannot
//     vouch for it;
//   - the publication profile of a public repository carries no private address, host, user or path.
// Inputs:  synthetic evidence documents, deterministic throwaway Ed25519 seeds, an in-memory ledger.
// Outputs: pass or fail.
// Constraints: the verifier, signer, freshness and gate under test are the production ones.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const goodRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func strp(s string) *string { return &s }

// goodEvidence is bound, clean, passing, full selection, coordinator-recorded.
func goodEvidence() model.Evidence {
	checks := []model.Check{
		{ID: "vet", JobID: "build", Kind: "quality", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "lint", JobID: "lint", Kind: "quality", Tool: "golangci-lint", Required: true, Substantive: true, Result: "pass"},
	}
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: goodRevision, RepoID: "repo-1", RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64),
			Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}, {Path: "pnpm-lock.yaml", SHA256: strings.Repeat("2", 64)}},
			Generated: []model.DigestRef{{Path: "gen/api.go", SHA256: strings.Repeat("3", 64)}}},
		Runner:       model.RunnerRef{ID: "mac-mini.local", Provider: "local", Platform: "darwin/arm64", Isolation: "process"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"},
		ToolVersions: map[string]string{"go": "1.27.2"}, Checks: checks, Artifacts: []model.Artifact{}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 3, Total: 3},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

// goodGate accepts goodEvidence; the gate comes from protected policy, never from the document.
func goodGate(ev model.Evidence) attest.Gate {
	cur := ev.InputDigests
	return attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build", "lint"},
		MinTrust: "internal", Class: attest.ClassDefault, Current: &cur}
}

func hasReason(rs []attest.Reason, want attest.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

// TestStaleEvidenceNeverSatisfiesGate (I13, TM-27): the control passes; every way the world moves
// since the evidence was made is refused with its own reason.
func TestStaleEvidenceNeverSatisfiesGate(t *testing.T) {
	ev := goodEvidence()
	if got := attest.GateAccept(ev, goodGate(ev)); len(got) != 0 {
		t.Fatalf("control: current evidence rejected: %v", got)
	}
	rows := []struct {
		name string
		bend func(*model.Evidence, *attest.Gate)
		want attest.Reason
	}{
		{"head moved", func(e *model.Evidence, g *attest.Gate) { g.Revision = strings.Repeat("b", 40) }, attest.ReasonRevisionMismatch},
		{"config changed", func(e *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Config = strings.Repeat("d", 64)
			g.Current = &c
		}, attest.ReasonStale},
		{"lockfile changed", func(e *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Lockfiles = []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("9", 64)}, c.Lockfiles[1]}
			g.Current = &c
		}, attest.ReasonStale},
		{"lockfile added", func(e *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Lockfiles = append(append([]model.DigestRef(nil), c.Lockfiles...), model.DigestRef{Path: "Cargo.lock", SHA256: strings.Repeat("4", 64)})
			g.Current = &c
		}, attest.ReasonStale},
		{"generated input changed", func(e *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Generated = []model.DigestRef{{Path: "gen/api.go", SHA256: strings.Repeat("8", 64)}}
			g.Current = &c
		}, attest.ReasonStale},
		{"current inputs unknown", func(e *model.Evidence, g *attest.Gate) { g.Current = nil }, attest.ReasonStale},
		{"policy changed", func(e *model.Evidence, g *attest.Gate) { g.PolicyDigest = "policy-2" }, attest.ReasonPolicyMismatch},
		{"another repository", func(e *model.Evidence, g *attest.Gate) { g.RepoID = "repo-2" }, attest.ReasonRepoMismatch},
		{"a required job vanished", func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = []string{"build", "lint", "deploy-check"} }, attest.ReasonJobsMismatch},
		{"trust fell below the gate minimum", func(e *model.Evidence, g *attest.Gate) { e.SourceTrust = "untrusted" }, attest.ReasonTrustBelowMin},
		{"worktree dirty", func(e *model.Evidence, g *attest.Gate) { e.WorktreeState = "dirty" }, attest.ReasonUnbound},
		{"partial selection on a release gate", func(e *model.Evidence, g *attest.Gate) { e.Selection.Mode = "affected"; g.Class = attest.ClassRelease }, attest.ReasonSelectionPartial},
		{"a failing check", func(e *model.Evidence, g *attest.Gate) {
			e.Checks[2].Result, e.Checks[2].Required = "fail", false
			e.Counts = model.Counts{Pass: 2, Fail: 1, Total: 3}
		}, attest.ReasonNotPassing},
	}
	for _, row := range rows {
		attest.RegisterRemoteVerifier(nil)
		e := goodEvidence()
		g := goodGate(e)
		row.bend(&e, &g)
		if got := attest.GateAccept(e, g); !hasReason(got, row.want) {
			t.Errorf("%s: reasons %v, want %s", row.name, got, row.want)
		}
	}
}

// seedSigner is a node key from a fixed seed (throwaway: it guards nothing).
type seedSigner struct{ priv ed25519.PrivateKey }

func newSeedSigner(b byte) seedSigner {
	return seedSigner{ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))}
}
func (s seedSigner) pub() ed25519.PublicKey        { return s.priv.Public().(ed25519.PublicKey) }
func (s seedSigner) KeyID() string                 { return nodekey.KeyIDOf(s.pub()) }
func (s seedSigner) Sign(m []byte) ([]byte, error) { return ed25519.Sign(s.priv, m), nil }

var _ nodekey.Signer = seedSigner{}

// memLedger is the coordinator's view of attempts, nodes, revoked keys and consumed signatures.
type memLedger struct {
	mu       sync.Mutex
	attempts map[string]attest.AttemptFacts
	nodes    map[string][]attest.KeyNode
	revoked  map[string]bool
	uses     map[[3]string]attest.Use
}

func (l *memLedger) AttemptFacts(_ context.Context, id string) (attest.AttemptFacts, bool, error) {
	f, ok := l.attempts[id]
	return f, ok, nil
}
func (l *memLedger) NodesByKey(_ context.Context, id string) ([]attest.KeyNode, error) {
	return l.nodes[id], nil
}
func (l *memLedger) Revoked(_ context.Context, id string) (bool, error) { return l.revoked[id], nil }
func (l *memLedger) Replayed(_ context.Context, attempt, key, digest string) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.uses {
		if k[1] == key && k[2] == digest && k[0] != attempt {
			return true, nil
		}
	}
	return false, nil
}
func (l *memLedger) Consume(ctx context.Context, u attest.Use) error {
	if l.revoked[u.KeyID] {
		return attest.ErrRevoked
	}
	if ok, _ := l.Replayed(ctx, u.AttemptID, u.KeyID, u.SigDigest); ok {
		return attest.ErrReplay
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uses[[3]string{u.AttemptID, u.KeyID, u.SigDigest}] = u
	return nil
}

const holderNode, otherNode = "node-1", "node-2"

var keyHolder, keyOther = newSeedSigner(1), newSeedSigner(2)

func newLedger() *memLedger {
	facts := attest.AttemptFacts{RunnerID: holderNode, Revision: goodRevision, RunID: "run-1", RepoID: "repo-1", SourceTrust: "owner", PolicyDigest: "policy-1", SelectionMode: "full", Job: "build"}
	return &memLedger{
		attempts: map[string]attest.AttemptFacts{"att-1": facts, "att-2": facts},
		nodes: map[string][]attest.KeyNode{
			keyHolder.KeyID(): {{NodeID: holderNode, Public: keyHolder.pub(), SigningEligible: true}},
			keyOther.KeyID():  {{NodeID: otherNode, Public: keyOther.pub(), SigningEligible: true}},
		},
		revoked: map[string]bool{}, uses: map[[3]string]attest.Use{},
	}
}

// attemptEvidence is goodEvidence as one attempt of job "build" records it.
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

func signed(t *testing.T, ev model.Evidence, s nodekey.Signer) model.Evidence {
	t.Helper()
	out, err := attest.Sign(ev, s)
	if err != nil {
		t.Fatal(err)
	}
	out.Producer.Kind = "signed-remote"
	return out
}

func doc(t *testing.T, ev model.Evidence) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestForgedEvidenceRefused (I13, TM-26): every forgery of a signed-remote document is refused by
// the production verifier with the reason that names it, and the honest document passes.
func TestForgedEvidenceRefused(t *testing.T) {
	good := signed(t, attemptEvidence("att-1"), keyHolder)
	edited := good
	edited.Result = "fail"
	byOther := signed(t, attemptEvidence("att-1"), keyOther) // a valid, eligible node that did not lease att-1
	replayed := signed(t, attemptEvidence("att-2"), keyHolder)
	replayed.Signature.Value = good.Signature.Value // att-1's signature on att-2's document
	forgedRepo := attemptEvidence("att-1")
	forgedRepo.RepoID = "victim-protected"
	cases := []struct {
		name   string
		ev     model.Evidence
		ledger func(*memLedger)
		first  *model.Evidence // an honest document verified before, which consumes its signature
		reason attest.RemoteReason
	}{
		{"honest", good, nil, nil, ""},
		{"edited after signing", edited, nil, nil, attest.RemoteSignatureInvalid},
		{"signed by a node that did not hold the lease", byOther, nil, nil, attest.RemoteLeaseHolderMismatch},
		{"signature replayed on another attempt", replayed, nil, &good, attest.RemoteReplay},
		{"revoked key", good, func(l *memLedger) { l.revoked["ci-node-"+keyHolder.KeyID()] = true }, nil, attest.RemoteKeyRevoked},
		{"node not eligible to sign", good, func(l *memLedger) { l.nodes[keyHolder.KeyID()][0].SigningEligible = false }, nil, attest.RemoteNodeNotEligible},
		{"unknown key", good, func(l *memLedger) { delete(l.nodes, keyHolder.KeyID()) }, nil, attest.RemoteKeyUnknown},
		{"forged repository", signed(t, forgedRepo, keyHolder), nil, nil, attest.RemoteContextMismatch},
		{"unknown attempt", signed(t, attemptEvidence("att-9"), keyHolder), nil, nil, attest.RemoteAttemptUnknown},
	}
	for _, c := range cases {
		l := newLedger()
		if c.ledger != nil {
			c.ledger(l)
		}
		v := &attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}
		if c.first != nil {
			if err := v.VerifyEvidence(doc(t, *c.first), *c.first); err != nil {
				t.Fatalf("%s: the honest document did not verify first: %v", c.name, err)
			}
		}
		err := v.VerifyEvidence(doc(t, c.ev), c.ev)
		if got := attest.RemoteReasonOf(err); got != c.reason || (c.reason == "" && err != nil) {
			t.Errorf("%s: reason %q (err %v), want %q", c.name, got, err, c.reason)
		}
	}
}

// TestSelfReportedAndUnverifiableEvidenceNeverGates: self-reported evidence passes only a default
// gate whose operator policy admitted it; signed-remote evidence with no verifier never passes.
func TestSelfReportedAndUnverifiableEvidenceNeverGates(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	self := goodEvidence()
	self.Producer.Kind = "self-reported"
	if got := attest.GateAccept(self, goodGate(self)); !hasReason(got, attest.ReasonSelfReportedNotAdmitted) {
		t.Fatalf("self-reported evidence on a default gate that did not admit it: %v", got)
	}
	g := goodGate(self)
	g.Class, g.AllowSelfReported = attest.ClassRelease, true
	if got := attest.GateAccept(self, g); !hasReason(got, attest.ReasonSelfReportedNotAdmitted) {
		t.Fatalf("self-reported evidence on a release gate: %v", got)
	}
	remote := signed(t, attemptEvidence("att-1"), keyHolder)
	if got := attest.GateAcceptDocument(doc(t, remote), goodGate(remote)); !hasReason(got, attest.ReasonSignatureUnverifiable) {
		t.Fatalf("signed-remote evidence with no verifier registered: %v", got)
	}
	// Control: with the production verifier and the honest ledger the same signed document reaches the gate
	// and fails only on what the gate itself judges (the attempt's job is not the whole required set).
	l := newLedger()
	attest.RegisterRemoteVerifier(&attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }})
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	got := attest.GateAcceptDocument(doc(t, remote), goodGate(remote))
	if hasReason(got, attest.ReasonSignatureUnverifiable) {
		t.Fatalf("control: the honest signed document was called unverifiable: %v", got)
	}
}

// TestPublicationProfileOnPublicFixture (TM-36, X15): evidence about a public repository carries no
// private address, host, user, home path or private URL, and a private repository is untouched.
func TestPublicationProfileOnPublicFixture(t *testing.T) {
	ev := goodEvidence()
	ev.Runner.ID = "mac-mini.local"
	ev.Checks[0].Excerpt = "connect to 192.168.1.20:5432 failed\ndial 10.0.0.7 timeout\nrunner mac-mini.local at /Users/synth-user/Sites/nself/x.go\n" +
		"ssh synth-user@build-box.lan\nsee https://ci.corp.local/runs/9?token=abc and https://github.com/nself-org/plugins/actions/runs/9\n" +
		"proxy 172.16.4.4 and fe80::1 and fd12:3456::9 and 127.0.0.1 at C:\\Users\\bob\\work"
	ev.Checks[1].LogRef = strp("/home/runner/work/log.txt")
	ev.Artifacts = []model.Artifact{{Path: "/Users/synth-user/out/app.tar", SHA256: strings.Repeat("a", 64), Size: 1, Committed: true}}
	ev.PlacementRef = strp("node mac-mini.local slot 2")
	out, err := json.Marshal(attest.Publish(ev, attest.Public))
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	for _, private := range []string{"192.168.1.20", "10.0.0.7", "mac-mini", "/Users/synth-user", "synth-user@", "ci.corp.local", "token=abc", "172.16.4.4", "fe80::1", "fd12:3456", "127.0.0.1", "build-box", "/home/runner"} {
		if strings.Contains(text, private) {
			t.Errorf("the published document still holds %q", private)
		}
	}
	if !strings.Contains(text, "https://github.com/nself-org/plugins/actions/runs/9") {
		t.Error("the public run URL was scrubbed along with the private ones")
	}
	// An unknown or empty visibility scrubs too: only the exact value private turns the profile off.
	for _, v := range []attest.Visibility{"", "internal", "PRIVATE"} {
		if attest.Publish(ev, v).Runner.ID == "mac-mini.local" {
			t.Errorf("visibility %q published the raw runner id", v)
		}
	}
	if kept := attest.Publish(ev, attest.Private); kept.Runner.ID != "mac-mini.local" {
		t.Error("a private repository's evidence must be left alone")
	}
}
