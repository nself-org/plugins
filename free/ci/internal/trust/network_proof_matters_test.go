package trust_test

// Purpose: NetworkProofMatters is exact (P7-CI-46): for every class it reports false, the NoLANRoute fact
//          changes no admission reason and no minimum isolation, so a run of those classes may skip the proof.
// Inputs: the invariants world generator. Outputs: pass or fail.

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestNetworkProofMattersOnlyWhereAdmissionReadsIt(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		if trust.NetworkProofMatters(w.Trust) {
			return nil
		}
		for _, hosted := range []bool{false, true} {
			with, without := declaration(w), declaration(w)
			with.Hosted, without.Hosted = hosted, hosted
			with.NoLANRoute, without.NoLANRoute = true, false
			if a, b := trust.Admissible(job(w), with, trust.Effective{}), trust.Admissible(job(w), without, trust.Effective{}); !reflect.DeepEqual(a, b) {
				return fmt.Errorf("class %s: NoLANRoute changed admission %v -> %v", w.Trust, b, a)
			}
			rf := trust.RunnerFacts{Ownership: with.Ownership, Hosted: hosted, UIDSeparation: true}
			rn := rf
			rf.NoLANRoute = true
			if a, b := trust.MinIsolation(job(w), rf), trust.MinIsolation(job(w), rn); a != b {
				return fmt.Errorf("class %s: NoLANRoute changed the floor %s -> %s", w.Trust, b, a)
			}
		}
		return nil
	})
	for _, c := range []string{"untrusted", "", "stranger"} {
		if !trust.NetworkProofMatters(modelClass(c)) {
			t.Fatalf("class %q must need the proof", c)
		}
	}
}

func modelClass(s string) model.TrustClass { return model.TrustClass(s) }
