package main

import (
	"context"
	"errors"

	"github.com/nself-org/plugins/free/ci/internal/netscope"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func init() {
	// Leases do not carry the coordinator's exclusion set yet. Local state is not
	// authoritative for other projects, so a proxy must not use it as the full set.
	cirun.RegisterNetscopeExclusions(agentExclusions)
}

func agentExclusions(context.Context) (netscope.Exclusions, error) {
	return netscope.Exclusions{}, errors.New("coordinator deploy exclusion source is unavailable on this agent")
}
