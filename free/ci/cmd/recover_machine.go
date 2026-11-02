package main

// Purpose: the one place read commands ask for crash recovery (P7-CI-62 review). Recovery kills stale process groups,
//          removes attempt workspaces and containers and rewrites run state, so a machine caller (MCP, HTTP: the cli
//          sets NSELF_INVOKED_BY) on a read row never triggers it; the CLI and `runs resolve` (destructive, confirmed) do.
// Inputs:  env NSELF_INVOKED_BY, NSELF_CI_HOME; an open store.
// Outputs: nothing; recovery runs only for a human caller.
// Constraints: recoverRuns is a seam so a test can count calls.

import (
	"context"
	"os"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

var recoverRuns = cirun.Recover

// recoverForHuman runs crash recovery unless a machine caller started this command.
func recoverForHuman(ctx context.Context, st *store.Store) {
	if os.Getenv("NSELF_INVOKED_BY") != "" {
		return
	}
	_ = recoverRuns(ctx, st, "", os.Getenv("NSELF_CI_HOME"))
}
