package hostedtier

// Purpose: the decision matrix (P7-AUTO-07): from the check runs of a commit, use the hosted
//          result, wait for it, or hand over to local nodes.
// Inputs:  ctx, the commit sha, the time the commit was pushed.
// Outputs: Result.
// Constraints: reads only; timers are keyed to the check run's own timestamps; a red hosted result
//          stays red; a billing-blocked annotation marks the provider unavailable and is never a
//          code failure; a run of another app or name is ignored.

import (
	"context"
	"strings"
	"time"
)

// Observer decides for commits of one repository.
type Observer struct {
	Cfg     Config
	Funnel  Funnel
	Breaker Breaker
	Now     func() time.Time
}

// billingMarkers are lower-case fragments of the annotation GitHub writes when an account's
// billing blocks Actions.
var billingMarkers = []string{"payments have failed", "spending limit", "billing"}

// Decide returns the decision for sha, pushed at pushed.
func (o *Observer) Decide(ctx context.Context, sha string, pushed time.Time) Result {
	cfg := o.Cfg.Normalize()
	mode, notes := cfg.effective()
	r := Result{Mode: mode, Notes: notes}
	if mode == ModeOff {
		return r.hand(ReasonOff)
	}
	if o.Breaker != nil && o.Breaker.Blocked(ctx) {
		return r.hand(ReasonUnavailable)
	}
	runs, err := o.Funnel.CheckRuns(ctx, sha)
	r.Requests++
	if err != nil {
		return r.hand(ReasonAPIError)
	}
	now := o.Now()
	run, ok := pick(runs, cfg)
	if !ok {
		if now.Sub(pushed) < cfg.Absent {
			return r.with(Wait, ReasonAbsent)
		}
		return r.hand(ReasonAbsentLong)
	}
	switch run.Status {
	case "completed":
		return o.completed(ctx, r, run, cfg, now)
	case "in_progress":
		if now.Sub(first(run.StartedAt, run.CreatedAt)) < cfg.Running {
			return r.with(Wait, ReasonRunning)
		}
		return r.hand(ReasonRunningLong)
	default: // queued, waiting, pending, requested
		if now.Sub(run.CreatedAt) < cfg.Queued {
			return r.with(Wait, ReasonQueued)
		}
		return r.hand(ReasonQueuedLong)
	}
}

func (o *Observer) completed(ctx context.Context, r Result, run CheckRun, cfg Config, now time.Time) Result {
	switch run.Conclusion {
	case "success", "neutral", "skipped":
		r.Residual = append([]string(nil), cfg.Residual...)
		return r.with(Use, ReasonSuccess)
	case "cancelled":
		return r.hand(ReasonCancelled)
	}
	notes, err := o.Funnel.Annotations(ctx, run.ID)
	r.Requests++
	if err == nil && billing(notes) {
		if o.Breaker != nil {
			o.Breaker.MarkBlocked(now.Add(DefaultRunning))
		}
		return r.hand(ReasonBilling)
	}
	r.Residual = append([]string(nil), cfg.Residual...)
	return r.with(Use, ReasonFailure) // a red hosted result stays red
}

func (r Result) with(d Decision, reason string) Result { r.Decision, r.Reason = d, reason; return r }
func (r Result) hand(reason string) Result             { return r.with(HandOver, reason) }

// pick returns the newest check run with the configured name and app.
func pick(runs []CheckRun, cfg Config) (CheckRun, bool) {
	var best CheckRun
	found := false
	for _, c := range runs {
		if c.Name != cfg.Context || c.AppID != cfg.AppID {
			continue
		}
		if !found || c.CreatedAt.After(best.CreatedAt) || c.CreatedAt.Equal(best.CreatedAt) && c.ID > best.ID {
			best, found = c, true
		}
	}
	return best, found
}

func billing(notes []string) bool {
	for _, n := range notes {
		low := strings.ToLower(n)
		for _, m := range billingMarkers {
			if strings.Contains(low, m) {
				return true
			}
		}
	}
	return false
}

func first(a, b time.Time) time.Time {
	if a.IsZero() {
		return b
	}
	return a
}
