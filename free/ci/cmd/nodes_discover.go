package main

// Purpose: registers `nself ci nodes discover` (P7-NODE-12, Epic D12): browse the LAN for pairing
//          candidates for this one invocation. The logic lives in internal/discovery; this file only
//          registers the key.
// Inputs:  --timeout, --json.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (usage, E667 discovery disabled) or 2.
// Constraints: V15 only; side-effect class read (nothing is written or paired). Nothing else in this
//          binary opens a multicast socket.

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/discovery"
)

func init() {
	registerV15("nodes discover", subcommandEntry{
		V15Only: true,
		Flags:   discovery.FlagSet,
		Handler: func(args []string) int {
			return discovery.RunDiscover(args, discovery.Env{Out: os.Stdout, Err: os.Stderr, Guard: &discovery.Guard{}})
		},
	})
}
