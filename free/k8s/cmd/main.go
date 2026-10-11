// Purpose: entry point for the standalone nself-k8s binary.
//
// Inputs: os.Args via cobra's rootCmd.Execute().
//
// Outputs: process exit code. Human mode: 0 on success, 1 on any error, with
// "Error: <msg>" on stderr, exactly as before. With --json: the exit class of
// the error envelope (user 1, infra 2, auth 3, destructive_blocked 4). The
// uninstall refusal exits 4 in both modes.
//
// Constraints: this file is package main in its own Go module, so — unlike
// the core CLI, where os.Exit is confined to cmd/nself/main.go — os.Exit here
// is the only sane way to report the exit code to the parent nself process
// that exec'd this binary (internal/plugin.ProxyCommand in the core CLI).
package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

func main() {
	js := wantsJSON(os.Args[1:])
	if js {
		// Flag and argument errors are cobra's, raised before any RunE: keep
		// cobra from printing them; report prints the envelope instead.
		rootCmd.SilenceErrors = true
		rootCmd.SilenceUsage = true
	}
	os.Exit(report(rootCmd.Execute(), js, os.Args[1:], os.Stdout, os.Stderr))
}

// report turns the result of Execute into the process exit code, printing the
// error where the mode says: "Error: <msg>" on stderr in human mode, an E401
// envelope on stdout in JSON mode (for errors cobra raised before a command
// ran; command errors in JSON mode were already written as envelopes).
func report(err error, js bool, args []string, stdout, stderr io.Writer) int {
	if err == nil {
		return 0
	}
	var ee *exitErr
	if errors.As(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(stderr, "Error:", ee.msg)
		}
		return ee.code
	}
	if js {
		d := usageDetail(err.Error())
		d.ExitCode = output.ExitCodeFor(d.Class)
		name := "k8s"
		if sub := subcommandOf(args); sub != "" {
			name += " " + sub
		}
		if werr := output.WriteError(stdout, name, d, nil); werr != nil {
			fmt.Fprintln(stderr, "Error:", err)
		}
		return d.ExitCode
	}
	fmt.Fprintln(stderr, "Error:", err)
	return 1
}

// subcommandOf returns the first non-flag argument that names a subcommand, or
// "" when none does (the envelope command is then "k8s").
func subcommandOf(args []string) string {
	for _, a := range args {
		for _, c := range rootCmd.Commands() {
			if a == c.Name() {
				return a
			}
		}
	}
	return ""
}
