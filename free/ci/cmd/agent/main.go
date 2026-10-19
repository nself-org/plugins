// Command nself-ci-agent is the small node binary of the CI fabric (ADR 0024 §2):
// it runs leased jobs on a host and speaks runner protocol v1.
//
// Purpose: dispatch to the registered subcommands (version, exec, serve here;
// join, pull, advertise, upgrade, verify-release and job-shim from their own
// files). It is a composition root: it wires internal/agent/*, internal/exec
// and internal/evidence and holds no logic of its own.
// Inputs:  argv; $NSELF_CI_AGENT_HOME for the state dir.
// Outputs: exit code 0 ok, 1 failed, 2 usage.
// Constraints: CGO_ENABLED=0 builds for linux/amd64, linux/arm64, darwin/arm64.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := run(ctx, os.Args[1:], IO{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr})
	stop()
	os.Exit(code)
}

func run(ctx context.Context, args []string, sio IO) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(sio.Stderr, "usage: nself-ci-agent <command> [args]")
		for _, l := range usageLines() {
			fmt.Fprintln(sio.Stderr, l)
		}
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	fn, ok := lookup(args[0])
	if !ok {
		fmt.Fprintf(sio.Stderr, "nself-ci-agent: unknown command %q\n", args[0])
		return 2
	}
	return fn(ctx, args[1:], sio)
}

// stateDir is $NSELF_CI_AGENT_HOME, else ~/.nself/ci-agent (user mode). System
// mode (/var/lib/nself-ci-agent) sets the variable in its service unit (P7-NODE-06).
func stateDir() (string, error) {
	if v := os.Getenv("NSELF_CI_AGENT_HOME"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".nself", "ci-agent"), nil
}
