//go:build security

package redteam

// Purpose: red team attacks on release promotion and evidence authenticity (TM-17, TM-18): evidence
// that is hollow but "passing", a forged coordinator-recorded document handed to the verifier,
// signatures made with the wrong purpose or by the wrong node, duplicate-key parser differentials.
// Inputs:  model.Evidence fixtures, the real authz.Promotable, attest.Verify and attest.RunnerVerifier,
//          evidence.VerifyDocument. Outputs: pass or fail; a pass means the attack failed.
// Constraints: in-memory ledger fake as fixture only; seeded throwaway keys.

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

const (
	relRev    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	relDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

func relEvidence() model.Evidence {
	return model.Evidence{
		Schema: "ci.evidence/v1", Revision: relRev, RepoID: "repo-1", RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "policy-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("1", 64)}}},
		Runner:       model.RunnerRef{ID: "mac-mini.local", Provider: "local", Platform: "darwin/arm64", Isolation: "process"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"},
		ToolVersions: map[string]string{"go": "1.27.2"},
		Checks: []model.Check{
			{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"},
			{ID: "lint", JobID: "lint", Kind: "quality", Tool: "golangci-lint", Required: true, Substantive: true, Result: "pass"}},
		Artifacts: []model.Artifact{{Path: "dist/app", SHA256: relDigest, Size: 10, Committed: true}}, Retries: []model.Retry{},
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"},
		Result:    "pass", Counts: model.Counts{Pass: 2, Total: 2},
		Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"},
	}
}

func relGate(ev model.Evidence) attest.Gate {
	cur := ev.InputDigests
	return attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build", "lint"},
		MinTrust: "internal", Class: attest.ClassRelease, FullSelection: true, Current: &cur}
}

func TestB_Release_ControlPromotes(t *testing.T) {
	ev := relEvidence()
	ok, why := authz.Promotable(authz.Artifact{Project: "repo-1", Revision: relRev, SHA256: relDigest}, ev, relGate(ev))
	if !ok {
		t.Fatalf("control: good evidence refused: %v", why)
	}
}

// Evidence that satisfies the field checks but proves nothing must not release.
func TestB_Release_HollowEvidenceNeverPromotes(t *testing.T) {
	mut := map[string]func(e *model.Evidence){
		"non-substantive-checks": func(e *model.Evidence) { e.Checks[0].Substantive, e.Checks[1].Substantive = false, false },
		"extra-failing-job": func(e *model.Evidence) {
			e.Checks = append(e.Checks, model.Check{ID: "x", JobID: "extra", Kind: "test", Tool: "go", Result: "fail"})
		},
		"extra-skipped-required": func(e *model.Evidence) { e.Checks[1].Result = "skip" },
		"counts-lie":             func(e *model.Evidence) { e.Counts = model.Counts{Pass: 5, Total: 5} },
		"zero-counts":            func(e *model.Evidence) { e.Counts = model.Counts{} },
		"adhoc-pipeline":         func(e *model.Evidence) { e.Pipeline.Kind = "adhoc" },
		"drill-pipeline":         func(e *model.Evidence) { e.Pipeline.Kind = "drill" },
		"dirty-tree":             func(e *model.Evidence) { e.WorktreeState = "dirty" },
		"unbound":                func(e *model.Evidence) { e.Binding.State = "unbound" },
		"untrusted-source":       func(e *model.Evidence) { e.SourceTrust = "untrusted" },
		"unknown-trust":          func(e *model.Evidence) { e.SourceTrust = "Owner" },
		"upper-revision":         func(e *model.Evidence) { e.Revision = strings.ToUpper(relRev) },
		"short-revision":         func(e *model.Evidence) { e.Revision = "aaaa" },
		"empty-checks":           func(e *model.Evidence) { e.Checks = nil },
		"duplicate-job-fail": func(e *model.Evidence) {
			e.Checks = append(e.Checks, model.Check{ID: "t2", JobID: "build", Kind: "test", Tool: "go", Result: "fail"})
		},
		"unknown-result": func(e *model.Evidence) { e.Checks[0].Result = "passed" },
		"result-case":    func(e *model.Evidence) { e.Result = "PASS" },
		"other-schema":   func(e *model.Evidence) { e.Schema = "ci.evidence/v2" },
		"stale-self-label": func(e *model.Evidence) {
			e.Freshness = model.Freshness{State: "current"}
			e.InputDigests.Config = strings.Repeat("e", 64)
		},
		"uncommitted-artifact": func(e *model.Evidence) { e.Artifacts[0].Committed = false },
		"empty-producer":       func(e *model.Evidence) { e.Producer.Kind = "" },
		"unknown-producer":     func(e *model.Evidence) { e.Producer.Kind = "Coordinator-Recorded" },
	}
	for name, f := range mut {
		ev := relEvidence()
		f(&ev)
		if ok, _ := authz.Promotable(authz.Artifact{Project: "repo-1", Revision: relRev, SHA256: relDigest}, ev, relGate(relEvidence())); ok {
			t.Errorf("%s: hollow or inconsistent evidence was promoted to a release", name)
		}
	}
}

// A document that merely claims coordinator-recorded must not authenticate on a machine that
// holds no record of the run.
func TestB_Release_ForgedCoordinatorRecordedDocument(t *testing.T) {
	ev := relEvidence()
	doc, _ := json.Marshal(ev)
	cur := ev.InputDigests
	v, err := evidence.VerifyDocument(doc, relRev, &cur, nil)
	if err == nil && v.Exit == evidence.ExitOK {
		t.Errorf("forged document with no local record verified OK (exit %d)", v.Exit)
	}
	// a recorded function that answers for the run with a different stored document: edit detected
	stored := relEvidence()
	stored.Result = "fail"
	v, _ = evidence.VerifyDocument(doc, relRev, &cur, func(model.Evidence) *model.Evidence { return &stored })
	if v.Exit == evidence.ExitOK {
		t.Errorf("document that differs from the recorded one verified OK")
	}
	// a forged signed-remote claim with a bogus signature and no verifier registered
	forged := relEvidence()
	forged.Producer.Kind = "signed-remote"
	k, val := "ci-node-0123456789abcdef", "AAAA"
	forged.Signature = model.Signature{Kind: "runner-ed25519", KeyID: &k, Value: &val}
	fd, _ := json.Marshal(forged)
	if v, err = evidence.VerifyDocument(fd, relRev, &cur, nil); err == nil && v.Exit == evidence.ExitOK {
		t.Errorf("forged signed-remote document verified OK")
	}
	if _, err = attest.Verify(fd); err == nil {
		t.Errorf("attest.Verify accepted a signed-remote document with a junk signature")
	}
}

func TestB_Release_DuplicateKeysAndCaseDifferentials(t *testing.T) {
	ev := relEvidence()
	ev.Result = "fail"
	good, _ := json.Marshal(ev)
	for name, doc := range map[string][]byte{
		"dup-result":          bytes.Replace(good, []byte(`"result":"fail"`), []byte(`"result":"fail","result":"pass"`), 1),
		"dup-nested":          bytes.Replace(good, []byte(`"state":"bound"`), []byte(`"state":"unbound","state":"bound"`), 1),
		"case-variant-result": bytes.Replace(good, []byte(`"result":"fail",`), []byte(`"result":"fail","RESULT":"pass",`), 1),
		"escaped-key":         bytes.Replace(good, []byte(`"result":"fail",`), []byte(`"result":"fail","result":"pass",`), 1),
	} {
		if bytes.Equal(doc, good) {
			t.Fatalf("%s: fixture did not change the document", name)
		}
		got, err := attest.Verify(doc)
		if err == nil && got.Result == "pass" {
			t.Errorf("%s: a document stored as fail decoded as pass", name)
		}
	}
}

// ---- TM-18: runner verifier misuse ----

type relSigner struct{ priv ed25519.PrivateKey }

func (s relSigner) pub() ed25519.PublicKey        { return s.priv.Public().(ed25519.PublicKey) }
func (s relSigner) KeyID() string                 { return nodekey.KeyIDOf(s.pub()) }
func (s relSigner) Sign(m []byte) ([]byte, error) { return ed25519.Sign(s.priv, m), nil }

var _ nodekey.Signer = relSigner{}

type relLedger struct {
	facts   attest.AttemptFacts
	nodes   []attest.KeyNode
	revoked bool
}

func (l *relLedger) AttemptFacts(context.Context, string) (attest.AttemptFacts, bool, error) {
	return l.facts, true, nil
}
func (l *relLedger) NodesByKey(context.Context, string) ([]attest.KeyNode, error) {
	return l.nodes, nil
}
func (l *relLedger) Revoked(context.Context, string) (bool, error) { return l.revoked, nil }
func (l *relLedger) Replayed(context.Context, string, string, string) (bool, error) {
	return false, nil
}
func (l *relLedger) Consume(context.Context, attest.Use) error { return nil }

func TestB_Release_SignatureMisuse(t *testing.T) {
	seed := func(b byte) relSigner {
		return relSigner{ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, ed25519.SeedSize))}
	}
	holder, intruder := seed(7), seed(8)
	ev := relEvidence()
	ev.Attempt = &model.AttemptRef{ID: "att-1", N: 1}
	ev.Job = strp("build")
	for i := range ev.Checks {
		ev.Checks[i].JobID = "build"
	}
	facts := attest.AttemptFacts{RunnerID: "node-1", Revision: relRev, RunID: "run-1", RepoID: "repo-1", SourceTrust: "owner", PolicyDigest: "policy-1", SelectionMode: "full", Job: "build"}
	ledger := func(mod func(l *relLedger)) *relLedger {
		l := &relLedger{facts: facts, nodes: []attest.KeyNode{{NodeID: "node-1", Public: holder.pub(), SigningEligible: true}}}
		if mod != nil {
			mod(l)
		}
		return l
	}
	verify := func(l *relLedger, doc model.Evidence) error {
		raw, _ := json.Marshal(doc)
		return (&attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}).VerifyEvidence(raw, doc)
	}
	signed, err := attest.Sign(ev, holder)
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(ledger(nil), signed); err != nil {
		t.Fatalf("control: the lease holder's signature was refused: %v", err)
	}
	attacks := map[string]struct {
		l   *relLedger
		doc func() model.Evidence
	}{
		"revoked-key":      {ledger(func(l *relLedger) { l.revoked = true }), func() model.Evidence { return signed }},
		"not-eligible":     {ledger(func(l *relLedger) { l.nodes[0].SigningEligible = false }), func() model.Evidence { return signed }},
		"other-node-holds": {ledger(func(l *relLedger) { l.facts.RunnerID = "node-9" }), func() model.Evidence { return signed }},
		"intruder-pinned-as-holder": {ledger(func(l *relLedger) {
			l.nodes = []attest.KeyNode{{NodeID: "node-1", Public: intruder.pub(), SigningEligible: true}}
		}), func() model.Evidence { return signed }},
		"wrong-job-doc":              {ledger(func(l *relLedger) { l.facts.Job = "lint" }), func() model.Evidence { return signed }},
		"field-edited-after-signing": {ledger(nil), func() model.Evidence { e := signed; e.Result = "fail"; e.Result = "pass"; e.Counts.Pass++; return e }},
		"purpose-release": {ledger(nil), func() model.Evidence {
			e := signed
			id := "ci-release-" + holder.KeyID()
			e.Signature.KeyID = &id
			return e
		}},
		"purpose-agent": {ledger(nil), func() model.Evidence { e := signed; id := "agent-" + holder.KeyID(); e.Signature.KeyID = &id; return e }},
		"purpose-case": {ledger(nil), func() model.Evidence {
			e := signed
			id := "CI-NODE-" + holder.KeyID()
			e.Signature.KeyID = &id
			return e
		}},
		"purpose-double": {ledger(nil), func() model.Evidence {
			e := signed
			id := "ci-node-ci-release-" + holder.KeyID()
			e.Signature.KeyID = &id
			return e
		}},
		"key-id-padded": {ledger(nil), func() model.Evidence {
			e := signed
			id := *signed.Signature.KeyID + " "
			e.Signature.KeyID = &id
			return e
		}},
		"signed-by-intruder": {ledger(nil), func() model.Evidence {
			e, _ := attest.Sign(ev, intruder)
			id := "ci-node-" + holder.KeyID()
			e.Signature.KeyID = &id
			return e
		}},
	}
	for name, a := range attacks {
		doc := a.doc()
		if name == "field-edited-after-signing" {
			doc.Checks = append([]model.Check(nil), doc.Checks...)
			doc.Checks[0].Excerpt = "edited"
		}
		err := verify(a.l, doc)
		if err == nil {
			t.Errorf("%s: the forged or misused signature verified", name)
			continue
		}
		var re *attest.RemoteError
		if !errors.As(err, &re) {
			t.Errorf("%s: refusal is not a typed E686: %v", name, err)
		}
	}
}

func strp(s string) *string { return &s }
