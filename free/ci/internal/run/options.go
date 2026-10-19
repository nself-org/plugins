package run

// Purpose: the options of a bare `nself ci` run. cmd/v15.go parses flags
// into Options, applies the registered option hooks and hands the value to
// the Coordinator; nothing here reads the environment or argv.
// Inputs:  parsed flags. Outputs: validated Options.
// Constraints: unknown trust or network values are E627; --sha is E624.

import (
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Options is everything a run needs from its caller.
type Options struct {
	Root      string   // repository root (project key)
	Home      string   // state directory; "" means NSELF_CI_HOME or ~/.nself/ci
	Pipeline  string   // --pipeline
	Job       string   // --job: run only this job and its dependencies
	Trigger   string   // --trigger
	JSON      bool     // --json
	Stream    bool     // --stream
	Container bool     // --container: run jobs in the toolchain container
	Services  []string // --service (repeatable)
	Network   string   // --network
	Force     bool     // --force: skip the capacity reservations
	NoStatus  bool     // --no-status
	Timeout   time.Duration
	Trust     string   // --trust
	SHA       string   // --sha: refused
	Adhoc     []string // `-- <cmd...>`
	Renderer  string   // "human" unless a registered renderer is named
	Full      bool     // plan every job: no affected-only selection (evidence verify must run the whole pipeline)
	// ConfirmFlaky reruns a job that failed with class code up to N more times, explicitly
	// and recorded as retries (P7-CI-40). The failed result never changes; 0 is off.
	ConfirmFlaky int
	// Record marks a commit CI run of record (set by `ci runs rerun <sha>` and by watch);
	// false for bare and dev runs. The dispatch hook maps it to sched.Pipeline.Record (P7-AUTO-27).
	Record bool
	// Interactive is true for a person waiting at a terminal: stdin is a terminal, no --trigger
	// and no NSELF_INVOKED_BY. Only an interactive run gets the 10 minute foreground queue cap.
	Interactive bool
	// PolicyRun carries per-run policy values into PolicyRequest.Run (filled by P7-AUTO-29 from
	// `--where`, `--prefer` and `--runner`); nil means none.
	PolicyRun RunValues
	// PolicyRunDefaulted lists the PolicyRun keys the caller filled by default (the foreground
	// queue cap), not the person: a refusal of one is not reported.
	PolicyRunDefaulted []string
}

// RunOptions is the name the cmd layer and its option hooks use.
type RunOptions = Options

var networks = map[string]bool{"": true, "none": true, "internet": true, "restricted": true, "lan": true}

// Validate refuses options that can never run. hooks supplies the registered
// classifier and ad hoc handler.
func (o Options) Validate(h Hooks) error {
	if o.SHA != "" {
		return fail(CodeSHA, "--sha is not supported; use `nself ci evidence verify %s`", o.SHA)
	}
	if o.Trust != "" {
		if !model.TrustClass(o.Trust).Valid() {
			return fail(CodeOptions, "unknown --trust class %q", o.Trust)
		}
		if h.Trust == nil {
			return fail(CodeTrust, "--trust %s needs a trust classifier and none is registered", o.Trust)
		}
	}
	if !networks[o.Network] {
		return fail(CodeOptions, "unknown --network scope %q (none, internet, restricted, lan)", o.Network)
	}
	if o.Timeout < 0 {
		return fail(CodeOptions, "--timeout must not be negative")
	}
	if len(o.Adhoc) > 0 && h.Adhoc == nil {
		return fail(CodeOptions, "`nself ci -- <cmd>` is not available in this build")
	}
	if len(o.Adhoc) > 0 && (o.Pipeline != "" || o.Job != "") {
		return fail(CodeOptions, "--pipeline and --job do not combine with `-- <cmd>`")
	}
	return nil
}
