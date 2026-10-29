package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestNoLANRouteNeedsTheProofAndABelowLanScope(t *testing.T) {
	enforced := []model.NetworkScope{"none", "restricted"}
	for _, c := range []struct {
		trust model.TrustClass
		scope model.NetworkScope
		proof []model.NetworkScope
		want  bool
	}{
		{"untrusted", "", enforced, true}, // the default for untrusted is restricted
		{"untrusted", "restricted", enforced, true},
		{"untrusted", "restricted", nil, false},    // no proof
		{"untrusted", "internet", enforced, false}, // internet was not proven
		{"collaborator", "", enforced, false},      // default internet
		{"owner", "", enforced, false},             // default lan: never "no LAN route"
		{"untrusted", "lan", enforced, false},
		{"untrusted", "privileged", []model.NetworkScope{"privileged"}, false},
		{"untrusted", "none", []model.NetworkScope{"none"}, true},
	} {
		if got := noLANRoute(Job{Trust: c.trust, Network: c.scope}, c.proof); got != c.want {
			t.Errorf("%s/%q proof %v: %v, want %v", c.trust, c.scope, c.proof, got, c.want)
		}
	}
}

// Placement: an untrusted job is placed on a runner only where the world carries a proof for its scope.
func TestUntrustedPlacementFollowsTheNetworkProof(t *testing.T) {
	job := func() Job {
		j := fixtureJob("job")
		j.Trust, j.Network, j.PersonalOwnRun = "untrusted", "restricted", false
		return j
	}
	r := fixtureRunner("r", "lan")
	r.Decl.NoLANRoute = true // forged declarations cannot replace a missing proof
	r.Decl.Isolation = "sandboxed-container"
	for name, c := range map[string]struct {
		net    map[string]NetworkFact
		placed bool
	}{
		"no proof":         {nil, false},
		"proof elsewhere":  {map[string]NetworkFact{"other": {Enforced: []model.NetworkScope{"restricted"}}}, false},
		"scope not proven": {map[string]NetworkFact{"r": {Enforced: []model.NetworkScope{"none"}}}, false},
		"scope proven":     {map[string]NetworkFact{"r": {Enforced: []model.NetworkScope{"none", "restricted"}}}, true},
	} {
		w := fixtureWorld(r)
		w.Network = c.net
		got := Place(w, []Job{job()})
		if (len(got.Placements) == 1) != c.placed {
			t.Errorf("%s: placements %+v reasons %+v", name, got.Placements, got.Explanations[0].Alternatives)
		}
		if !c.placed && !containsCode(got.Explanations[0].Alternatives[0].Reasons, "network.lan_for_untrusted") {
			t.Errorf("%s: no lan-for-untrusted reason: %+v", name, got.Explanations[0].Alternatives[0].Reasons)
		}
	}
}
