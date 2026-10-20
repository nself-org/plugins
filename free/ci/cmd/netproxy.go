package main

// Purpose: the hidden `netproxy` key: the entrypoint of the per-attempt proxy sidecar and of the scope probe
// container (P7-TRUST-08, Epic D11). Both run this same static binary, bind-mounted read-only into a minimal
// image, so the proxy is never part of the agent or coordinator process.
// Inputs:  env NSELF_NETPROXY_CONFIG (the encoded netscope.Config); `--probe host:port ...` instead dials the
// targets and prints the ones that connected.
// Outputs: JSON event lines on stdout (ready, one per denial); exit 0 on a signalled stop, 2 on a bad config.
// Constraints: V15 only and Hidden, so help and the subcommand count do not show it and v1.4 argv stays frozen.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/nself-org/plugins/free/ci/internal/netscope"
)

func netproxyFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("netproxy", flag.ContinueOnError)
	fs.Bool("probe", false, "dial the targets and print the ones that connected")
	fs.String("proxy", "", "with --probe: the proxy to ask for a tunnel to the metadata address")
	return fs
}

func init() {
	registerV15("netproxy", subcommandEntry{V15Only: true, Hidden: true, Flags: netproxyFlags, Handler: runNetproxy})
}

func runNetproxy(args []string) int {
	fs := netproxyFlags()
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if fs.Lookup("probe").Value.String() == "true" {
		netscope.RunProbeMode(ctx, fs.Lookup("proxy").Value.String(), fs.Args(), os.Stdout)
		return 0
	}
	if err := netscope.RunSidecar(ctx, os.Getenv(netscope.ConfigEnv), os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "netproxy:", err)
		return 2
	}
	return 0
}
