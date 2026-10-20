package main

// Purpose: registers `nself ci nodes upgrade [<node>|--all] [--version V] [--allow-downgrade]` (hq NODE D11, P7-NODE-24). The
//          state machine, the ceiling, the swap and the carriers live in internal/agent/update; this file wires the pinned
//          version, the release mirror and the trusted key set into it, nothing else.
// Inputs:  argv after the key; env NSELF_CI_HOME, NSELF_CI_REPO, NSELF_CI_AGENT_RELEASE_URL.
// Outputs: one line per node or one v1 JSON envelope; exit 0, 1 (refused or usage), 2 (infrastructure) or 3 (host key, revoked).
// Constraints: V15 only, so v1.4 argv is unchanged; side-effect class remote (it drains, installs and enables a node).

import (
	"flag"
	"io"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/agent/update"
	"github.com/nself-org/plugins/free/ci/internal/agent/verify"
	"github.com/nself-org/plugins/free/ci/internal/nodes/lifecycle"
)

func init() {
	registerV15("nodes upgrade", subcommandEntry{
		V15Only: true,
		Flags:   func() *flag.FlagSet { return update.FlagSet(io.Discard) },
		Handler: func(args []string) int {
			env := update.Env{
				Lifecycle:      lifecycle.ProductionEnv(currentProject),
				DefaultVersion: agentPinnedVersion,
				BaseURL:        os.Getenv("NSELF_CI_AGENT_RELEASE_URL"),
				Verifier:       func() (*verify.Verifier, error) { return agentVerifier() },
			}
			return update.Run(args, env, os.Stdout, os.Stderr)
		},
	})
}
