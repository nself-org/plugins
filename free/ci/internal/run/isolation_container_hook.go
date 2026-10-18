package run

// Purpose: register the hardened container profile with the isolation registry.
// Inputs:  NSELF_CI_NODE_SEPARATED=1 in the agent's own environment on a separated node
// (NODE sets it where the agent user has no docker group; it is never read from job env).
// Outputs: a registered isolation.Profile for the `container` level.
// Constraints: one file per profile, so profiles never share a hook file (Epic D7).

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/isolation/container"
)

func init() {
	isolation.Register(container.New(container.Options{
		Separated: func() bool { return os.Getenv("NSELF_CI_NODE_SEPARATED") == "1" },
	}))
}
