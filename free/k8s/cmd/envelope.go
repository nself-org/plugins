// Purpose: the --json output path shared by every subcommand: one v1 envelope
// on stdout, written through sdk/go/output, and the exit status of its class.
//
// Inputs: the cobra command (its --json flag), the subcommand name, data or an
// error.
//
// Outputs: with --json, exactly one envelope on stdout and nothing else (helm's
// stdout is sent to stderr, tui lines are not printed, warnings travel in
// meta.warnings); without --json the run helpers print what the plugin always
// printed and return the error unchanged.
//
// Constraints: no hand-written envelope JSON, only sdk/go/output. The envelope
// command is "k8s <sub>". After an error envelope the helper returns exitErr,
// which main turns into the class exit status without printing anything more.
// Error text passed to the envelope never carries a secret, a licence key or
// helm's release JSON (see errcodes.go).
package main

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-k8s/internal/tui"
)

// exitErr ends the process with code. msg, when set, is printed by main as
// "Error: <msg>" on stderr (human mode); it is empty after an error envelope.
type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string { return e.msg }

// addJSONFlag registers --json on a subcommand.
func addJSONFlag(c *cobra.Command) {
	c.Flags().Bool("json", false, "Print one v1 JSON envelope on stdout (progress and helm output go to stderr)")
}

// invocation is the output context of one subcommand run.
type invocation struct {
	cmd      *cobra.Command
	sub      string // subcommand name, e.g. "install"
	json     bool
	warnings []string
}

// newInvocation reads --json and, in JSON mode, silences cobra's own error and
// usage printing so stdout and stderr stay clean.
func newInvocation(cmd *cobra.Command, sub string) *invocation {
	js, _ := cmd.Flags().GetBool("json")
	if js {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
	}
	return &invocation{cmd: cmd, sub: sub, json: js}
}

func (r *invocation) command() string { return "k8s " + r.sub }

// info prints a progress line (human mode only).
func (r *invocation) info(msg string) {
	if !r.json {
		tui.Info(msg)
	}
}

// success prints the success line (human mode only).
func (r *invocation) success(msg string) {
	if !r.json {
		tui.Success(msg)
	}
}

// warn prints a warning on stderr (human) or records it for meta.warnings (JSON).
func (r *invocation) warn(msg string) {
	if r.json {
		r.warnings = append(r.warnings, msg)
		return
	}
	tui.Warn(msg)
}

// helmStdout is where helm's standard output goes: stdout normally, stderr in
// JSON mode so stdout carries only the envelope.
func (r *invocation) helmStdout() io.Writer {
	if r.json {
		return os.Stderr
	}
	return os.Stdout
}

// done finishes a successful run: JSON mode writes the data envelope.
func (r *invocation) done(data any) error {
	if !r.json {
		return nil
	}
	if err := output.WriteData(r.cmd.OutOrStdout(), r.command(), data, r.meta()); err != nil {
		return &exitErr{code: output.ExitInfra, msg: "writing the JSON envelope: " + err.Error()}
	}
	return nil
}

// fail finishes a failed run. Human mode returns err unchanged (main prints
// "Error: <err>" and exits 1). JSON mode classifies err, writes the error
// envelope and returns the class exit status.
func (r *invocation) fail(err error) error {
	if !r.json {
		return err
	}
	return r.emit(classify(err))
}

// usageFail reports a usage error (E401): JSON mode writes the envelope; human
// mode returns the plain error, as before.
func (r *invocation) usageFail(msg string) error {
	if !r.json {
		return &plainError{msg}
	}
	return r.emit(usageDetail(msg))
}

// emit writes an error envelope and returns the matching exitErr.
func (r *invocation) emit(d output.ErrorDetail) error {
	d.ExitCode = output.ExitCodeFor(d.Class)
	if err := output.WriteError(r.cmd.OutOrStdout(), r.command(), d, r.meta()); err != nil {
		return &exitErr{code: output.ExitInfra, msg: "writing the JSON envelope: " + err.Error()}
	}
	return &exitErr{code: d.ExitCode}
}

func (r *invocation) meta() *output.Meta {
	if len(r.warnings) == 0 {
		return nil
	}
	return &output.Meta{Warnings: r.warnings}
}

// plainError is an error whose text is the message, as fmt.Errorf produced.
type plainError struct{ msg string }

func (e *plainError) Error() string { return e.msg }

// wantsJSON reports whether args carry --json (before any "--").
func wantsJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" || a == "--json=true" {
			return true
		}
	}
	return false
}
