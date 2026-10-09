package sched

import (
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// TestComparisonBoundaries covers independent eligibility terms at their boundaries.
func TestComparisonBoundaries(t *testing.T) {
	cases := []struct {
		name string
		code model.PlacementReason
		set  func(*World, *Job, *Runner)
	}{
		{"limit-at-cap", model.LimitPipeline, func(w *World, _ *Job, _ *Runner) { w.Policy.Limits.Pipeline, w.Counts.Pipeline = 1, 1 }},
		{"revoked-state", model.NodeRevoked, func(_ *World, _ *Job, r *Runner) { r.Capability.Availability.State.Value = ptr("revoked") }},
		{"interactive-background", model.NodeInteractive, func(_ *World, j *Job, r *Runner) {
			j.Pipeline.Band = "background"
			r.Capability.Availability.Interactive.Value = ptr("active")
		}},
		{"reserved-cpu", model.NodeReservation, func(_ *World, _ *Job, r *Runner) {
			r.Capability.Resources.Reserved.CPU.Value = ptr(float64(1))
			r.Avail.CPU = 0
		}},
		{"cpu-short", model.ResourcesInsufficient, func(_ *World, j *Job, r *Runner) {
			j.Requirements.CPU = 9
			r.Capability.Resources.CPU.Value = ptr(float64(8))
		}},
		{"memory-short", model.ResourcesInsufficient, func(_ *World, j *Job, r *Runner) {
			j.Requirements.MemMB = 8193
			r.Capability.Resources.MemMB.Value = ptr(int64(8192))
		}},
		{"local-only", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.LocalOnly = true }},
		{"lan-only", model.OverrideExcluded, func(w *World, _ *Job, r *Runner) { w.Policy.Overrides.LANOnly = true; r.Location = "remote" }},
		{"private-only", model.OverrideExcluded, func(w *World, _ *Job, r *Runner) { w.Policy.Overrides.PrivateOnly = true; r.Location = "hosted" }},
		{"runner-override", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.Runner = "other" }},
		{"os-override", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.OS = "darwin" }},
		{"os-unknown", model.OverrideExcluded, func(w *World, _ *Job, r *Runner) {
			w.Policy.Overrides.OS = "darwin"
			r.Capability.Platform.OS.Value = nil
		}},
		{"arch-override", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.Arch = "arm64" }},
		{"arch-unknown", model.OverrideExcluded, func(w *World, _ *Job, r *Runner) {
			w.Policy.Overrides.Arch = "arm64"
			r.Capability.Platform.Arch.Value = nil
		}},
		{"provider-allow", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.ProvidersAllow = []string{"other"} }},
		{"provider-deny", model.OverrideExcluded, func(w *World, _ *Job, _ *Runner) { w.Policy.Overrides.ProvidersDeny = []string{"operator"} }},
		{"isolation-floor", model.OverrideMinIsolation, func(w *World, _ *Job, r *Runner) {
			w.Policy.Overrides.MinIsolation = "vm"
			r.Decl.Isolation = "process"
		}},
		{"queue-cap", model.OverrideMaxQueue, func(w *World, _ *Job, r *Runner) { w.Policy.Overrides.MaxQueueMs = 1; r.QueuedAhead = 2 }},
		{"interactive-queued", model.BackgroundInteractiveQueued, func(w *World, j *Job, _ *Runner) {
			j.Pipeline.Band = "background"
			w.InteractiveQueued = map[string]bool{"r": true}
		}},
		{"background-contention", model.BackgroundContention, func(_ *World, j *Job, r *Runner) { j.Pipeline.Band = "background"; r.Active = true }},
		{"remote-only", model.RunnerRemoteOnly, func(_ *World, j *Job, r *Runner) { j.RemoteOnly = true; r.Location = "local" }},
		{"cost-class-fallback", model.PaidDisabled, func(w *World, _ *Job, r *Runner) { r.Class = ""; w.Costs[r.ID] = Cost{Class: "metered", RateMicro: 1} }},
		{"provider-owner", model.ProviderOwnedElsewhere, func(w *World, _ *Job, r *Runner) {
			r.Class = "metered"
			w.Costs[r.ID] = Cost{RateMicro: 1, OwnerCoordinator: "other"}
			w.Policy.CoordinatorID = "self"
		}},
		{"team-only", model.BurstTeamOnly, func(w *World, _ *Job, r *Runner) {
			r.Class = "ephemeral"
			r.CoordinatorMode = trust.Personal
			w.Costs[r.ID] = Cost{RateMicro: 1}
		}},
		{"approval-threshold", model.ApprovalPending, func(w *World, _ *Job, r *Runner) {
			r.Class = "metered"
			w.Costs[r.ID] = Cost{RateMicro: 1}
			w.Policy.ApprovalThresholdMicro = 1
		}},
		{"maximum-cost", model.OverrideMaxCost, func(w *World, _ *Job, r *Runner) {
			r.Class = "metered"
			w.Costs[r.ID] = Cost{RateMicro: 1}
			w.Policy.Overrides.MaxCostMicro = 1
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := fixtureRunner("r", "lan")
			j := fixtureJob("j")
			w := fixtureWorld(r)
			tc.set(&w, &j, &r)
			if !containsCode(eligibility(w, j, r, false), string(tc.code)) {
				t.Fatalf("missing %s for %s", tc.code, tc.name)
			}
		})
	}
}

func TestHelperComparisonBoundaries(t *testing.T) {
	if !labelMatches([]string{"xcode"}, "xcode") {
		t.Fatal("exact label")
	}
	for _, name := range []string{"docker", "podman", "gvisor", "tart"} {
		c := model.Capability{}
		switch name {
		case "docker":
			c.Tools.Docker.Value = ptr(true)
		case "podman":
			c.Tools.Podman.Value = ptr(true)
		case "gvisor":
			c.Tools.GVisor.Value = ptr(true)
		case "tart":
			c.Tools.Tart.Value = ptr(true)
		}
		if !toolPresent(c, name) {
			t.Fatalf("tool %s", name)
		}
	}
	if !hasInt([]int{1}, 1) {
		t.Fatal("protocol version")
	}
}
