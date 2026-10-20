// Package doctor is the `nodes` section of `nself ci doctor` (P7-NODE-16, Epic NODE D15, X18): fifteen checks
// CI-DN001..CI-DN015 on the registered nodes, shown by `nself ci doctor --nodes`. There is no `ci nodes doctor`
// command; the section is the only way in.
//
// Purpose: say, per node, whether it can take work and what to do when it cannot.
// Inputs:  the node registry and run store under the CI home (read only), the pin file, one TCP dial and one
// `date -u +%s` per ssh node, the discovery switch, the binary's limits (Settings).
// Outputs: doctor.Check values with stable ids, a remediation on every result and a subject (the node id).
// Constraints: registers itself in the doctor section registry from init, so internal/doctor is not edited;
// the section never creates the state database; a node with no pinned host key is never contacted.
package doctor

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/doctor"
)

func init() {
	doctor.RegisterSection(doctor.Section{Name: SectionName, Selector: doctor.SelectNodes, Run: run})
}

// gatherTimeout keeps the readings inside the section budget (doctor.SectionBudget), so a slow node yields a
// result instead of the whole-section warning.
const gatherTimeout = doctor.SectionBudget - 2*time.Second

func run(ctx context.Context, env doctor.Env) []doctor.Check {
	gctx, cancel := context.WithTimeout(ctx, gatherTimeout)
	defer cancel()
	return Evaluate(Gather(gctx, env, DefaultDeps()))
}

// Evaluate runs every check on the facts, in id order.
func Evaluate(f Facts) []doctor.Check {
	var out []doctor.Check
	for _, c := range []func(Facts) []doctor.Check{
		checkReach, checkHostKey, checkOpenSSH, checkCert, checkSkew, checkVersion, checkDrift, checkDisk,
		checkExposure, checkDeploy, checkBreaker, checkDiscovery, checkUpgrade, checkGitHub, checkSeparate,
	} {
		out = append(out, c(f)...)
	}
	return out
}
