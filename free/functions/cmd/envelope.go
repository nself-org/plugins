package main

// Purpose: the --json output path shared by every functions subcommand: one v1
// envelope on stdout, written through sdk/go/output, and the exit status of its
// class.
//
// Inputs: the cobra command (its --json flag), data or an error.
//
// Outputs: with --json, exactly one envelope on stdout and nothing else: for the
// length of the run os.Stdout is pointed at os.Stderr and the envelope goes to
// the real stdout saved before. Without --json the helpers print what the plugin
// always printed and return the error unchanged.
//
// Constraints: no hand-written envelope JSON, only sdk/go/output. The envelope
// command is the cobra path with the binary word kept: "functions list". After
// an error envelope the helper returns exitErr, which report turns into the
// class exit status without printing anything more.
import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// exitErr ends the process with code; it is returned after an error envelope
// was written.
// msg, when set, is printed by report as "Error: <msg>" on stderr; it is empty
// after an error envelope. cause keeps the underlying error for errors.Is and
// tests; it is never printed.
type exitErr struct {
	code  int
	msg   string
	cause error
}

func (e *exitErr) Error() string {
	if e.msg != "" {
		return e.msg
	}
	if e.cause != nil {
		return e.cause.Error()
	}
	return "exit " + strconv.Itoa(e.code)
}

func (e *exitErr) Unwrap() error { return e.cause }

// addJSONFlag registers --json on a subcommand.
func addJSONFlag(c *cobra.Command) {
	c.Flags().Bool("json", false, "Print one v1 JSON envelope on stdout (progress text goes to stderr)")
}

// invocation is the output context of one subcommand run.
type invocation struct {
	cmd   *cobra.Command
	json  bool
	out   io.Writer // where the envelope goes: the stdout of the process at begin
	saved *os.File  // os.Stdout before the redirect, nil when not redirected
}

// ran records that a subcommand's RunE started, so report can tell a cobra usage
// error (no command ran) from a command error.
var ran bool

// begin reads --json. In JSON mode it redirects os.Stdout to os.Stderr until
// end. legacy true keeps the pre-contract output of a command: --json is then
// ignored here and the command prints its own bytes.
func begin(cmd *cobra.Command, legacy bool) *invocation {
	js, _ := cmd.Flags().GetBool("json")
	ran = true
	r := &invocation{cmd: cmd, json: js && !legacy, out: os.Stdout}
	if r.json {
		r.saved = os.Stdout
		os.Stdout = os.Stderr
	}
	return r
}

// end restores os.Stdout. Call it with defer right after begin.
func (r *invocation) end() {
	if r.saved != nil {
		os.Stdout = r.saved
		r.saved = nil
	}
}

func (r *invocation) command() string { return r.cmd.CommandPath() }

// done finishes a successful run: JSON mode writes the data envelope.
func (r *invocation) done(data any) error {
	if !r.json {
		return nil
	}
	if err := output.WriteData(r.out, r.command(), data, nil); err != nil {
		return &exitErr{code: output.ExitInfra, msg: "writing the JSON envelope: " + err.Error()}
	}
	return nil
}

// fail finishes a failed run. Human mode returns err unchanged (report prints
// "Error: <err>" and exits 1). JSON mode classifies err, writes the error
// envelope and returns the class exit status.
func (r *invocation) fail(err error) error {
	if !r.json {
		return err
	}
	return writeErrorEnvelope(r.out, r.command(), classify(err), err)
}

// usage reports a usage error (E401).
func (r *invocation) usage(msg string) error {
	return r.fail(&usageError{msg: msg})
}

// usageSafe is usage for a message that echoes a flag value: human mode prints
// msg, the envelope carries safe.
func (r *invocation) usageSafe(msg, safe string) error {
	return r.fail(&usageError{msg: msg, safe: safe})
}

// writeErrorEnvelope writes the error envelope for d and returns the exitErr
// with the class status.
func writeErrorEnvelope(w io.Writer, command string, d output.ErrorDetail, cause error) error {
	d.ExitCode = output.ExitCodeFor(d.Class)
	if err := output.WriteError(w, command, d, nil); err != nil {
		return &exitErr{code: output.ExitInfra, msg: "writing the JSON envelope: " + err.Error()}
	}
	return &exitErr{code: d.ExitCode, cause: cause}
}

// wantsJSON reports whether args carry --json (before any "--"). Like pflag,
// the last value wins: --json=false --json is true.
func wantsJSON(args []string) bool {
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

// report turns the result of Execute into the process exit code. A command that
// already wrote an error envelope arrives as exitErr. With --json, an error
// cobra raised before a command ran (unknown flag, wrong argument count) becomes
// an E401 envelope on stdout. Otherwise the plain error goes to stderr, exit 1.
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
	if js && !ran {
		name := "functions"
		if c, _, ferr := rootCmd.Find(args); ferr == nil && c != rootCmd {
			name = c.CommandPath()
		}
		if werr := writeErrorEnvelope(stdout, name, usageDetail(safeUsage(err.Error())), err); werr != nil {
			fmt.Fprintln(stderr, "Error:", safeUsage(err.Error()))
		}
		return output.ExitUser
	}
	fmt.Fprintln(stderr, "Error:", err)
	return 1
}
