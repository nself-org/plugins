package main

import (
	"context"
	"fmt"
	"runtime"
)

// Version is the agent release; the release build sets it with
// -ldflags "-X main.Version=<v>" (P7-NODE-28). It must be x.y.z: the
// coordinator compares it with min_agent_version.
var Version = "1.5.0"

func init() {
	must(RegisterSubcommand("version", "print the agent version", func(_ context.Context, args []string, sio IO) int {
		if len(args) != 0 {
			fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent version")
			return 2
		}
		fmt.Fprintf(sio.Stdout, "nself-ci-agent %s %s/%s\n", Version, runtime.GOOS, runtime.GOARCH)
		return 0
	}))
}
