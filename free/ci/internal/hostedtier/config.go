package hostedtier

// Purpose: defaults and the effective mode of the hosted tier (P7-AUTO-07).
// Inputs:  a Config resolved from the operator-scope keys.
// Outputs: Normalize, effective.
// Constraints: paid maps onto the SCHED-17 spend keys through PaidAllowed only; no new spend path.

import "time"

// Normalize fills defaults. A non-positive timer takes its default; an unknown mode reads as free.
func (c Config) Normalize() Config {
	if c.Mode != ModeOff && c.Mode != ModePaid {
		c.Mode = ModeFree
	}
	if c.Context == "" {
		c.Context = DefaultContext
	}
	if c.AppID == 0 {
		c.AppID = GitHubActionsAppID
	}
	c.Queued = orDefault(c.Queued, DefaultQueued)
	c.Running = orDefault(c.Running, DefaultRunning)
	c.Absent = orDefault(c.Absent, DefaultAbsent)
	if len(c.Residual) == 0 {
		c.Residual = []string{DefaultResidualID}
	}
	return c
}

func orDefault(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

// effective returns the mode that applies and the notes explaining a downgrade: paid without
// PaidAllowed behaves as free and says so.
func (c Config) effective() (Mode, []string) {
	if c.Mode == ModePaid && !c.PaidAllowed {
		return ModeFree, []string{NotePaidDenied}
	}
	return c.Mode, nil
}
