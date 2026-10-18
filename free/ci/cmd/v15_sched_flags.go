package main

// Purpose: the per-run scheduling flags `--prefer <preset>`, `--runner <id>`
// and `--max-queue <duration>` on bare `nself ci` and `nself ci plan`
// (P7-SCHED-02, Epic D10).
// Inputs: argv, through the two package-level flag sets. Outputs: the
// revision-scope policy values the flags stand for (schedRevisionValues).
// Constraints: registered from init() on bareFlags and planFlags, so cmd/v15.go
// and cmd/plan.go stay unedited; a value is checked while the flags parse
// (an unknown preset or a bad duration is E691 and exit 2), before any run
// starts; this file loads and stores no policy.

import (
	"encoding/json"
	"flag"

	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

// schedFlags holds the flag values of the current invocation. The two flag
// sets never parse in one invocation, so they share it. A process runs one
// invocation; tests reset it.
var schedFlags schedpolicy.Flags

func init() {
	for _, fs := range []*flag.FlagSet{bareFlags, planFlags} {
		fs.Func("prefer", "scheduling preset for this run: "+presetList()+" (local = prefer-local)", func(v string) error {
			if _, err := schedpolicy.PresetFromFlag(v); err != nil {
				return err
			}
			schedFlags.Prefer = v
			return nil
		})
		fs.Func("runner", "place every job on this runner id (an exceptional override)", func(v string) error {
			if v == "" {
				return schedpolicy.CheckRunner(v)
			}
			schedFlags.Runner = v
			return nil
		})
		fs.Func("max-queue", "longest a job may wait for a runner, a duration such as 10m (default 10m for a foreground run)", func(v string) error {
			if _, err := schedpolicy.ParseDurationMs(schedpolicy.KeyMaxQueue, v); err != nil {
				return err
			}
			schedFlags.MaxQueue = v
			return nil
		})
	}
}

// clearSchedFlags forgets the flags of an earlier parse.
func clearSchedFlags() { schedFlags = schedpolicy.Flags{} }

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

// foregroundTrigger reports whether a run with this trigger is a person
// waiting at a terminal: the default (no trigger), local and manual runs. It
// is a heuristic: --trigger says which pipelines to plan, not who waits; the
// TTY test would be exact; parseBare wires the values.
func foregroundTrigger(trigger string) bool {
	return trigger == "" || trigger == "local" || trigger == "manual"
}

// schedRevisionValues returns the revision-scope policy values of the parsed
// flags, plus the 10 minute queue cap of a foreground run without --max-queue.
func schedRevisionValues(trigger string) (map[string]json.RawMessage, error) {
	f := schedFlags
	f.Foreground = foregroundTrigger(trigger)
	return f.Values()
}
