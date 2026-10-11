// Purpose: the --json output path shared by every infra subcommand: one v1
// envelope on stdout, written through sdk/go/output, and the exit status of
// its class.
//
// Inputs: the cobra command (its --json flag), data or an error.
//
// Outputs: with --json, exactly one envelope on stdout and nothing else:
// internal/infra points terraform's stdout at os.Stdout when it runs, so for the
// length of the run os.Stdout is pointed at os.Stderr and the envelope goes to
// the real stdout saved before. Without --json the helpers print what the plugin
// always printed and return the error unchanged.
//
// Constraints: no hand-written envelope JSON, only sdk/go/output. The envelope
// command is the cobra path without the binary word: "infra plan",
// "infra server list". After an error envelope the helper returns exitErr, which
// main turns into the class exit status without printing anything more. Error
// text passed to the envelope never carries a token, an env value or a raw
// provider or terraform error (see errcodes.go).
package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nself-org/cli/sdk/go/v2/output"
	"github.com/nself-org/nself-infra/internal/ui"
)

// exitErr ends the process with code. msg, when set, is printed by main as
// "Error: <msg>" on stderr; it is empty after an error envelope or a silent
// refusal. cause keeps the underlying error for errors.Is and for tests; it is
// never printed.
type exitErr struct {
	code  int
	msg   string
	cause error
	diag  string // redacted diagnostic for stderr after an error envelope; never in the envelope
}

func (e *exitErr) Error() string {
	if e.msg == "" && e.cause != nil {
		return e.cause.Error()
	}
	return e.msg
}

func (e *exitErr) Unwrap() error { return e.cause }

// addJSONFlag registers --json on a subcommand.
func addJSONFlag(c *cobra.Command) {
	c.Flags().Bool("json", false, "Print one v1 JSON envelope on stdout (progress lines and terraform output go to stderr)")
}

// invocation is the output context of one subcommand run.
type invocation struct {
	cmd   *cobra.Command
	json  bool
	out   io.Writer // where the envelope goes: the stdout of the process at begin
	saved *os.File  // os.Stdout before the redirect, nil when not redirected
	sub   string    // "plan", "server list": the command path after "infra"
}

// ran records that a subcommand's RunE started, so main can tell a cobra usage
// error (no command ran) from a command error.
var ran bool

// begin reads --json. In JSON mode it silences cobra's own error and usage
// printing and redirects os.Stdout to os.Stderr until end.
func begin(cmd *cobra.Command) *invocation {
	js, _ := cmd.Flags().GetBool("json")
	ran = true
	sub := strings.TrimPrefix(strings.TrimPrefix(cmd.CommandPath(), "infra"), " ")
	r := &invocation{cmd: cmd, json: js, out: os.Stdout, sub: sub}
	if js {
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		r.saved = os.Stdout
		os.Stdout = os.Stderr
	} else {
		// main silenced cobra because the arguments looked like --json (for
		// example a flag whose value is "--json"). The command runs in human
		// mode: restore cobra's own error printing.
		cmd.SilenceErrors = false
		infraCmd.SilenceErrors = false
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

func (r *invocation) command() string {
	if r.sub == "" {
		return "infra"
	}
	return "infra " + r.sub
}

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

// fail finishes a failed run. Human mode returns err unchanged (main prints
// "Error: <err>" and exits 1). JSON mode classifies err, writes the error
// envelope and returns the class exit status.
func (r *invocation) fail(err error) error {
	if !r.json {
		return err
	}
	return r.emit(classify(err, r.sub), err)
}

// usage reports a usage error (E401): JSON mode writes the envelope; human
// mode returns the plain error, as before.
func (r *invocation) usage(msg string) error {
	return r.fail(&usageError{msg: msg})
}

// usageSafe is usage for a message that echoes a flag value: human mode prints
// msg, the envelope carries safe.
func (r *invocation) usageSafe(msg, safe string) error {
	return r.fail(&usageError{msg: msg, safe: safe})
}

// refuse reports a refused destructive or spending action. Human mode prints
// warn on stderr and exits 4 with nothing more; JSON mode writes an E403
// envelope (exit 4).
func (r *invocation) refuse(warn, msg, remediation string) error {
	if r.json {
		return r.emit(blockedDetail(msg, remediation), nil)
	}
	ui.Warn(warn)
	return r.silentExit(output.ExitDestructiveBlocked)
}

// silentExit ends the process with code and no further output from cobra or main.
func (r *invocation) silentExit(code int) error {
	r.cmd.SilenceErrors = true
	return &exitErr{code: code}
}

// emit writes an error envelope and returns the matching exitErr.
func (r *invocation) emit(d output.ErrorDetail, cause error) error {
	d.ExitCode = output.ExitCodeFor(d.Class)
	if err := output.WriteError(r.out, r.command(), d, nil); err != nil {
		return &exitErr{code: output.ExitInfra, msg: "writing the JSON envelope: " + err.Error()}
	}
	return &exitErr{code: d.ExitCode, cause: cause, diag: r.diagnostic(d, cause)}
}

// diagnostic is the stderr line for a failure that has no tool output of its
// own (terraform prints its own): the Hetzner, network or local error text,
// redacted. It never enters the envelope.
func (r *invocation) diagnostic(d output.ErrorDetail, cause error) string {
	switch d.Code {
	case CodeHetznerFailed, CodeServerNotFound, CodeTokenRejected, CodeUnexpected:
	default:
		return ""
	}
	var ee *exec.ExitError
	if cause == nil || errors.As(cause, &ee) {
		return ""
	}
	secrets := []string{}
	if v, _ := r.cmd.Flags().GetString("token"); v != "" {
		secrets = append(secrets, v)
	}
	for _, name := range []string{"HETZNER_NSELF_TOKEN", "HCLOUD_TOKEN", TerraformTokenEnvVar} {
		secrets = append(secrets, os.Getenv(name))
	}
	if name, _ := r.cmd.Flags().GetString("token-env"); name != "" {
		secrets = append(secrets, os.Getenv(name))
	}
	return redact(cause.Error(), secrets)
}

// redact removes every secret value and drops everything from the first
// "token", "Bearer" or "Authorization" (any case) to the end.
func redact(text string, secrets []string) string {
	for _, s := range secrets {
		if s != "" {
			text = strings.ReplaceAll(text, s, "[redacted]")
		}
	}
	low := strings.ToLower(text)
	cut := len(text)
	for _, w := range []string{"token", "bearer", "authorization"} {
		if i := strings.Index(low, w); i >= 0 && i < cut {
			cut = i
		}
	}
	if cut < len(text) {
		text = strings.TrimRight(text[:cut], " :,;-") + " [redacted]"
	}
	return text
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
		// Every form pflag accepts for a bool: 1, t, T, TRUE, true, True, 0, f, ...
		if v, ok := strings.CutPrefix(a, "--json="); ok {
			if b, err := strconv.ParseBool(v); err == nil {
				js = b
			}
		}
	}
	return js
}
