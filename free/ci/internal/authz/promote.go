package authz

// Purpose: may an artifact be promoted (released, published, deployed)? Only
// on current, verifiable, passing, full-selection evidence of the artifact's
// exact revision. This is attest.GateAccept judged as a release gate; nothing
// is re-implemented here.
// Inputs:  the artifact's project and revision, the evidence from the
// coordinator's own store, the gate the caller built from protected policy.
// Outputs: Promotable.
// Constraints: pure. The gate class is forced to release, self-reported
// evidence is never admitted and selection must be full, whatever the caller's
// gate said. An artifact of another repo or revision is refused.

import (
	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Artifact names what is being promoted.
type Artifact struct {
	Project  string // the repo id the evidence is about
	Revision string
}

// Promotable returns true with no reasons when ev satisfies g as a release gate
// and a is a build of exactly g's revision in g's repo; otherwise false and
// every reason, in attest's stable order.
func Promotable(a Artifact, ev model.Evidence, g attest.Gate) (bool, []attest.Reason) {
	g.Class = attest.ClassRelease
	g.FullSelection = true
	g.AllowSelfReported = false
	reasons := attest.GateAccept(ev, g)
	have := map[attest.Reason]bool{}
	for _, r := range reasons {
		have[r] = true
	}
	if (a.Revision == "" || a.Revision != g.Revision) && !have[attest.ReasonRevisionMismatch] {
		reasons = append(reasons, attest.ReasonRevisionMismatch)
	}
	if (a.Project == "" || a.Project != g.RepoID) && !have[attest.ReasonRepoMismatch] {
		reasons = append(reasons, attest.ReasonRepoMismatch)
	}
	return len(reasons) == 0, reasons
}
