package run

// Purpose: the platform and priority band an attempt asks the scheduler for (split from
// dispatch_world.go under the 300-line budget).
// Inputs:  the attempt's declared band, its run trigger and its explicit platform.
// Outputs: the band string and the platform string sched places with.

import (
	"runtime"

	"github.com/nself-org/plugins/free/ci/internal/fairq"
)

// platform is the local runner's platform string.
func platform() string { return runtime.GOOS + "/" + runtime.GOARCH }

// attemptBand is the priority band of an attempt: the band it declares, else the
// one its run's trigger gives (fairq.For). Every live attempt is built with
// Foreground set (the coordinator waits for it), so the flag says nothing about
// the run and is not used. Background is held back to normal until P7-SCHED-12
// can place background work: a background job today meets the interactive-
// queued and operator-idle rules of sched for its own attempt and would never run.
func attemptBand(declared, trigger string) string {
	b := fairq.For(declared, trigger, false)
	if b == fairq.Background {
		b = fairq.Normal
	}
	return b.String()
}

// wantPlatform is the platform an attempt asks the scheduler for: the one its job or matrix entry names
// explicitly (AttemptSpec.Platform), else the invoking host's own. An explicit platform steers placement
// to a node of that platform; with none the host's platform stays the default. A job nobody can take
// (no online node of that platform) is unplaceable with platform.mismatch on every alternative, which
// `ci explain` shows; it never runs on the wrong architecture.
func wantPlatform(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return platform()
}
