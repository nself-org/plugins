package run

// Purpose: register the sandboxed-container (gVisor) profile with the isolation registry.
// Inputs:  NSELF_CI_NODE_SEPARATED=1 in the agent's own environment on a separated node, the
// same switch the container profile reads (it is never read from job env).
// Outputs: a registered isolation.Profile for the `sandboxed-container` level.
// Constraints: one file per profile, so profiles never share a hook file (Epic D7). Registering
// does not make the level available: the profile probes by really starting runsc.

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/gvisor"
)

func init() {
	isolation.Register(gvisor.New(gvisor.Options{
		Separated: func() bool { return os.Getenv("NSELF_CI_NODE_SEPARATED") == "1" },
	}))
}
