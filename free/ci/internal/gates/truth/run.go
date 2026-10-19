//go:build ignore

// The truth check runner: `go run -mod=vendor ./internal/gates/truth/run.go check ...`
// from free/ci. It is a thin main over truth.Main so the logic stays testable;
// the plugin binary also links the check as the V15 `truth` subcommand (D-1240).
package main

import (
	"os"

	"github.com/nself-org/plugins/free/ci/internal/gates/truth"
)

func main() { os.Exit(truth.Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
