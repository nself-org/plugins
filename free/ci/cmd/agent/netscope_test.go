package main

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/isolation"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

func TestAgentHasNoAuthoritativeExclusionSource(t *testing.T) {
	_, err := agentExclusions(context.Background())
	if err == nil {
		t.Fatal("agent local state was accepted as coordinator exclusions")
	}
	// The source is consumed only for proxy scopes; offline and explicitly LAN
	// jobs remain available while the coordinator-set lease extension is absent.
	planner := &netscope.Planner{Exclusion: agentExclusions}
	for _, scope := range []model.NetworkScope{"none", "lan"} {
		t.Run(string(scope), func(t *testing.T) {
			_, err := planner.PlanAttempt(context.Background(), "no-runtime", isolation.Attempt{}, scope)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
