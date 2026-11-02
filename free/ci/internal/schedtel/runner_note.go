package schedtel

// Purpose: the runner an admitted attempt runs on, in the form its evidence
// records (node, provider, platform, isolation), noted for internal/run.
// Inputs:  the dispatcher's world at the decision and the chosen runner id.
// Outputs: a model.AttemptRunner; false when the world does not hold the runner.
// Constraints: only what the runner's capability declares; an unknown platform
// is "unknown" and an undeclared isolation is "process", the weakest claim.

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// attemptRunner describes the runner id in w as an attempt's evidence records it.
func attemptRunner(w *sched.World, id string) (model.AttemptRunner, bool) {
	if w == nil {
		return model.AttemptRunner{}, false
	}
	for _, r := range append(append([]sched.Runner(nil), w.Runners...), w.Hosted...) {
		if r.ID != id {
			continue
		}
		c := r.Capability
		out := model.AttemptRunner{Kind: kindOf(r), Tier: sched.TierFor(r.Location, classOf(r)), Node: r.ID,
			Provider: firstOf(r.Provider, c.Identity.Provider), Platform: "unknown", Isolation: "process"}
		if c.Platform.OS.Value != nil && c.Platform.Arch.Value != nil {
			out.Platform = *c.Platform.OS.Value + "/" + *c.Platform.Arch.Value
		}
		if v := c.Trust.Isolation.Value; v != nil && v.Valid() {
			out.Isolation = *v
		}
		return out, true
	}
	return model.AttemptRunner{}, false
}

// kindOf is the placement kind of a runner: where it is, then how it is reached.
func kindOf(r sched.Runner) string {
	switch {
	case r.Location == "hosted":
		return "hosted"
	case r.Location == "local" || r.Capability.Identity.Transport == "local":
		return "local"
	case r.Capability.Identity.Transport == "agent":
		return "agent"
	}
	return "ssh"
}

func firstOf(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
