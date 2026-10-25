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
// gate said, and the trust floor is at least "internal" (an unknown floor is refused, not raised). An artifact of another repo or
// revision is refused, and so is one whose SHA-256 the evidence does not list
// as a committed artifact: a fork build of the same commit is not the release.

import (
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Artifact names what is being promoted.
type Artifact struct {
	Project  string // the repo id the evidence is about
	Revision string
	SHA256   string // lowercase hex digest of the artifact bytes, computed by the caller
}

// ReasonArtifactUnbound is returned when the evidence does not list the
// artifact's digest as a committed artifact.
const ReasonArtifactUnbound attest.Reason = "artifact.unbound"

// promoteMinTrust is the lowest source trust a release may rest on.
const promoteMinTrust model.TrustClass = "internal"

// Promotable returns true with no reasons when ev satisfies g as a release gate
// and a is a build of exactly g's revision in g's repo; otherwise false and
// every reason, in attest's stable order.
func Promotable(a Artifact, ev model.Evidence, g attest.Gate) (bool, []attest.Reason) {
	g.Class = attest.ClassRelease
	g.FullSelection = true
	g.AllowSelfReported = false
	// An unknown floor (ranks -1) is left as is so GateAccept rejects it; raising it
	// to the default would let a typo'd owner floor weaken to internal.
	if r := trust.RankTrust(g.MinTrust); r >= 0 && r < trust.RankTrust(promoteMinTrust) {
		g.MinTrust = promoteMinTrust
	}
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
	if !bound(a.SHA256, ev.Artifacts) {
		reasons = append(reasons, ReasonArtifactUnbound)
	}
	return len(reasons) == 0, reasons
}

// bound is true when sum names a committed artifact in the evidence.
func bound(sum string, list []model.Artifact) bool {
	if sum == "" {
		return false
	}
	for _, x := range list {
		if x.Committed && strings.EqualFold(x.SHA256, sum) {
			return true
		}
	}
	return false
}
