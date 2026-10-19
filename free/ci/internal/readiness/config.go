package readiness

// Purpose: the readiness settings and the floors rule. Config holds the resolved
// readiness.* policy values; Decide turns Config plus one reading into a Report.
// Inputs: Config, sample.Live, the hook outcome and the yield matches.
// Outputs: Report with one human reason per refusing floor.
// Constraints: pure (no I/O, no clock). A floor of 0 is switched off. A floor that is on
// and whose value could not be read refuses with its own reason (fail closed): the
// operator sets the floor to 0 to say "do not check this here".

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/readiness/sample"
)

// Config is the resolved readiness.* policy.
type Config struct {
	MinMemMB       int64
	MaxLoadPerCore float64
	MinIdlePct     float64
	MinDiskMB      int64
	// Cmd is the hook argv (readiness.cmd); empty means no hook.
	Cmd []string
	// YieldTo lists process names that make this host not ready while they run.
	YieldTo []string
}

// Default floors. Operators change them with the readiness.* keys.
const (
	DefaultMinMemMB       = 1024
	DefaultMaxLoadPerCore = 1.0
	DefaultMinIdlePct     = 20.0
	DefaultMinDiskMB      = 2048
)

// DefaultConfig returns the registered defaults.
func DefaultConfig() Config {
	return Config{MinMemMB: DefaultMinMemMB, MaxLoadPerCore: DefaultMaxLoadPerCore, MinIdlePct: DefaultMinIdlePct, MinDiskMB: DefaultMinDiskMB}
}

// HookTimeout is how long readiness.cmd may run.
const HookTimeout = 5 * time.Second

// Inputs is everything Decide needs besides Config.
type Inputs struct {
	Live      sample.Live
	Hook      Hook
	YieldedTo []string
	// YieldErr is set when yield_to is configured but the process list could not be read.
	YieldErr string
}

// Decide applies the floors. Memory, disk and idle refuse on their own. Load is
// idle-based: a high load average only refuses when the CPU is not demonstrably idle
// (many threads blocked on I/O push the average up without using the machine), so
// load 46 on 8 cores at 93% idle is ready, load 9 at 5% idle is not.
func Decide(c Config, in Inputs) Report {
	r := Report{Schema: SchemaID, Why: []string{}, YieldedTo: []string{},
		MemFreeMB: in.Live.MemFreeMB, LoadPerCore: in.Live.LoadPerCore, CPUIdlePct: in.Live.CPUIdlePct, DiskFreeMB: in.Live.DiskFreeMB}
	refuse := func(format string, a ...any) { r.Why = append(r.Why, fmt.Sprintf(format, a...)) }
	notes := in.Live.Notes

	if c.MinMemMB > 0 {
		switch {
		case in.Live.MemFreeMB == nil:
			refuse("free memory unreadable (%s)", noteOr(notes, "mem_free_mb"))
		case *in.Live.MemFreeMB < c.MinMemMB:
			refuse("free memory %d MB below the %d MB floor", *in.Live.MemFreeMB, c.MinMemMB)
		}
	}
	if c.MinDiskMB > 0 {
		switch {
		case in.Live.DiskFreeMB == nil:
			refuse("free disk unreadable (%s)", noteOr(notes, "disk_free_mb"))
		case *in.Live.DiskFreeMB < c.MinDiskMB:
			refuse("free disk %d MB below the %d MB floor", *in.Live.DiskFreeMB, c.MinDiskMB)
		}
	}
	idleOn := c.MinIdlePct > 0
	if idleOn {
		switch {
		case in.Live.CPUIdlePct == nil:
			refuse("CPU idle unreadable (%s)", noteOr(notes, "cpu_idle_pct"))
		case *in.Live.CPUIdlePct < c.MinIdlePct:
			refuse("CPU idle %.0f%% below the %.0f%% floor", *in.Live.CPUIdlePct, c.MinIdlePct)
		}
	}
	if c.MaxLoadPerCore > 0 {
		idleForgives := idleOn && in.Live.CPUIdlePct != nil && *in.Live.CPUIdlePct >= c.MinIdlePct
		switch {
		case in.Live.LoadPerCore == nil:
			refuse("load average unreadable (%s)", noteOr(notes, "load_per_core"))
		case *in.Live.LoadPerCore > c.MaxLoadPerCore && !idleForgives:
			refuse("load %.2f per core above the %.2f ceiling", *in.Live.LoadPerCore, c.MaxLoadPerCore)
		}
	}
	if len(in.YieldedTo) > 0 {
		r.YieldedTo = append(r.YieldedTo, in.YieldedTo...)
		sort.Strings(r.YieldedTo)
		refuse("yielding to %s", strings.Join(r.YieldedTo, ", "))
	}
	if in.YieldErr != "" {
		refuse("yield_to could not be checked (%s)", in.YieldErr)
	}
	if in.Hook.Ran && in.Hook.Result != HookReady {
		refuse("%s", in.Hook.Why)
	}
	r.Ready = len(r.Why) == 0
	return r
}

func noteOr(notes map[string]string, name string) string {
	if n := notes[name]; n != "" {
		return n
	}
	return "no reading"
}
