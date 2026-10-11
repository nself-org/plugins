// Purpose: `--help --json` prints one v1 envelope instead of plain help text,
// so a machine caller gets the same shape from every command (core does the
// same for `nself help --json`).
//
// Inputs: the raw arguments. --json together with --help or -h, in any
// position, for the root or any subcommand.
//
// Outputs: exactly one ok envelope on stdout: command = the cobra path without
// the binary word ("infra", "infra plan", "infra server list"), data =
// {"help": <the exact text human --help prints>}, exit 0, nothing else. Human
// --help is untouched: this file never runs without --json.
//
// Constraints: a hook inside cobra's help function is not enough. The root has
// no --json flag, so cobra rejects `--help --json` as an unknown flag before it
// reaches help, and the E401 envelope would win. The check therefore runs on the
// raw arguments before the normal Execute (main and the test harness both call
// it). It never runs a command: it resolves the target with Find, parses that
// command's flags, and answers only if the parsed help flag is true, so "-h" as
// the value of another flag (--domain -h) is not a help request and no RunE can
// run here. Anything else (flag error, unknown command) is left to the normal
// path. The text comes from the command's own Help, byte for byte what human
// --help prints. Only sdk/go/output writes the envelope.
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// wantsHelp reports whether args ask for help (--help or -h, before any "--").
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--help" || a == "-h" {
			return true
		}
	}
	return false
}

// helpJSON handles `--help --json`. It returns handled=false (and writes
// nothing) unless args carry both; otherwise it writes the envelope to stdout
// and returns the process exit code.
func helpJSON(root *cobra.Command, args []string, stdout, stderr io.Writer) (handled bool, code int) {
	if !wantsHelp(args) || !wantsJSON(args) {
		return false, 0
	}
	rest := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--json" || strings.HasPrefix(a, "--json=") {
			continue
		}
		rest = append(rest, a)
	}
	// Resolve the command the way Execute would, without running it: add the
	// default help command and flag, find the target, parse its flags, and treat
	// the request as help only if the parsed help flag is true. "-h" as the value
	// of another flag (--domain -h) leaves help false, so nothing here can reach a
	// RunE and the normal path handles the arguments.
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd(rest...)
	c, flags, err := root.Find(rest)
	if err != nil || c == nil {
		return false, 0
	}
	c.InitDefaultHelpFlag()
	help := false
	if c.ParseFlags(flags) == nil {
		help, _ = c.Flags().GetBool("help")
	}
	resetFlags(c) // the normal path parses the same flags again; slices would append twice
	if !help {
		return false, 0
	}
	var buf bytes.Buffer
	c.SetOut(&buf)
	err = c.Help()
	c.SetOut(nil)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return true, output.ExitInfra
	}
	name := c.CommandPath()
	if err := output.WriteData(stdout, name, map[string]any{"help": buf.String()}, nil); err != nil {
		fmt.Fprintln(stderr, "Error: writing the JSON envelope:", err)
		return true, output.ExitInfra
	}
	return true, 0
}

// resetFlags puts every flag of c back to its default, so a parse done only to
// look for --help leaves no trace.
func resetFlags(c *cobra.Command) {
	c.Flags().VisitAll(func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			_ = sv.Replace(nil)
		} else {
			_ = f.Value.Set(f.DefValue)
		}
		f.Changed = false
	})
}
