package sched

// Purpose: the `auto` preset's tier wait (P7-AUTO-05). Under `auto` a job whose
// better-tier runners are only busy or not yet ready waits up to
// sched.auto.step_sec for them before it runs on the best eligible runner.
// Inputs: the World (Policy.Preset, Policy.Trust for step_sec, Bias for the
// tier of each runner, written by internal/tiers) and Job.QueuedMs.
// Outputs: TierFor, TierOf, AutoStepMs and the tier.waiting hold Place applies.
// Constraints: pure and deterministic: QueuedMs comes from the Job, never a
// clock; only capacity.busy and node.not_ready are waited for (an explicit set,
// not PlacementReason.Static: offline, draining and provider.down are not
// static but are never waited for); under auto the local fast path is off,
// because the tier order (local last) decides; the hold is bounded by one step_sec,
// whatever QueuedMs says, and ends AutoPollMs before the job's max_queue, so a hold
// never runs a job into E693 while a lower tier is free; a step_sec of 0 never
// waits; a tier that sched.auto.order leaves out is excluded (override.excluded) and
// never waited for; every other preset is untouched.

import (
	"fmt"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// AutoPreset is the preset that places by tier order (P7-AUTO-05).
const AutoPreset = "auto"

// The tiers a runner can be in, by location class.
const (
	TierHosted = "hosted"
	TierVPS    = "vps"
	TierLAN    = "lan"
	TierLocal  = "local"
)

// TierStepMs is the Bias between adjacent tiers. A rank stays below it, so the
// tier index of a runner is its Bias divided by TierStepMs.
const TierStepMs int64 = 60000

// KeyAutoStepSec is the policy key of the tier wait, in seconds (0: never wait).
const KeyAutoStepSec = "sched.auto.step_sec"

// KeyAutoOrder is the policy key of the tier order (registered by internal/tiers).
const KeyAutoOrder = "sched.auto.order"

// AutoPollMs is the slowest dispatcher re-tick (the TEAM tick, one second). A hold
// ends this long before max_queue so the tick that ends it still places the job.
const AutoPollMs int64 = 1000

// DefaultAutoStepSec is the registered default of KeyAutoStepSec.
const DefaultAutoStepSec int64 = 30

// tierWaitReasons are the only reasons a better tier is waited for.
var tierWaitReasons = map[string]bool{string(model.CapacityBusy): true, string(model.NodeNotReady): true}

// TierFor returns the tier of a location and cost class: hosted (hosted
// location or included class), lan, local, or vps (any other remote runner).
func TierFor(location, class string) string {
	switch {
	case location == "hosted" || class == "included":
		return TierHosted
	case location == "lan":
		return TierLAN
	case location == "local":
		return TierLocal
	}
	return TierVPS
}

// TierOf returns the tier of r in w.
func TierOf(w World, r Runner) string { return TierFor(location(r), runnerClass(w, r)) }

// AutoStepMs returns sched.auto.step_sec of the world's policy in ms; the
// default when the key is absent, 0 when it is 0 or not a whole number >= 0.
func AutoStepMs(w World) int64 {
	return stepMs(w.Policy.Trust)
}

func stepMs(eff trust.Effective) int64 {
	sec := DefaultAutoStepSec
	if v, _, ok := eff.Value(KeyAutoStepSec); ok {
		n, isInt := trust.IntValue(v)
		if !isInt || n < 0 {
			return 0
		}
		sec = n
	}
	ms, _ := Mul(sec, 1000)
	return ms
}

// tierListed reports whether r's tier is in the effective sched.auto.order. It is
// true outside `auto`, when the key is unset, and when the key names no known tier
// (internal/tiers refuses to build such a world, so this only guards a direct caller).
func tierListed(w World, r Runner) bool {
	if w.Policy.Preset != AutoPreset {
		return true
	}
	v, _, ok := w.Policy.Trust.Value(KeyAutoOrder)
	if !ok {
		return true
	}
	var listed []string
	switch xs := v.(type) {
	case []string:
		listed = xs
	case []any:
		for _, x := range xs {
			if s, ok := x.(string); ok {
				listed = append(listed, s)
			}
		}
	}
	known := false
	tier := TierOf(w, r)
	for _, s := range listed {
		switch s {
		case TierHosted, TierVPS, TierLAN, TierLocal:
			known = true
		}
		if s == tier {
			return true
		}
	}
	return !known
}

// placementFastPath is fastPath for every preset but auto, which never takes it.
func placementFastPath(w World, jobs []Job, runners []Runner) model.FastPath {
	if w.Policy.Preset == AutoPreset {
		return model.FastPath{Reason: "none"}
	}
	return fastPath(w, jobs, runners)
}

func tierIndex(w World, r Runner) int64 {
	b := w.Bias[r.ID]
	if b <= 0 {
		return 0
	}
	return b / TierStepMs
}

// waitOnly reports whether every reason is one the tier wait waits for.
func waitOnly(rs []model.PlacementReasonDetail) bool {
	if len(rs) == 0 {
		return false
	}
	for _, q := range rs {
		if !tierWaitReasons[q.Code] {
			return false
		}
	}
	return true
}

// tierHold returns the tier.waiting reason when j must wait for a better-tier
// runner than runners[best]. alts[i] is the alternative of runners[i].
func tierHold(w World, j Job, runners []Runner, alts []model.Alternative, best int) (model.PlacementReasonDetail, bool) {
	if w.Policy.Preset != AutoPreset || best < 0 || len(alts) != len(runners) {
		return model.PlacementReasonDetail{}, false
	}
	step := AutoStepMs(w)
	queued := j.QueuedMs
	if queued < 0 { // a skewed clock never adds wait
		queued = 0
	}
	limit, capped := step, false
	if mq := w.Policy.Overrides.MaxQueueMs; mq > 0 && mq-AutoPollMs < limit {
		limit, capped = mq-AutoPollMs, true
	}
	if step <= 0 || limit <= 0 || queued >= limit {
		return model.PlacementReasonDetail{}, false
	}
	bestTier := tierIndex(w, runners[best])
	held := -1
	for i, a := range alts {
		if a.Eligible || !waitOnly(a.Reasons) || tierIndex(w, runners[i]) >= bestTier {
			continue
		}
		if held < 0 || w.Bias[runners[i].ID] < w.Bias[runners[held].ID] {
			held = i
		}
	}
	if held < 0 {
		return model.PlacementReasonDetail{}, false
	}
	r := runners[held]
	codes := make([]string, len(alts[held].Reasons))
	for i, q := range alts[held].Reasons {
		codes[i] = q.Code
	}
	bound := fmt.Sprintf("step_sec %d s", step/1000)
	if capped {
		bound = fmt.Sprintf("max_queue %d ms less one poll", w.Policy.Overrides.MaxQueueMs)
	}
	detail := fmt.Sprintf("held for %s (tier %s, bias %d, %s): %d ms left of %s, then %s (tier %s)",
		r.ID, TierOf(w, r), w.Bias[r.ID], strings.Join(codes, ","), limit-queued, bound, runners[best].ID, TierOf(w, runners[best]))
	return reason(model.TierWaiting, detail), true
}
