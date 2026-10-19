package trust

// Purpose: the E689 error code for the production boundary. It lives beside prodboundary.go (not in codes.go) so this
// Ticket owns its whole write scope; the boundary.* reasons map to it in the scheduler that renders refusals.

import "github.com/nself-org/plugins/free/ci/internal/model"

func init() {
	model.Register(model.Code{ID: "E689", Class: "usage", Summary: "placement refused at the production boundary", Fix: "Run the job on a node that is not a deploy host, or on a deploy host the operator overrode with owner or internal work, isolation vm or stronger and no release or deploy secrets; a coordinator host needs vm for team mode and for collaborator or untrusted work"})
}

// DetailOf is the detail text a refusal carries for r: the three boundary.* reasons lead with E689 so the operator
// can look up its Fix text; every other reason keeps its own name.
func DetailOf(r Reason) string {
	switch r {
	case BoundaryDeployHost, BoundaryOverrideMinimum, BoundaryCoordinatorHost:
		return "E689: " + ReasonMeaning[r]
	}
	return string(r)
}
