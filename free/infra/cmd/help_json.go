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
// it): it drops --json and runs the real help path into a buffer, so the text
// is the human help byte for byte. A flag error in the arguments is not handled
// here; the normal path reports it. Only sdk/go/output writes the envelope.
package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/spf13/cobra"
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
	// Run the real help path (cobra adds its help flag and default commands
	// during Execute), so the text is what human --help prints, byte for byte.
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs(rest)
	err := root.Execute()
	root.SetOut(nil)
	root.SetArgs(args) // a caller that goes on to Execute must see the original arguments
	if err != nil || buf.Len() == 0 {
		return false, 0 // a flag error or similar: the normal path reports it
	}
	c, _, ferr := root.Find(rest)
	if ferr != nil || c == nil {
		c = root
	}
	name := c.CommandPath()
	if err := output.WriteData(stdout, name, map[string]any{"help": buf.String()}, nil); err != nil {
		fmt.Fprintln(stderr, "Error: writing the JSON envelope:", err)
		return true, output.ExitInfra
	}
	return true, 0
}
