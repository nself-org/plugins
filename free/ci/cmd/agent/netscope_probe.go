package main

// Purpose: `nself-ci-agent netscope-probe` (D-1332): run the network scope proof on this node and print what it
//          proved, so the coordinator's probe can record capability network.enforced_scopes.
// Inputs:  none; the container runtime and the host's own deploy exclusion set are read from this node.
// Outputs: one JSON line {"scopes":["none",...]} on stdout; an empty list when nothing could be proven.
// Constraints: the proof builds a job network and a sidecar and removes them, so it takes seconds; it claims
//          nothing it could not demonstrate (a node without a runtime prints []). The production start guard runs first (E663 on
//          a deploy host). Exit 0 whenever a line was printed.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// proveScopes is replaced by tests: the real proof needs a container runtime.
var proveScopes = cirun.ProveNetworkScopes

func init() {
	must(RegisterSubcommand("netscope-probe", "prove which network scopes this node enforces (JSON)", func(ctx context.Context, args []string, sio IO) int {
		if len(args) != 0 {
			fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent netscope-probe")
			return 2
		}
		if err := RunPreStart(ctx, "netscope-probe"); err != nil { // the proof starts containers: never on a deploy host
			fmt.Fprintln(sio.Stderr, "error:", err)
			return 1
		}
		scopes := proveScopes(ctx)
		if scopes == nil {
			scopes = []model.NetworkScope{}
		}
		b, err := json.Marshal(struct {
			Scopes []model.NetworkScope `json:"scopes"`
		}{scopes})
		if err != nil {
			fmt.Fprintln(sio.Stderr, "nself-ci-agent: netscope-probe:", err)
			return 1
		}
		fmt.Fprintf(sio.Stdout, "%s\n", b)
		return 0
	}))
}
