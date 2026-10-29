package attest_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestGateAccept_AcceptsCurrentCoordinatorEvidence(t *testing.T) {
	ev := goodEvidence()
	if got := attest.GateAccept(ev, goodGate(ev)); len(got) != 0 {
		t.Fatalf("good evidence rejected: %v", got)
	}
}

// Each row bends one thing and expects exactly one reason: the reasons are distinct.
func TestGateAccept_DistinctReasons(t *testing.T) {
	rows := []struct {
		name string
		bend func(*model.Evidence, *attest.Gate)
		want attest.Reason
	}{
		{"repo", func(e *model.Evidence, g *attest.Gate) { g.RepoID = "other" }, attest.ReasonRepoMismatch},
		{"revision", func(e *model.Evidence, g *attest.Gate) { g.Revision = strings.Repeat("b", 40) }, attest.ReasonRevisionMismatch},
		{"policy", func(e *model.Evidence, g *attest.Gate) { g.PolicyDigest = "policy-2" }, attest.ReasonPolicyMismatch},
		{"job removed", func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = []string{"build", "lint", "deploy-check"} }, attest.ReasonJobsMismatch},
		{"no required jobs", func(e *model.Evidence, g *attest.Gate) { g.RequiredJobs = nil }, attest.ReasonJobsMismatch},
		{"trust", func(e *model.Evidence, g *attest.Gate) { e.SourceTrust = "collaborator" }, attest.ReasonTrustBelowMin},
		{"adhoc", func(e *model.Evidence, g *attest.Gate) { e.Pipeline.Kind = "adhoc" }, attest.ReasonAdhoc},
		{"drill", func(e *model.Evidence, g *attest.Gate) { e.Pipeline.Kind = "drill" }, attest.ReasonAdhoc},
		{"stale", func(e *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Config = strings.Repeat("d", 64)
			g.Current = &c
		}, attest.ReasonStale},
		{"unknown freshness", func(e *model.Evidence, g *attest.Gate) { g.Current = nil }, attest.ReasonStale},
		{"partial release", func(e *model.Evidence, g *attest.Gate) { e.Selection.Mode = "affected"; g.Class = attest.ClassRelease }, attest.ReasonSelectionPartial},
		{"partial protected", func(e *model.Evidence, g *attest.Gate) { e.Selection.Mode = "affected"; g.FullSelection = true }, attest.ReasonSelectionPartial},
		{"self reported default not admitted", func(e *model.Evidence, g *attest.Gate) { e.Producer.Kind = "self-reported" }, attest.ReasonSelfReportedNotAdmitted},
		{"self reported release", func(e *model.Evidence, g *attest.Gate) {
			e.Producer.Kind = "self-reported"
			g.Class, g.AllowSelfReported = attest.ClassRelease, true
		}, attest.ReasonSelfReportedNotAdmitted},
		{"signed remote no verifier", func(e *model.Evidence, g *attest.Gate) { *e = signedRemote(*e) }, attest.ReasonSignatureUnverifiable},
		{"failed job", func(e *model.Evidence, g *attest.Gate) {
			e.Checks[2].Result, e.Checks[2].Required = "fail", false
			e.Counts = model.Counts{Pass: 2, Fail: 1, Total: 3}
		}, attest.ReasonNotPassing},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			attest.RegisterRemoteVerifier(nil)
			ev := goodEvidence()
			g := goodGate(ev)
			row.bend(&ev, &g)
			got := attest.GateAccept(ev, g)
			if !sameReasons(got, []attest.Reason{row.want}) {
				t.Fatalf("got %v, want [%s]", got, row.want)
			}
		})
	}
}

func TestGateAccept_UnboundAndDirty(t *testing.T) {
	ev := goodEvidence()
	reason := model.BindingReason("dirty")
	ev.WorktreeState, ev.Binding = "dirty", model.Binding{State: "unbound", Reason: &reason}
	got := attest.GateAccept(ev, goodGate(ev))
	if !sameReasons(got, []attest.Reason{attest.ReasonUnbound}) {
		t.Fatalf("got %v", got)
	}
	// bound but dirty is a lie, not a gate input
	ev = goodEvidence()
	ev.WorktreeState = "dirty"
	if got = attest.GateAccept(ev, goodGate(ev)); !hasReason(got, attest.ReasonInvalid) || !hasReason(got, attest.ReasonUnbound) {
		t.Fatalf("dirty+bound: %v", got)
	}
}

// Unbound for a reason other than a dirty tree (the revision did not match the
// checkout) is still unbound.
func TestGateAccept_UnboundShaMismatchOnCleanTree(t *testing.T) {
	ev := goodEvidence()
	reason := model.BindingReason("sha_mismatch")
	ev.Binding = model.Binding{State: "unbound", Reason: &reason}
	ev.Freshness = model.Freshness{State: "unknown"}
	if got := attest.GateAccept(ev, goodGate(ev)); !sameReasons(got, []attest.Reason{attest.ReasonUnbound}) {
		t.Fatalf("got %v", got)
	}
}

func TestGateAccept_SelfReportedAdmittedOnlyByPolicyAndNeverRelease(t *testing.T) {
	ev := goodEvidence()
	ev.Producer.Kind = "self-reported"
	g := goodGate(ev)
	g.AllowSelfReported = true
	if got := attest.GateAccept(ev, g); len(got) != 0 {
		t.Fatalf("admitted by policy: %v", got)
	}
	g.Class = attest.ClassRelease
	if got := attest.GateAccept(ev, g); !hasReason(got, attest.ReasonSelfReportedNotAdmitted) {
		t.Fatalf("release gate admitted self-reported: %v", got)
	}
}

// FullSelection on a default gate demands full selection only; it does not
// make the gate refuse self-reported evidence (that is release-class only).
func TestGateAccept_FullSelectionDefaultGateStillAdmitsSelfReported(t *testing.T) {
	ev := goodEvidence()
	ev.Producer.Kind = "self-reported"
	g := goodGate(ev)
	g.AllowSelfReported, g.FullSelection = true, true
	if got := attest.GateAccept(ev, g); len(got) != 0 {
		t.Fatalf("default gate with FullSelection refused full self-reported evidence: %v", got)
	}
	ev.Selection.Mode = "affected"
	if got := attest.GateAccept(ev, g); !hasReason(got, attest.ReasonSelectionPartial) {
		t.Fatalf("partial selection accepted by FullSelection gate: %v", got)
	}
}

func TestGateAccept_UnknownClassIsRelease(t *testing.T) {
	ev := goodEvidence()
	ev.Selection.Mode = "affected"
	g := goodGate(ev)
	g.Class = "weird"
	if got := attest.GateAccept(ev, g); !hasReason(got, attest.ReasonSelectionPartial) {
		t.Fatalf("unknown class treated as default: %v", got)
	}
}

func TestGateAccept_ZeroGateAcceptsNothing(t *testing.T) {
	if got := attest.GateAccept(goodEvidence(), attest.Gate{}); len(got) == 0 {
		t.Fatal("zero gate accepted evidence")
	}
	if got := attest.GateAccept(model.Evidence{}, attest.Gate{}); len(got) == 0 {
		t.Fatal("zero gate accepted zero evidence")
	}
}

func TestGateAccept_EmptyValuesNeverMatch(t *testing.T) {
	ev := goodEvidence()
	ev.RepoID, ev.PolicyDigest = "", ""
	g := goodGate(ev)
	g.RepoID, g.PolicyDigest = "", ""
	got := attest.GateAccept(ev, g)
	if !hasReason(got, attest.ReasonRepoMismatch) || !hasReason(got, attest.ReasonPolicyMismatch) {
		t.Fatalf("empty matched empty: %v", got)
	}
}

func TestGateAccept_TrustRankAndUnknownTrust(t *testing.T) {
	for _, row := range []struct {
		have, min model.TrustClass
		ok        bool
	}{
		{"owner", "owner", true}, {"owner", "internal", true}, {"internal", "internal", true}, {"internal", "owner", false},
		{"collaborator", "internal", false}, {"untrusted", "untrusted", true}, {"untrusted", "collaborator", false},
		{"bogus", "untrusted", false}, {"owner", "bogus", false}, {"owner", "", false},
	} {
		ev := goodEvidence()
		ev.SourceTrust = row.have
		g := goodGate(ev)
		g.MinTrust = row.min
		got := hasReason(attest.GateAccept(ev, g), attest.ReasonTrustBelowMin)
		if got == row.ok {
			t.Errorf("have %q min %q: below-min=%v", row.have, row.min, got)
		}
	}
}

func TestGateAccept_ReasonsAreOrderedAndUnique(t *testing.T) {
	ev := goodEvidence()
	ev.SourceTrust, ev.Pipeline.Kind = "untrusted", "adhoc"
	g := goodGate(ev)
	g.RepoID, g.PolicyDigest, g.Current = "x", "y", nil
	first := attest.GateAccept(ev, g)
	for i := 0; i < 5; i++ {
		if again := attest.GateAccept(ev, g); !sameReasons(first, again) {
			t.Fatalf("unstable: %v vs %v", first, again)
		}
	}
	seen := map[attest.Reason]bool{}
	for _, r := range first {
		if seen[r] {
			t.Fatalf("duplicate %s", r)
		}
		seen[r] = true
	}
}

func TestGateAccept_StaleIgnoresStoredFreshness(t *testing.T) {
	ev := goodEvidence()
	ev.Freshness = model.Freshness{State: "current"}
	g := goodGate(ev)
	c := *g.Current
	c.Lockfiles = []model.DigestRef{{Path: "go.sum", SHA256: strings.Repeat("9", 64)}, c.Lockfiles[1]}
	g.Current = &c
	if got := attest.GateAccept(ev, g); !hasReason(got, attest.ReasonStale) {
		t.Fatalf("stored freshness trusted: %v", got)
	}
}

func TestGateAccept_SignedRemoteWithVerifier(t *testing.T) {
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	ev := signedRemote(goodEvidence())
	g := goodGate(ev)

	ok := &fakeVerifier{}
	attest.RegisterRemoteVerifier(ok)
	if got := attest.GateAccept(ev, g); len(got) != 0 || ok.calls != 1 {
		t.Fatalf("verified remote evidence: %v calls %d", got, ok.calls)
	}
	attest.RegisterRemoteVerifier(&fakeVerifier{err: errors.New("bad signature")})
	if got := attest.GateAccept(ev, g); !sameReasons(got, []attest.Reason{attest.ReasonSignatureUnverifiable}) {
		t.Fatalf("refused signature accepted: %v", got)
	}
	attest.RegisterRemoteVerifier(&fakeVerifier{panics: true})
	if got := attest.GateAccept(ev, g); !sameReasons(got, []attest.Reason{attest.ReasonSignatureUnverifiable}) {
		t.Fatalf("panicking verifier accepted: %v", got)
	}
	attest.RegisterRemoteVerifier(nil)
	if got := attest.GateAccept(ev, g); !sameReasons(got, []attest.Reason{attest.ReasonSignatureUnverifiable}) {
		t.Fatalf("no verifier accepted: %v", got)
	}
}

// A document that claims a local producer but carries a runner signature, or
// the other way round, contradicts itself.
func TestGateAccept_ProducerSignatureMismatch(t *testing.T) {
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	attest.RegisterRemoteVerifier(&fakeVerifier{})
	ev := goodEvidence()
	ev.Signature = model.Signature{Kind: "runner-ed25519", KeyID: strp("k"), Value: strp("v")}
	if got := attest.GateAccept(ev, goodGate(ev)); !hasReason(got, attest.ReasonInvalid) {
		t.Fatalf("local producer with runner signature: %v", got)
	}
	ev = signedRemote(goodEvidence())
	ev.Signature = model.Signature{Kind: "coordinator-recorded"}
	if got := attest.GateAccept(ev, goodGate(ev)); !hasReason(got, attest.ReasonInvalid) {
		t.Fatalf("remote producer without signature: %v", got)
	}
}

func TestGateAccept_EditedResultAndCounts(t *testing.T) {
	ev := goodEvidence()
	ev.Checks[0].Result = "fail" // result still says pass
	ev.Counts = model.Counts{Pass: 2, Fail: 1, Total: 3}
	if got := attest.GateAccept(ev, goodGate(ev)); !hasReason(got, attest.ReasonInvalid) {
		t.Fatalf("edited result accepted: %v", got)
	}
	ev = goodEvidence()
	ev.Counts.Pass = 2
	if got := attest.GateAccept(ev, goodGate(ev)); !hasReason(got, attest.ReasonInvalid) {
		t.Fatalf("wrong counts accepted: %v", got)
	}
}

func TestGateAccept_RevisionFormat(t *testing.T) {
	for _, rev := range []string{"", "abc", strings.ToUpper(goodRevision), goodRevision + "0", strings.Repeat("g", 40)} {
		ev := goodEvidence()
		ev.Revision = rev
		g := goodGate(ev)
		if got := attest.GateAccept(ev, g); len(got) == 0 {
			t.Errorf("revision %q accepted", rev)
		}
	}
	ev := goodEvidence()
	ev.Revision = strings.Repeat("a", 64)
	g := goodGate(ev)
	g.Revision = ev.Revision
	if got := attest.GateAccept(ev, g); len(got) != 0 {
		t.Fatalf("sha256 revision rejected: %v", got)
	}
}

// A failing job and a removed job are both reported.
func TestGateAccept_ReportsEveryJobProblem(t *testing.T) {
	ev := goodEvidence()
	ev.Checks[2].Result, ev.Checks[2].Required = "fail", false // lint fails
	ev.Counts = model.Counts{Pass: 2, Fail: 1, Total: 3}
	g := goodGate(ev)
	g.RequiredJobs = []string{"lint", "deploy-check"}
	got := attest.GateAccept(ev, g)
	if !hasReason(got, attest.ReasonJobsMismatch) || !hasReason(got, attest.ReasonNotPassing) {
		t.Fatalf("got %v", got)
	}
}

func TestGateAcceptDocument(t *testing.T) {
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	attest.RegisterRemoteVerifier(nil)
	ev := goodEvidence()
	if got := attest.GateAcceptDocument(marshal(t, ev), goodGate(ev)); len(got) != 0 {
		t.Fatalf("good document rejected: %v", got)
	}
	if got := attest.GateAcceptDocument([]byte("{"), goodGate(ev)); !sameReasons(got, []attest.Reason{attest.ReasonInvalid}) {
		t.Fatalf("garbage: %v", got)
	}
	// the verifier sees the exact bytes, exactly once
	rec := &recordingVerifier{}
	attest.RegisterRemoteVerifier(rec)
	signed := signedRemote(ev)
	doc, err := json.MarshalIndent(signed, "", "  ") // bytes a re-marshal would not reproduce
	if err != nil {
		t.Fatal(err)
	}
	if got := attest.GateAcceptDocument(doc, goodGate(signed)); len(got) != 0 {
		t.Fatalf("signed document rejected: %v", got)
	}
	if rec.calls != 1 || string(rec.doc) != string(doc) {
		t.Fatalf("verifier saw %d calls, doc match %v", rec.calls, string(rec.doc) == string(doc))
	}
	attest.RegisterRemoteVerifier(nil)
	if got := attest.GateAcceptDocument(doc, goodGate(signed)); !hasReason(got, attest.ReasonSignatureUnverifiable) {
		t.Fatalf("no verifier: %v", got)
	}
}

// An unnamed required job is never satisfied, even by an unnamed check.
func TestGateAccept_EmptyJobNameNeverSatisfied(t *testing.T) {
	ev := goodEvidence()
	ev.Checks = append(ev.Checks, model.Check{ID: "anon", JobID: "", Kind: "test", Tool: "x", Required: true, Substantive: true, Result: "pass"})
	ev.Counts = model.Counts{Pass: 4, Total: 4}
	g := goodGate(ev)
	g.RequiredJobs = []string{"build", "lint", ""}
	if got := attest.GateAccept(ev, g); !sameReasons(got, []attest.Reason{attest.ReasonJobsMismatch}) {
		t.Fatalf("got %v", got)
	}
}

func TestGateAccept_NamedPipelineIsAccepted(t *testing.T) {
	ev := goodEvidence()
	ev.Pipeline.Kind = "named"
	if got := attest.GateAccept(ev, goodGate(ev)); len(got) != 0 {
		t.Fatalf("named pipeline rejected: %v", got)
	}
}

// Freshness that cannot be decided (the evidence binds no config digest) is not freshness.
func TestGateAccept_UnknownFreshnessIsStale(t *testing.T) {
	ev := goodEvidence()
	ev.InputDigests.Config = ""
	g := goodGate(ev)
	if f := attest.Fresh(ev, *g.Current); f.State != "unknown" {
		t.Fatalf("setup: %+v", f)
	}
	if got := attest.GateAccept(ev, g); !sameReasons(got, []attest.Reason{attest.ReasonStale}) {
		t.Fatalf("got %v", got)
	}
}

// Only a pass satisfies a gate, whatever the checks say.
func TestGateAccept_OnlyAPassingResultPasses(t *testing.T) {
	for _, result := range []model.Result{"fail", "error", "cancelled"} {
		ev := goodEvidence()
		ev.Result = result
		if got := attest.GateAccept(ev, goodGate(ev)); !sameReasons(got, []attest.Reason{attest.ReasonNotPassing}) {
			t.Errorf("result %s: got %v", result, got)
		}
	}
}

func TestGateAccept_EmptyRevisionNeverMatchesEmpty(t *testing.T) {
	ev := goodEvidence()
	ev.Revision = ""
	g := goodGate(ev)
	g.Revision = ""
	if got := attest.GateAccept(ev, g); !hasReason(got, attest.ReasonRevisionMismatch) {
		t.Fatalf("empty revision matched empty: %v", got)
	}
}

func TestGateAccept_DoesNotModifyInput(t *testing.T) {
	ev := goodEvidence()
	g := goodGate(ev)
	before := string(marshal(t, ev))
	jobs := append([]string(nil), g.RequiredJobs...)
	attest.GateAccept(ev, g)
	if string(marshal(t, ev)) != before || strings.Join(jobs, ",") != strings.Join(g.RequiredJobs, ",") {
		t.Fatal("GateAccept modified its input")
	}
}
