package sched

import (
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestBoundaryRefusalSurfacesE689 (D-1250): a placement refused at the production boundary carries
// E689 in its detail, so the operator can look up the Fix; the reason code stays the boundary.* string.
func TestBoundaryRefusalSurfacesE689(t *testing.T) {
	r := fixtureRunner("deploy1", "lan")
	r.Decl.DeployHost = trust.DeployHost{Matched: true}
	got := Place(fixtureWorld(r), []Job{fixtureJob("j")})
	if len(got.Placements) != 0 {
		t.Fatalf("placed on a deploy host without an override: %+v", got.Placements)
	}
	for _, x := range got.Explanations[0].Alternatives[0].Reasons {
		if x.Code == string(trust.BoundaryDeployHost) {
			if !strings.HasPrefix(x.Detail, "E689: ") {
				t.Fatalf("detail %q does not lead with E689", x.Detail)
			}
			return
		}
	}
	t.Fatalf("no %s reason: %+v", trust.BoundaryDeployHost, got.Explanations[0].Alternatives[0].Reasons)
}
