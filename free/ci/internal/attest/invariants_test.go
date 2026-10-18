package attest_test

// Purpose: TestInvariant_I12 (evidence binds the exact revision and input
// digests) and TestInvariant_I13 (stale or unverifiable evidence never
// satisfies a current gate) over the shared world product: enumerated, then
// 10000 generated worlds (invariants.Check).
// Each world is turned into an evidence document and a gate by fixed rules, so
// a world is a reproducible scenario, and an independent oracle states which
// scenarios must be accepted.

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

var trustNames = []model.TrustClass{"untrusted", "collaborator", "internal", "owner"}

func indexOf(name, value string) int {
	for i, v := range model.EnumValues(name) {
		if v == value {
			return i
		}
	}
	return -1
}

// scene is one world turned into a scenario.
type scene struct {
	ev                              model.Evidence
	gate                            attest.Gate
	revBad, policyBad, dirty, stale bool
	adhoc, partialBad               bool
	trustBad                        bool
	producer                        string
	release, allowSelf              bool
}

var baseEvidence = goodEvidence()

func build(w invariants.World) scene {
	var mask int
	for _, s := range w.Secrets {
		mask |= 1 << indexOf("SecretClass", string(s))
	}
	privacy := indexOf("PrivacyZone", string(w.Privacy))
	isolation := indexOf("Isolation", string(w.Isolation))
	ev := baseEvidence
	cur := baseEvidence.InputDigests
	cur.Lockfiles = append([]model.DigestRef(nil), cur.Lockfiles...)
	sc := scene{release: w.Mode == "team", allowSelf: w.Ownership == "operator"}

	ev.SourceTrust = w.Trust
	min := trustNames[indexOf("NetworkScope", string(w.Network))%len(trustNames)]
	sc.trustBad = trust.RankTrust(ev.SourceTrust) < trust.RankTrust(min)

	switch sc.producer = []string{"coordinator-recorded", "signed-remote", "self-reported"}[isolation%3]; sc.producer {
	case "signed-remote":
		ev = signedRemote(ev)
	case "self-reported":
		ev.Producer.Kind = "self-reported"
	}
	affected := privacy%2 == 1
	if affected {
		ev.Selection.Mode = "affected"
	}
	fullRequired := sc.release || privacy == 2
	sc.partialBad = fullRequired && affected
	if w.Isolation == "process" && w.Network == "none" {
		ev.Pipeline.Kind = "adhoc"
		sc.adhoc = true
	}

	g := attest.Gate{RepoID: ev.RepoID, Revision: ev.Revision, PolicyDigest: ev.PolicyDigest, RequiredJobs: []string{"build", "lint"},
		MinTrust: min, Class: attest.ClassDefault, AllowSelfReported: sc.allowSelf, FullSelection: privacy == 2}
	if sc.release {
		g.Class = attest.ClassRelease
	}
	if mask&1 != 0 {
		g.Revision, sc.revBad = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", true
	}
	if mask&2 != 0 {
		g.PolicyDigest, sc.policyBad = "policy-other", true
	}
	if mask&4 != 0 {
		cur.Config, sc.stale = "changed", true
	}
	if mask&8 != 0 {
		cur.Lockfiles[0].SHA256, sc.stale = "changed", true
	}
	if mask&16 != 0 {
		cur.Generated, sc.stale = nil, true
	}
	if mask&32 != 0 {
		reason := model.BindingReason("dirty")
		ev.WorktreeState, ev.Binding, sc.dirty = "dirty", model.Binding{State: "unbound", Reason: &reason}, true
	}
	g.Current = &cur
	if privacy == 3 {
		g.Current, sc.stale = nil, true
	}
	sc.ev, sc.gate = ev, g
	return sc
}

// shouldAccept is the oracle: the rules of Epic D14 restated.
func (s scene) shouldAccept(verifier bool) bool {
	if s.revBad || s.policyBad || s.dirty || s.stale || s.adhoc || s.partialBad || s.trustBad {
		return false
	}
	switch s.producer {
	case "signed-remote":
		return verifier
	case "self-reported":
		return !s.release && s.allowSelf
	}
	return true
}

func equalInputs(a, b model.InputDigests) bool { return reflect.DeepEqual(a, b) }

func TestInvariant_I12(t *testing.T) {
	attest.RegisterRemoteVerifier(&fakeVerifier{})
	t.Cleanup(func() { attest.RegisterRemoteVerifier(nil) })
	accepted := 0
	invariants.Check(t, func(w invariants.World) error {
		sc := build(w)
		got := attest.GateAccept(sc.ev, sc.gate)
		if len(got) == 0 {
			accepted++
			switch {
			case sc.ev.Revision != sc.gate.Revision || !regexp40(sc.ev.Revision):
				return fmt.Errorf("accepted without the exact revision: %+v", w)
			case sc.ev.Binding.State != "bound" || sc.ev.WorktreeState != "clean":
				return fmt.Errorf("accepted unbound evidence: %+v", w)
			case sc.gate.Current == nil || !equalInputs(sc.ev.InputDigests, *sc.gate.Current):
				return fmt.Errorf("accepted evidence whose input digests differ: %+v", w)
			}
		}
		if want := sc.shouldAccept(true); want != (len(got) == 0) {
			return fmt.Errorf("accept=%v want %v (%v) for %+v", len(got) == 0, want, got, w)
		}
		return nil
	})
	if accepted == 0 {
		t.Fatal("no world was accepted: the invariant held vacuously")
	}
}

func TestInvariant_I13(t *testing.T) {
	attest.RegisterRemoteVerifier(nil) // nothing can verify a runner signature here
	rejected := 0
	invariants.Check(t, func(w invariants.World) error {
		sc := build(w)
		got := attest.GateAccept(sc.ev, sc.gate)
		if sc.stale {
			rejected++
			if !hasReason(got, attest.ReasonStale) {
				return fmt.Errorf("stale evidence satisfied the gate (%v): %+v", got, w)
			}
		}
		if sc.producer == "signed-remote" {
			rejected++
			if !hasReason(got, attest.ReasonSignatureUnverifiable) {
				return fmt.Errorf("unverifiable signature satisfied the gate (%v): %+v", got, w)
			}
		}
		if want := sc.shouldAccept(false); want != (len(got) == 0) {
			return fmt.Errorf("accept=%v want %v (%v) for %+v", len(got) == 0, want, got, w)
		}
		return nil
	})
	if rejected == 0 {
		t.Fatal("no stale or unverifiable world: the invariant held vacuously")
	}
}

func regexp40(rev string) bool {
	if len(rev) != 40 && len(rev) != 64 {
		return false
	}
	for _, c := range rev {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
