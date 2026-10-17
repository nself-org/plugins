// Package jsonout is the --json output path shared by every sentry-cli
// subcommand and by sentry-server.
//
// Purpose: write one v1 envelope on stdout through sdk/go/output, and end the
// process with the exit status of the error class. Commands that printed
// pre-contract JSON keep those bytes in v1.4 mode; the envelope replaces them
// only when compat.V15() is true. Commands that printed no JSON before are
// marked with Always and print the envelope in both modes.
//
// Inputs: the cobra command (its --json flag), data or an error.
//
// Outputs: with --json and an envelope mode, exactly one envelope on stdout and
// nothing else; progress and human text stay on stderr.
//
// Constraints: no hand-written envelope JSON, only sdk/go/output. The envelope
// command is the cobra path ("sentry monitors list"). Error text in an envelope
// is fixed text from errcodes.go: it never carries an API key, a token, an env
// value or a raw upstream error.
package jsonout

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-sentry-cli/internal/errs"
	"github.com/spf13/cobra"
)

// annotation marks a command whose --json output is the envelope in both modes.
const annotation = "nself.jsonout"

// ran records that a wrapped RunE started, so Execute can tell a cobra usage
// error (no command ran) from a command error.
var ran bool

// Always marks c as a command with no pre-contract JSON: its --json output is
// the envelope in v1.4 and v1.5 mode alike.
func Always(c *cobra.Command) {
	if c.Annotations == nil {
		c.Annotations = map[string]string{}
	}
	c.Annotations[annotation] = "always"
}

// AddFlag registers --json on a command that has none.
func AddFlag(c *cobra.Command, usage string) {
	c.Flags().Bool("json", false, usage)
}

// wanted reports whether --json was given on cmd.
func wanted(cmd *cobra.Command) bool {
	j, _ := cmd.Flags().GetBool("json")
	return j
}

// Envelope reports whether cmd prints the envelope for this run: --json is set
// and either the command has no pre-contract shape or v1.5 mode is on.
func Envelope(cmd *cobra.Command) bool {
	if !wanted(cmd) {
		return false
	}
	if cmd.Annotations[annotation] == "always" {
		return true
	}
	// compat.V15(P7-SURF-31): pre-contract JSON stays the v1.4 default -> v1 envelope
	return compat.V15()
}

// Done writes v as the success envelope of cmd.
func Done(cmd *cobra.Command, v any) error {
	if err := output.WriteData(cmd.OutOrStdout(), cmd.CommandPath(), v, nil); err != nil {
		return fmt.Errorf("write json output: %w", err)
	}
	return nil
}

// Data prints v for a command that has a pre-contract shape. legacy prints the
// v1.4 bytes; it runs unless the envelope applies.
func Data(cmd *cobra.Command, v any, legacy func() error) error {
	if Envelope(cmd) {
		return Done(cmd, v)
	}
	return legacy()
}

// Wrap replaces the RunE of every leaf command under root that has a --json
// flag, so a failed run in an envelope mode prints the error envelope and ends
// with the exit status of its class. Calling it twice on one tree is harmless.
func Wrap(root *cobra.Command) {
	for _, c := range root.Commands() {
		if len(c.Commands()) > 0 {
			Wrap(c)
			continue
		}
		if c.RunE == nil || c.Flags().Lookup("json") == nil || c.Annotations["nself.jsonout.wrapped"] != "" {
			continue
		}
		orig := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			ran = true
			err := orig(cmd, args)
			if err == nil || !Envelope(cmd) {
				return err
			}
			return fail(cmd, err)
		}
		if c.Annotations == nil {
			c.Annotations = map[string]string{}
		}
		c.Annotations["nself.jsonout.wrapped"] = "1"
	}
}

// fail prints the error envelope of err and returns the class exit error. An
// exit error means the command already reported what it found.
func fail(cmd *cobra.Command, err error) error {
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var ee *errs.ExitError
	if errors.As(err, &ee) {
		return err
	}
	d := Classify(err, cmd.CommandPath())
	if werr := output.WriteError(cmd.OutOrStdout(), cmd.CommandPath(), d, nil); werr != nil {
		cmd.SilenceErrors = false
		return err
	}
	return errs.Exit(output.ExitCodeFor(d.Class))
}

// hasJSONArg reports whether args turn --json on. Like pflag the last value
// wins (--json=false --json is true, --json --json=false is false), and
// everything after a bare "--" is a positional, not a flag.
func hasJSONArg(args []string) bool {
	js := false
	for _, a := range args {
		if a == "--" {
			break
		}
		if a == "--json" {
			js = true
			continue
		}
		if v, ok := strings.CutPrefix(a, "--json="); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				js = b
			}
		}
	}
	return js
}

// isAlways reports whether args resolve to a command marked with Always.
func isAlways(root *cobra.Command, args []string) bool {
	c, _, err := root.Find(args)
	return err == nil && c != nil && c.Annotations[annotation] == "always"
}

// Execute runs root with args and returns the process exit status. With --json
// it also turns a usage error (no command ran) into an envelope: in v1.5 mode
// for every command, in v1.4 mode for the Always commands only.
func Execute(root *cobra.Command, args []string) int {
	ran = false
	root.SetArgs(args)
	// compat.V15(P7-SURF-31): cobra prints usage errors -> one usage error envelope on stdout
	// (login, logout and sentry-server provision printed no JSON before: envelope in both modes)
	jsonMode := hasJSONArg(args) && (compat.V15() || isAlways(root, args))
	if jsonMode {
		root.SilenceErrors = true
		root.SilenceUsage = true
	}
	err := root.Execute()
	if err == nil {
		return 0
	}
	var ee *errs.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	if !jsonMode {
		return 1
	}
	if ran {
		// A command ran in human mode (--json was a flag value): keep the
		// plain error line the command would have printed.
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}
	path := root.CommandPath()
	if c, _, ferr := root.Find(args); ferr == nil && c != nil {
		path = c.CommandPath()
	}
	d := output.ErrorDetail{Code: CodeUsage, Class: output.ClassUser, Message: SafeUsage(err.Error()),
		Remediation: "run the command with --help for its usage"}
	if werr := output.WriteError(root.OutOrStdout(), path, d, nil); werr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	}
	return output.ExitUser
}
