//go:build security

package redteam

// Purpose: red team group C, evidence (TM-26 forged or replayed evidence, TM-27 stale or unbound
// evidence, TM-28 hosted evidence). The attacker edits the evidence document at the byte level
// (escaped duplicate keys, key case, trailing data), swaps key ids and signature encodings, makes
// the ledger fail, and bends every field a gate compares. Every test calls the production verifier
// and gate and asserts the SECURE outcome.
// Inputs:  synthetic documents, throwaway Ed25519 seeds, an in-memory attest.Ledger.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: no network, no real key; the ledger is the only fake (it is the coordinator store).

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/agent/nodekey"
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// cBS is a backslash: unicode escapes are built in code, never typed in this file.
const cBS = "\\"

const cRev = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type cSigner struct{ priv ed25519.PrivateKey }

func cSignerOf(b byte) cSigner                  { return cSigner{ed25519.NewKeyFromSeed(bytes.Repeat([]byte{b}, 32))} }
func (s cSigner) pub() ed25519.PublicKey        { return s.priv.Public().(ed25519.PublicKey) }
func (s cSigner) KeyID() string                 { return nodekey.KeyIDOf(s.pub()) }
func (s cSigner) Sign(m []byte) ([]byte, error) { return ed25519.Sign(s.priv, m), nil }

// cLedger is a configurable coordinator view; errAt names the method that fails.
type cLedger struct {
	mu      sync.Mutex
	facts   map[string]attest.AttemptFacts
	nodes   map[string][]attest.KeyNode
	revoked map[string]bool
	uses    map[[3]string]bool
	errAt   string
}

var errCLedger = errors.New("ledger down")

func (l *cLedger) AttemptFacts(_ context.Context, id string) (attest.AttemptFacts, bool, error) {
	if l.errAt == "facts" {
		return attest.AttemptFacts{}, false, errCLedger
	}
	f, ok := l.facts[id]
	return f, ok, nil
}
func (l *cLedger) NodesByKey(_ context.Context, id string) ([]attest.KeyNode, error) {
	if l.errAt == "nodes" {
		return nil, errCLedger
	}
	return l.nodes[id], nil
}
func (l *cLedger) Revoked(_ context.Context, id string) (bool, error) {
	if l.errAt == "revoked" {
		return false, errCLedger
	}
	return l.revoked[id], nil
}
func (l *cLedger) Replayed(_ context.Context, att, key, dig string) (bool, error) {
	if l.errAt == "replayed" {
		return false, errCLedger
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for k := range l.uses {
		if k[1] == key && k[2] == dig && k[0] != att {
			return true, nil
		}
	}
	return false, nil
}
func (l *cLedger) Consume(ctx context.Context, u attest.Use) error {
	if l.errAt == "consume" {
		return errCLedger
	}
	if ok, _ := l.Replayed(ctx, u.AttemptID, u.KeyID, u.SigDigest); ok {
		return attest.ErrReplay
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.uses[[3]string{u.AttemptID, u.KeyID, u.SigDigest}] = true
	return nil
}

var cHolder, cOther = cSignerOf(11), cSignerOf(12)

func cNewLedger() *cLedger {
	f := attest.AttemptFacts{RunnerID: "node-h", Revision: cRev, RunID: "run-1", RepoID: "repo-1", SourceTrust: "owner", PolicyDigest: "pd-1", SelectionMode: "full", Job: "build"}
	return &cLedger{facts: map[string]attest.AttemptFacts{"att-1": f},
		nodes: map[string][]attest.KeyNode{
			cHolder.KeyID(): {{NodeID: "node-h", Public: cHolder.pub(), SigningEligible: true}},
			cOther.KeyID():  {{NodeID: "node-o", Public: cOther.pub(), SigningEligible: true}}},
		revoked: map[string]bool{}, uses: map[[3]string]bool{}}
}

func cEvidence() model.Evidence {
	chk := []model.Check{{ID: "vet", JobID: "build", Kind: "quality", Tool: "go", Required: true, Substantive: true, Result: "pass"},
		{ID: "test", JobID: "build", Kind: "test", Tool: "go", Required: true, Substantive: true, Result: "pass"}}
	job := "build"
	return model.Evidence{Schema: "ci.evidence/v1", Revision: cRev, RepoID: "repo-1", RunID: "run-1",
		Pipeline: model.PipelineRef{ID: "p1", Name: "default", Kind: "default"}, Trigger: "local", SourceTrust: "owner",
		PolicyDigest: "pd-1", WorktreeState: "clean", Binding: model.Binding{State: "bound"},
		InputDigests: model.InputDigests{Config: strings.Repeat("c", 64), Lockfiles: []model.DigestRef{}, Generated: []model.DigestRef{}},
		Runner:       model.RunnerRef{ID: "node-h", Provider: "agent", Platform: "linux/amd64", Isolation: "vm"},
		Producer:     model.Producer{Kind: "coordinator-recorded", Version: "1"}, ToolVersions: map[string]string{"go": "1.27"},
		Checks: chk, Artifacts: []model.Artifact{}, Retries: []model.Retry{}, Attempt: &model.AttemptRef{ID: "att-1", N: 1}, Job: &job,
		Selection: model.Selection{Mode: "full", Reason: "selection.full_required", Digest: "d"}, Result: "pass",
		Counts: model.Counts{Pass: 2, Total: 2}, Freshness: model.Freshness{State: "current"}, Signature: model.Signature{Kind: "coordinator-recorded"}}
}

func cGate(ev model.Evidence) attest.Gate {
	cur := ev.InputDigests
	return attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build"}, MinTrust: "internal", Class: attest.ClassDefault, Current: &cur}
}

func cSign(t *testing.T, ev model.Evidence, s nodekey.Signer) model.Evidence {
	t.Helper()
	out, err := attest.Sign(ev, s)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func cDoc(t *testing.T, ev model.Evidence) []byte {
	t.Helper()
	b, err := json.Marshal(ev)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestC26ByteLevelDifferentials: the same document read two ways must not exist. A repeated key written
// with a unicode escape, a repeated key nested in a check, a key that differs only in case and trailing
// data after the object are each refused (the first-wins reader of another implementation would see a
// different verdict than the last-wins Go reader).
func TestC26ByteLevelDifferentials(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	good := cDoc(t, cEvidence())
	if got := attest.GateAcceptDocument(good, cGate(cEvidence())); len(got) != 0 {
		t.Fatalf("control: the honest coordinator-recorded document was refused: %v", got)
	}
	s := string(good)
	rows := map[string]string{
		"escaped duplicate top-level key": strings.Replace(s, `"result":"pass"`, `"result":"fail","`+cBS+`u0072esult":"pass"`, 1),
		"duplicate key in a check":        strings.Replace(s, `"tool":"go"`, `"tool":"go","t`+cBS+`u006fol":"x"`, 1),
		"key differing only in case":      strings.Replace(s, `"result":"pass"`, `"result":"fail","RESULT":"pass"`, 1),
		"trailing data":                   s + ` {"result":"pass"}`,
		"leading BOM":                     "\xef\xbb\xbf" + s,
	}
	for name, doc := range rows {
		if doc == s {
			t.Fatalf("%s: the mutation did not apply", name)
		}
		if got := attest.GateAcceptDocument([]byte(doc), cGate(cEvidence())); len(got) == 0 {
			t.Errorf("%s: the gate accepted the mutated document", name)
		}
	}
}

// TestC26SigningPayloadNeverCoversTwoDocuments: unicode-escaped, reordered and re-spaced renderings of
// one document sign to the same payload (canonical form), while a float, an exponent and a duplicate
// key have no payload at all.
func TestC26SigningPayloadNeverCoversTwoDocuments(t *testing.T) {
	ev := cEvidence()
	doc := cDoc(t, ev)
	base, err := attest.SigningPayload(doc)
	if err != nil {
		t.Fatal(err)
	}
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(doc, &obj)
	var b strings.Builder
	b.WriteString("{\n")
	first := true
	for k, v := range obj { // map order is random: field reordering
		if !first {
			b.WriteString(",\n")
		}
		first = false
		b.WriteString("  " + `"` + strings.ReplaceAll(k, "a", `a`) + `": ` + string(v))
	}
	b.WriteString("\n}")
	again, err := attest.SigningPayload([]byte(b.String()))
	if err != nil || !bytes.Equal(again, base) {
		t.Errorf("reordered/escaped rendering: err %v, equal=%v", err, bytes.Equal(again, base))
	}
	for name, d := range map[string]string{
		"float counts":    strings.Replace(string(doc), `"pass":2`, `"pass":2.0`, 1),
		"exponent counts": strings.Replace(string(doc), `"pass":2`, `"pass":2e0`, 1),
		"duplicate":       strings.Replace(string(doc), `"result":"pass"`, `"result":"pass","result":"fail"`, 1),
	} {
		if p, err := attest.SigningPayload([]byte(d)); err == nil {
			t.Errorf("%s: a payload exists (%d bytes)", name, len(p))
		}
	}
}

// TestC26KeyIDAndSignatureForms: a valid signature behind a re-labelled key id (other purpose, upper
// case hex, trailing space, long id), a truncated or over-long signature and padded or tail-bit
// variants of the signature text are all refused.
func TestC26KeyIDAndSignatureForms(t *testing.T) {
	good := cSign(t, cEvidence(), cHolder)
	verify := func(ev model.Evidence) error {
		l := cNewLedger()
		return (&attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}).VerifyEvidence(cDoc(t, ev), ev)
	}
	if err := verify(good); err != nil {
		t.Fatalf("control: %v", err)
	}
	id := *good.Signature.KeyID
	hexPart := strings.TrimPrefix(id, "ci-node-")
	ids := []string{"ci-agent-" + hexPart, "release-" + hexPart, "ci-node-" + strings.ToUpper(hexPart), id + " ", " " + id, id + "0", "ci-node-" + hexPart[:15], "CI-NODE-" + hexPart, "ci-node-ci-node-" + hexPart}
	for _, k := range ids {
		ev := good
		ev.Signature.KeyID = &k
		if err := verify(ev); err == nil {
			t.Errorf("key id %q was accepted", k)
		}
	}
	raw, _ := base64.RawStdEncoding.DecodeString(*good.Signature.Value)
	vals := map[string]string{
		"truncated 63 bytes": base64.RawStdEncoding.EncodeToString(raw[:63]),
		"extended 65 bytes":  base64.RawStdEncoding.EncodeToString(append(append([]byte{}, raw...), 0)),
		"url alphabet":       base64.RawURLEncoding.EncodeToString(raw) + "x",
		"double padded":      base64.StdEncoding.EncodeToString(raw) + "=",
		"all zero":           base64.RawStdEncoding.EncodeToString(make([]byte, 64)),
	}
	for name, v := range vals {
		ev := good
		ev.Signature.Value = &v
		if err := verify(ev); err == nil {
			t.Errorf("signature %s was accepted", name)
		}
	}
}

// TestC26LedgerFailureFailsClosed: every ledger method that errors refuses the evidence, and a
// revocation or a node removal between the lookup and the consume is not a pass.
func TestC26LedgerFailureFailsClosed(t *testing.T) {
	ev := cSign(t, cEvidence(), cHolder)
	for _, at := range []string{"facts", "nodes", "revoked", "replayed", "consume"} {
		l := cNewLedger()
		l.errAt = at
		v := &attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}
		if err := v.VerifyEvidence(cDoc(t, ev), ev); err == nil {
			t.Errorf("ledger error in %s: evidence accepted", at)
		}
	}
	var nilLedger *attest.RunnerVerifier
	if err := nilLedger.VerifyEvidence(cDoc(t, ev), ev); err == nil {
		t.Error("a nil verifier accepted evidence")
	}
	l := cNewLedger() // an empty runner id on both sides must not read as "the same node"
	f := l.facts["att-1"]
	f.RunnerID = ""
	l.facts["att-1"] = f
	l.nodes[cHolder.KeyID()][0].NodeID = ""
	v := &attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}
	if err := v.VerifyEvidence(cDoc(t, ev), ev); err == nil {
		t.Error("an empty lease holder matched an empty node id")
	}
}
