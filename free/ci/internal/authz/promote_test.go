package authz_test

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

func goodArtifact() authz.Artifact {
	return authz.Artifact{Project: "repo-1", Revision: goodRevision, SHA256: goodDigest}
}

func TestPromotable_CurrentReleaseEvidenceOfExactRevision(t *testing.T) {
	ev := goodEvidence()
	ok, reasons := authz.Promotable(goodArtifact(), ev, goodGate(ev))
	if !ok || len(reasons) != 0 {
		t.Fatalf("good evidence refused: %v", reasons)
	}
}

func TestPromotable_Refusals(t *testing.T) {
	rows := []struct {
		name string
		bend func(*authz.Artifact, *model.Evidence, *attest.Gate)
		want attest.Reason
	}{
		{"unknown minimum trust", func(_ *authz.Artifact, _ *model.Evidence, g *attest.Gate) { g.MinTrust = "misspelled" }, attest.ReasonTrustBelowMin},
		{"stale", func(_ *authz.Artifact, _ *model.Evidence, g *attest.Gate) {
			c := *g.Current
			c.Config = strings.Repeat("d", 64)
			g.Current = &c
		}, attest.ReasonStale},
		{"unknown freshness", func(_ *authz.Artifact, _ *model.Evidence, g *attest.Gate) { g.Current = nil }, attest.ReasonStale},
		{"unverifiable signed-remote", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) {
			k, v := "ci-node-0123456789abcdef", "c2ln"
			e.Producer.Kind = "signed-remote"
			e.Signature = model.Signature{Kind: "runner-ed25519", KeyID: &k, Value: &v}
		}, attest.ReasonSignatureUnverifiable},
		{"not a release selection", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Selection.Mode = "affected" }, attest.ReasonSelectionPartial},
		{"self reported", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Producer.Kind = "self-reported" }, attest.ReasonSelfReportedNotAdmitted},
		{"different revision (artifact)", func(a *authz.Artifact, _ *model.Evidence, _ *attest.Gate) { a.Revision = strings.Repeat("b", 40) }, attest.ReasonRevisionMismatch},
		{"different revision (evidence)", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Revision = strings.Repeat("b", 40) }, attest.ReasonRevisionMismatch},
		{"artifact without revision", func(a *authz.Artifact, _ *model.Evidence, _ *attest.Gate) { a.Revision = "" }, attest.ReasonRevisionMismatch},
		{"different repo (artifact)", func(a *authz.Artifact, _ *model.Evidence, _ *attest.Gate) { a.Project = "repo-2" }, attest.ReasonRepoMismatch},
		{"digest not in evidence", func(a *authz.Artifact, _ *model.Evidence, _ *attest.Gate) { a.SHA256 = strings.Repeat("e", 64) }, authz.ReasonArtifactUnbound},
		{"artifact without digest", func(a *authz.Artifact, _ *model.Evidence, _ *attest.Gate) { a.SHA256 = "" }, authz.ReasonArtifactUnbound},
		{"evidence lists no artifacts", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Artifacts = nil }, authz.ReasonArtifactUnbound},
		{"digest listed but not committed", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Artifacts[0].Committed = false }, authz.ReasonArtifactUnbound},
		{"failing evidence", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.Result = "fail" }, attest.ReasonNotPassing},
		{"trust below minimum", func(_ *authz.Artifact, e *model.Evidence, _ *attest.Gate) { e.SourceTrust = "collaborator" }, attest.ReasonTrustBelowMin},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			a, ev := goodArtifact(), goodEvidence()
			g := goodGate(ev)
			r.bend(&a, &ev, &g)
			ok, reasons := authz.Promotable(a, ev, g)
			if ok {
				t.Fatal("promoted")
			}
			found := false
			for _, got := range reasons {
				found = found || got == r.want
			}
			if !found {
				t.Fatalf("reasons %v lack %s", reasons, r.want)
			}
		})
	}
}

// The caller's gate cannot weaken the release rules.
func TestPromotable_ForcesReleaseGate(t *testing.T) {
	ev := goodEvidence()
	ev.Producer.Kind = "self-reported"
	ev.Selection.Mode = "affected"
	g := goodGate(ev)
	g.Class, g.FullSelection, g.AllowSelfReported = attest.ClassDefault, false, true
	ok, reasons := authz.Promotable(goodArtifact(), ev, g)
	if ok || len(reasons) < 2 {
		t.Fatalf("weak gate promoted: %v", reasons)
	}
}

// A gate with a low trust floor cannot promote evidence from an untrusted source.
func TestPromotable_TrustFloor(t *testing.T) {
	for _, floor := range []model.TrustClass{"", "untrusted", "unknown"} {
		ev := goodEvidence()
		ev.SourceTrust = "untrusted"
		g := goodGate(ev)
		g.MinTrust = floor
		if ok, _ := authz.Promotable(goodArtifact(), ev, g); ok {
			t.Fatalf("floor %q promoted untrusted evidence", floor)
		}
	}
}
