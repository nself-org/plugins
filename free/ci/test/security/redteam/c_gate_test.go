//go:build security

package redteam

// Purpose: red team group C, gate acceptance and hosted evidence (TM-27, TM-28), second half of the evidence group.
// Inputs:  see the sibling file of the same group.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: local fixtures only; see c_evidence_test.go header.

import (
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"strings"
	"testing"
)

// TestC27GateFieldVariants: every comparison of the gate refuses a case, whitespace, prefix or enum
// variant of the value it expects.
func TestC27GateFieldVariants(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	rows := map[string]func(*model.Evidence, *attest.Gate){
		"revision upper case":      func(e *model.Evidence, g *attest.Gate) { e.Revision = strings.ToUpper(cRev[:39]) + "A" },
		"revision trailing space":  func(e *model.Evidence, g *attest.Gate) { e.Revision = cRev + " " },
		"revision short prefix":    func(e *model.Evidence, g *attest.Gate) { e.Revision = cRev[:12] },
		"gate revision prefix":     func(e *model.Evidence, g *attest.Gate) { g.Revision = cRev[:12] },
		"policy digest case":       func(e *model.Evidence, g *attest.Gate) { e.PolicyDigest = "PD-1" },
		"policy digest empty both": func(e *model.Evidence, g *attest.Gate) { e.PolicyDigest, g.PolicyDigest = "", "" },
		"repo empty both":          func(e *model.Evidence, g *attest.Gate) { e.RepoID, g.RepoID = "", "" },
		"trust upper case":         func(e *model.Evidence, g *attest.Gate) { e.SourceTrust = "OWNER" },
		"trust empty":              func(e *model.Evidence, g *attest.Gate) { e.SourceTrust = "" },
		"gate min trust empty":     func(e *model.Evidence, g *attest.Gate) { g.MinTrust = "" },
		"gate min trust unknown":   func(e *model.Evidence, g *attest.Gate) { g.MinTrust = "everyone" },
		"required job case":        func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = []string{"Build"} },
		"required job padded":      func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = []string{"build "} },
		"required job empty name":  func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = []string{""} },
		"no required jobs":         func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = nil },
		"pipeline kind adhoc":      func(e *model.Evidence, g *attest.Gate) { e.Pipeline.Kind = "adhoc" },
		"pipeline kind drill":      func(e *model.Evidence, g *attest.Gate) { e.Pipeline.Kind = "drill" },
		"dirty worktree bound":     func(e *model.Evidence, g *attest.Gate) { e.WorktreeState = "dirty" },
		"sha override unbound": func(e *model.Evidence, g *attest.Gate) {
			e.Binding = model.Binding{State: "unbound", Reason: cReason("sha")}
		},
		"partial selection on rel": func(e *model.Evidence, g *attest.Gate) { e.Selection.Mode = "affected"; g.Class = attest.ClassRelease },
		"unknown gate class":       func(e *model.Evidence, g *attest.Gate) { e.Selection.Mode = "affected"; g.Class = "Release" },
		"freshness field claims":   func(e *model.Evidence, g *attest.Gate) { g.Current = nil },
	}
	for name, bend := range rows {
		ev := cEvidence()
		g := cGate(ev)
		bend(&ev, &g)
		if got := attest.GateAccept(ev, g); len(got) == 0 {
			t.Errorf("%s: the gate accepted", name)
		}
	}
}

func cStrp(s string) *string { return &s }

// TestC28SelfReportedNeverReleases: hosted (self-reported) evidence cannot pass a release gate, an
// unknown-class gate or a gate whose class has other casing, even with AllowSelfReported set, and a
// self-reported document with a forged coordinator-recorded signature kind is inconsistent.
func TestC28SelfReportedNeverReleases(t *testing.T) {
	attest.RegisterRemoteVerifier(nil)
	for _, class := range []attest.GateClass{attest.ClassRelease, "", "Release", "release ", "default ", "DEFAULT", "hosted"} {
		ev := cEvidence()
		ev.Producer.Kind = "self-reported"
		g := cGate(ev)
		g.Class, g.AllowSelfReported = class, true
		if got := attest.GateAccept(ev, g); len(got) == 0 {
			t.Errorf("class %q: self-reported evidence passed", class)
		}
	}
	ev := cEvidence()
	ev.Producer.Kind, ev.Signature = "self-reported", model.Signature{Kind: "runner-ed25519", KeyID: cStrp("ci-node-" + cHolder.KeyID()), Value: cStrp("AAAA")}
	g := cGate(ev)
	g.AllowSelfReported = true
	if got := attest.GateAccept(ev, g); len(got) == 0 {
		t.Error("self-reported evidence carrying a runner signature kind passed")
	}
}

func cReason(s string) *model.BindingReason { r := model.BindingReason(s); return &r }

// TestC26SignatureTextHasOneForm (C01): the verifier documents a strict signature text, but the base64
// decoder skips CR and LF, so one signature has many accepted spellings. Replay is unaffected (the
// replay set keys the raw bytes), the stored text of an evidence document is.
func TestC26SignatureTextHasOneForm(t *testing.T) {
	if !openFinding(t, "C01") {
		return
	}
	good := cSign(t, cEvidence(), cHolder)
	for name, v := range map[string]string{"trailing LF": *good.Signature.Value + "\n", "inner CRLF": (*good.Signature.Value)[:20] + "\r\n" + (*good.Signature.Value)[20:]} {
		ev := good
		ev.Signature.Value = &v
		l := cNewLedger()
		if err := (&attest.RunnerVerifier{Ledger: func() attest.Ledger { return l }}).VerifyEvidence(cDoc(t, ev), ev); err == nil {
			t.Errorf("%s: a second spelling of the signature verified", name)
		}
	}
}
