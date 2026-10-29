package main

// Purpose: the machine-invocation default of a bare V15 run (P7-CI-62, CI D29): a run started by a machine caller
// (the cli sets NSELF_INVOKED_BY for MCP and HTTP calls) posts no commit status unless `--post-status` is given.
// Inputs:  env NSELF_INVOKED_BY; the `--post-status` bare flag.
// Outputs: RunOptions.NoStatus set, with a one-line reason on stderr; nothing changes when NSELF_INVOKED_BY is unset
//          or `--post-status` is present.
// Constraints: an explicit `--no-status` still wins; the hook only ever turns status posting off.

import (
	"fmt"
	"io"
	"os"
)

var (
	postStatusFlag bool
	machineNotice  io.Writer = os.Stderr
)

func init() {
	bareFlags.BoolVar(&postStatusFlag, "post-status", false, "post the nself-ci commit status even when a machine caller (MCP, HTTP) started the run")
	RegisterOptionsHook(machineStatusDefault)
}

// machineStatusDefault applies the D29 default after flag parsing.
func machineStatusDefault(o *RunOptions) error {
	post := postStatusFlag
	postStatusFlag = false
	by := os.Getenv("NSELF_INVOKED_BY")
	if by == "" || post || o.NoStatus {
		return nil
	}
	o.NoStatus = true
	fmt.Fprintf(machineNotice, "nself ci: no commit status posted for a run invoked by %s (pass --post-status to post it)\n", by)
	return nil
}
