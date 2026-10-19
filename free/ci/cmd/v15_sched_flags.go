package main

// Purpose: the per-run scheduling flags `--prefer <preset>`, `--runner <id>`
// and `--max-queue <duration>` on bare `nself ci` and `nself ci plan`
// (P7-SCHED-02, Epic D10).
// Inputs: argv, through the two package-level flag sets. Outputs: the
// per-run policy values the flags stand for (schedRevisionValues).
// Constraints: registered from init() on bareFlags and planFlags, so cmd/v15.go
// and cmd/plan.go stay unedited; a value is checked while the flags parse
// (an unknown preset or a bad duration is E691 and exit 2), before any run
// starts; this file loads and stores no policy.

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// schedFlags holds the flag values of the current invocation. The two flag
// sets never parse in one invocation, so they share it. A process runs one
// invocation; tests reset it.
var schedFlags schedpolicy.Flags

func init() {
	RegisterOptionsHook(schedOptions)
	for _, fs := range []*flag.FlagSet{bareFlags, planFlags} {
		fs.Func("prefer", "scheduling preset for this run: "+presetList()+" (local = prefer-local); bare runs only", func(v string) error {
			if _, err := schedpolicy.PresetFromFlag(v); err != nil {
				return err
			}
			schedFlags.Prefer = v
			return nil
		})
		fs.Func("runner", "place every job on this runner id (an exceptional override); bare runs only", func(v string) error {
			if v == "" {
				return schedpolicy.CheckRunner(v)
			}
			schedFlags.Runner = v
			return nil
		})
		fs.Func("max-queue", "longest a job may wait for a runner, a duration such as 10m (an interactive run defaults to 10m); bare runs only", func(v string) error {
			if _, err := schedpolicy.ParseDurationMs(schedpolicy.KeyMaxQueue, v); err != nil {
				return err
			}
			schedFlags.MaxQueue = v
			return nil
		})
	}
}

func presetList() string {
	out := ""
	for i, n := range schedpolicy.PresetNames() {
		if i > 0 {
			out += ", "
		}
		out += n
	}
	return out
}

// stdinTerminal reports whether stdin is a terminal (isTerminal: a pipe, a file,
// /dev/null and /dev/zero are not). A seam for tests.
var stdinTerminal = func() bool { return isTerminal(os.Stdin) }

// interactiveRun reports whether a person waits at a terminal: stdin is a
// terminal, no --trigger names who started the run and NSELF_INVOKED_BY (a
// wrapper or scheduler may set it to say a script started the run) is empty.
// It replaces the trigger-string guess for real runs (P7-AUTO-29); only an
// interactive run keeps the dispatcher's 10 minute foreground queue cap.
func interactiveRun(trigger string, getenv func(string) string) bool {
	return trigger == "" && getenv("NSELF_INVOKED_BY") == "" && stdinTerminal()
}

// schedOptions turns the parsed flags into the per-run policy values of the
// run. The 10 minute foreground queue cap is not one of them: the dispatcher
// applies it when the policy sets no max_queue and the run is interactive
// (o.Interactive, set by parseBare before the hooks run), so it never shortens
// a longer limit an operator or project set.
func schedOptions(o *RunOptions) error {
	vals, err := schedRevisionValues()
	o.PolicyRun = vals
	return err
}

// foregroundTrigger reports whether a run with this trigger is a person
// waiting at a terminal: the default (no trigger), local and manual runs. It
// is a heuristic: --trigger says which pipelines to plan, not who waits; the
// `ci plan` still uses it (cmd/plan_placement.go); a real run uses interactiveRun.
func foregroundTrigger(trigger string) bool {
	return trigger == "" || trigger == "local" || trigger == "manual"
}

// schedRevisionValues returns the per-run policy values of the parsed flags
// (the run scope, internal/policy/runscope.go); nil when no flag was given.
func schedRevisionValues() (map[string]json.RawMessage, error) {
	return schedFlags.Values()
}
