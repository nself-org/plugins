package main

// Purpose: the --json output path shared by every plugin-dev subcommand: one v1
// envelope on stdout, written through sdk/go/output, and the exit status of its
// class.
// Inputs:  the subcommand name, whether --json is set, data or an error.
// Outputs: with --json, exactly one envelope on stdout and nothing else: for the
// length of the run os.Stdout points at os.Stderr (status lines, child process
// output and the launch.json snippet go there) and the envelope goes to the
// stdout saved before. Without --json nothing changes: the human lines and the
// "Error: <msg>" report are the ones core printed.
// Constraints: no hand-written envelope JSON, only sdk/go/output. plugin-dev had
// no --json before this change, so the envelope is printed in v1.4 and v1.5 mode
// alike (nothing to gate). The envelope command is "plugin-dev <subcommand>".
// Error text in the envelope is fixed text from errcodes.go: it never echoes a
// path, a flag value, an environment value or a raw child-process error.

import (
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"
)

// invocation is the output context of one subcommand run.
type invocation struct {
	name  string    // subcommand name as typed ("init", "link"), aliases resolved
	json  bool      // --json is set
	out   io.Writer // where the envelope goes: the stdout of the process at begin
	saved *os.File  // os.Stdout before the redirect, nil when not redirected
}

// begin starts a run. In JSON mode os.Stdout is pointed at os.Stderr until end.
func begin(name string, js bool) *invocation {
	r := &invocation{name: name, json: js, out: os.Stdout}
	if js {
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

// command is the envelope command word: "plugin-dev init".
func (r *invocation) command() string {
	if r.name == "" {
		return "plugin-dev"
	}
	return "plugin-dev " + r.name
}

// done finishes a successful run: JSON mode writes the data envelope. It returns
// the process exit status.
func (r *invocation) done(data any) int {
	if !r.json {
		return 0
	}
	if err := output.WriteData(r.out, r.command(), data, nil); err != nil {
		return output.ExitInfra
	}
	return 0
}

// fail finishes a failed run and returns the exit status. Human mode prints the
// core error line (status 1, or the child's own status). JSON mode classifies
// err, writes the error envelope and returns the class status.
func (r *invocation) fail(err error) int {
	if !r.json {
		return fail(err)
	}
	d := classify(err)
	d.ExitCode = output.ExitCodeFor(d.Class)
	if werr := output.WriteError(r.out, r.command(), d, nil); werr != nil {
		return output.ExitInfra
	}
	return d.ExitCode
}

// wantsJSON reports whether argv carries --json (before any "--"). Like pflag,
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
