package main

// Purpose: the agent's production start guard (P7-NODE-14, Epic D9). Registered as a PreStart hook, it
// stops `serve`, `exec`, `join` and `pull` on a host that is a deploy host (local identity in the exclusion set,
// or a running compose project with ENV=prod) before the command reads or writes anything.
// Inputs:  the command name from RunPreStart; the local host and $NSELF_CI_HOME (config.yaml, state.db).
// Outputs: nil, or an E663 error that makes the command exit non-zero; warnings on stderr.
// Constraints: the logic lives in internal/nodes/deployguard; this file only registers it; hook order is
//          registration order and this file sorts before the checkout hooks, so the guard runs first.

import (
	"context"
	"fmt"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
)

const prodGuardName = "prodguard"

// prodLocal reads this host and prodHome is the CI home (config.yaml, state.db); tests replace both so
// the guard never reads the developer's machine.
var (
	prodLocal = func() deployguard.Local { return deployguard.NewLocal(deployguard.FunnelRun) }
	prodHome  = ""
)

func init() {
	must(RegisterPreStart(prodGuardName, prodGuard))
}

func prodGuard(ctx context.Context, command string) error {
	warns, err := deployguard.StartGuard(ctx, prodHome, prodLocal(), "nself-ci-agent "+command)
	for _, w := range warns {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	return err
}
