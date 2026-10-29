package reconcile

// Purpose: the mark name the coordinator prunes stays the one the Engine writes (D-1319).

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/coordinator"
)

func TestMarkNameIsPrunedByTheCoordinator(t *testing.T) {
	for _, r := range coordinator.DefaultMarkRetention {
		if r.Name != markName {
			t.Fatalf("the coordinator prunes marks named %q but the Engine writes %q", r.Name, markName)
		}
	}
}
