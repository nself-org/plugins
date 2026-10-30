// Package selftest backs `nself ci doctor --selftest [--alert]` and
// `nself ci runs prune --tidy` (P7-AUTO-20): the health checks of an unattended
// host, the once-a-day alert, and the tidy plan that frees disk.
//
// Purpose: turn observations of one host (tools, login, disk, notifier counters,
// watcher liveness, claims, leftover job roots, infrastructure failures) into
// doctor checks CI-DS001… and decide whether an alert is due.
// Inputs:  Facts, gathered by Local or built by a test; Limits.
// Outputs: doctor.Check values; one notifier message per failing set per day.
// Constraints: Evaluate is pure (no clock, no I/O). A fact that could not be read
// is a warning naming the cause, never a pass. Nothing here removes anything
// except Tidy.Apply, and only when the caller asks for it.
package selftest

import "time"

// Tool is one gate tool looked up on the explicit PATH.
type Tool struct {
	Name     string
	Path     string
	Found    bool
	Required bool // a missing required tool fails the check, an optional one warns
}

// Provider is the provider login and API reachability.
type Provider struct {
	LoggedIn  bool
	Reachable bool
	Err       string // why the state could not be read
}

// Probe is one pass/fail observation with the reason.
type Probe struct {
	OK  bool
	Err string
}

// Disk is the free space under the CI home.
type Disk struct {
	FreeMB int64
	Err    string
}

// Sink holds one notifier sink's delivery counters (P7-AUTO-18).
type Sink struct {
	Name         string
	Delivered    int64
	Failed       int64
	LastFailedAt time.Time // zero: never failed
}

// Sinks are the counters of every sink.
type Sinks struct {
	List []Sink
	Err  string
}

// Watcher is the demand file of the watcher on this host.
type Watcher struct {
	Present  bool // a demand file exists
	Age      time.Duration
	Interval time.Duration // the watcher's pass interval; 0 means the default
	Err      string
}

// Claim is a claim of this host that has not seen a status since Age ago.
type Claim struct {
	Ref string // repo@sha
	Age time.Duration
}

// Claims are the open claims of this host.
type Claims struct {
	Known    bool // false: no claim source on this host
	List     []Claim
	RunLimit time.Duration
	Err      string
}

// Orphans are the job-root folders no live attempt owns (TRUST-09).
type Orphans struct {
	List []string
	Err  string
}

// Infra counts finished runs of the last 24 hours that ended in an infrastructure error.
type Infra struct {
	Count, Total int
	Err          string
}

// Facts is everything one selftest looks at.
type Facts struct {
	Now      time.Time
	Tools    []Tool
	Provider Provider
	Repo     Probe
	Disk     Disk
	Sinks    Sinks
	Watcher  Watcher
	Claims   Claims
	Orphans  Orphans
	Infra    Infra
}

// Limits are the thresholds the checks apply.
type Limits struct {
	MinFreeMB     int64         // below this the disk check fails; twice this warns
	WatcherPasses int           // demand file older than this many intervals fails
	InfraWarn     int           // infrastructure failures in 24 h that warn
	InfraFail     int           // infrastructure failures in 24 h that fail
	SinkWindow    time.Duration // a sink failure newer than this warns
}

// DefaultInterval is the watcher interval assumed when the demand file names none.
const DefaultInterval = 5 * time.Minute

// DefaultLimits returns the shipped thresholds.
func DefaultLimits() Limits {
	return Limits{MinFreeMB: 2048, WatcherPasses: 3, InfraWarn: 1, InfraFail: 3, SinkWindow: 24 * time.Hour}
}
