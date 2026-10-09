package sched

import (
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"strings"
)

func reason(code model.PlacementReason, detail string) model.PlacementReasonDetail {
	return model.PlacementReasonDetail{Code: string(code), Detail: detail}
}
func has(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func labelMatches(labels []string, requirement string) bool {
	if strings.Contains(requirement, "=") {
		return has(labels, requirement)
	}
	for _, v := range labels {
		if v == requirement || strings.HasPrefix(v, requirement+"=") {
			return true
		}
	}
	return false
}
func toolPresent(c model.Capability, name string) bool {
	switch name {
	case "docker":
		return c.Tools.Docker.Value != nil && *c.Tools.Docker.Value
	case "podman":
		return c.Tools.Podman.Value != nil && *c.Tools.Podman.Value
	case "gvisor":
		return c.Tools.GVisor.Value != nil && *c.Tools.GVisor.Value
	case "tart":
		return c.Tools.Tart.Value != nil && *c.Tools.Tart.Value
	}
	if f, ok := c.Tools.Toolchains[name]; ok && f.Value != nil && *f.Value != "" {
		return true
	}
	return c.Tools.Browsers.Value != nil && has(*c.Tools.Browsers.Value, name)
}
func limits(w World, j Job, r Runner) []model.PlacementReasonDetail {
	pairs := []struct {
		max, used int
		code      model.PlacementReason
	}{
		{w.Policy.Limits.Pipeline, w.Counts.Pipeline, model.LimitPipeline}, {w.Policy.Limits.Job, w.Counts.Job, model.LimitJob},
		{w.Policy.Limits.Runner, w.Counts.Runners[r.ID], model.LimitRunner}, {w.Policy.Limits.Provider, w.Counts.Providers[r.Provider], model.LimitProvider},
		{w.Policy.Limits.Project, w.Counts.Project, model.LimitProject}, {w.Policy.Limits.User, w.Counts.User, model.LimitUser},
		{w.Policy.Limits.Team, w.Counts.Team, model.LimitTeam}, {w.Policy.Limits.Global, w.Counts.Global, model.LimitGlobal},
	}
	var out []model.PlacementReasonDetail
	for _, p := range pairs {
		if p.max > 0 && p.used >= p.max {
			out = append(out, reason(p.code, "in-call limit reached"))
		}
	}
	_ = j
	return out
}

// eligibility evaluates every rule even when an earlier rule has already failed.
func eligibility(w World, j Job, r Runner, antiAffinity bool) []model.PlacementReasonDetail {
	out := make([]model.PlacementReasonDetail, 0)
	add := func(code model.PlacementReason, detail string) { out = append(out, reason(code, detail)) }
	c := r.Capability
	if c.Lifecycle.RevokedAt != nil || c.Availability.State.Value != nil && *c.Availability.State.Value == "revoked" {
		add(model.NodeRevoked, "revoked")
	}
	// Hosted authorization comes from the job's effective trust grants; enrolled
	// nodes require explicit project membership in their capability snapshot.
	hosted := r.Location == "hosted" || c.Location.Kind.Value != nil && *c.Location.Kind.Value == "hosted" || r.Decl.Hosted
	if c.Schema != "ci.runner-capability/v1" || c.Lifecycle.Discovered || j.Project == "" || !hosted && !has(c.Lifecycle.AuthorizedProjects, j.Project) {
		add(model.NodeDiscovered, "not authorized")
	}
	if c.DeployHost.Matched && !c.DeployHost.Override {
		add(model.NodeDeployHost, "deploy host")
	}
	if c.Protocol.Min > 1 || !hasInt(c.Protocol.Versions, 1) {
		add(model.NodeProtocol, "unsupported protocol")
	}
	if s := c.Availability.State.Value; s != nil {
		switch *s {
		case "offline":
			add(model.NodeOffline, "offline")
		case "draining":
			add(model.NodeDraining, "draining")
		case "maintenance":
			add(model.NodeMaintenance, "maintenance")
		}
	}
	operatorOwned := r.Decl.Ownership == "operator" || c.Identity.Ownership.Value != nil && *c.Identity.Ownership.Value == "operator"
	if c.Availability.PluggedIn.Value != nil && !*c.Availability.PluggedIn.Value || j.Pipeline.Band == "background" && operatorOwned && c.Availability.PluggedIn.Value == nil {
		add(model.NodeOnBattery, "unplugged")
	}
	if j.Pipeline.Band == "background" && (c.Availability.Interactive.Value != nil && *c.Availability.Interactive.Value == "active" || operatorOwned && (c.Availability.Interactive.Value == nil || *c.Availability.Interactive.Value != "idle")) {
		add(model.NodeInteractive, "active use")
	}
	if c.Resources.Reserved.CPU.Value != nil && *c.Resources.Reserved.CPU.Value > 0 && r.Avail.CPU <= 0 {
		add(model.NodeReservation, "reserved")
	}
	d := r.Decl
	if len(d.Accepts) == 0 {
		d = DeclarationFromCapability(c, r.CoordinatorHost, r.CoordinatorMode)
	}
	d.CoordinatorHost = r.CoordinatorHost
	d.CoordinatorMode = r.CoordinatorMode
	d.Hosted = hosted
	jf := trust.JobFacts{Trust: j.Trust, SecretClasses: j.SecretClasses, Network: j.Network, PersonalOwnRun: j.PersonalOwnRun, Release: j.Pipeline.Release}
	for _, tr := range trust.Admissible(jf, d, w.Policy.Trust) {
		out = append(out, model.PlacementReasonDetail{Code: string(tr), Detail: string(tr)})
	}
	if j.Requirements.IsolationMin != "" && trust.RankIsolation(d.Isolation) < trust.RankIsolation(trust.DecodeIsolation(string(j.Requirements.IsolationMin))) {
		out = append(out, model.PlacementReasonDetail{Code: string(trust.IsolationBelowMinimum), Detail: "job isolation minimum"})
	}
	if previous := w.PreviousIsolation[j.Key]; previous != "" && trust.RankIsolation(d.Isolation) < trust.RankIsolation(trust.RetryIsolation(previous, d.Isolation)) {
		out = append(out, model.PlacementReasonDetail{Code: string(trust.IsolationBelowMinimum), Detail: "retry isolation floor"})
	}
	os, arch := c.Platform.OS.Value, c.Platform.Arch.Value
	if os == nil || arch == nil || j.Requirements.Platform != "" && *os+"/"+*arch != j.Requirements.Platform {
		add(model.PlatformMismatch, j.Requirements.Platform)
	}
	for _, t := range j.Requirements.Tools {
		if !toolPresent(c, t) {
			add(model.ToolsMissing, t)
		}
	}
	if j.Requirements.CPU > 0 && (c.Resources.CPU.Value == nil || int64(*c.Resources.CPU.Value) < j.Requirements.CPU) || j.Requirements.MemMB > 0 && (c.Resources.MemMB.Value == nil || *c.Resources.MemMB.Value < j.Requirements.MemMB) {
		add(model.ResourcesInsufficient, "cpu or memory")
	}
	for _, a := range j.Accelerators {
		if c.Resources.Accelerators.Value == nil || !has(*c.Resources.Accelerators.Value, a) {
			add(model.ResourcesInsufficient, a)
		}
	}
	for _, a := range j.Requirements.Labels {
		if !labelMatches(c.Identity.Labels, a) {
			add(model.LabelsMismatch, a)
		}
	}
	for _, a := range w.Policy.Overrides.Labels {
		if !labelMatches(c.Identity.Labels, a) {
			add(model.LabelsMismatch, a)
		}
	}
	if antiAffinity && w.Previous[j.Key] == r.ID {
		add(model.RetryAntiAffinity, "previous attempt runner")
	}
	o := w.Policy.Overrides
	if o.LocalOnly && r.Location != "local" || o.LANOnly && r.Location != "local" && r.Location != "lan" || o.PrivateOnly && r.Location == "hosted" || o.Runner != "" && o.Runner != r.ID || o.OS != "" && (os == nil || *os != o.OS) || o.Arch != "" && (arch == nil || *arch != o.Arch) || len(o.ProvidersAllow) > 0 && !has(o.ProvidersAllow, r.Provider) || has(o.ProvidersDeny, r.Provider) {
		add(model.OverrideExcluded, "override")
	}
	if o.MinIsolation != "" && trust.RankIsolation(d.Isolation) < trust.RankIsolation(o.MinIsolation) {
		add(model.OverrideMinIsolation, "minimum isolation")
	}
	if o.MaxQueueMs > 0 && r.QueuedAhead > o.MaxQueueMs {
		add(model.OverrideMaxQueue, "queue")
	}
	if r.Avail.Slots <= 0 || r.Avail.CPU < j.Requirements.CPU || r.Avail.MemMB < j.Requirements.MemMB {
		add(model.CapacityBusy, "availability")
	}
	if j.Pipeline.Band == "background" && w.InteractiveQueued[r.ID] {
		add(model.BackgroundInteractiveQueued, "interactive queued")
	}
	if j.Pipeline.Band == "background" && r.Active {
		add(model.BackgroundContention, "active use")
	}
	out = append(out, limits(w, j, r)...)
	out = append(out, economics(w, j, r)...)
	if r.ProviderDown {
		add(model.ProviderDown, "provider unavailable")
	}
	if j.RemoteOnly && r.Location == "local" {
		add(model.RunnerRemoteOnly, "remote required")
	}
	if r.Ephemeral && r.Used {
		add(model.EphemeralUsed, "one job per runner")
	}
	return out
}
func hasInt(values []int, want int) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func economics(w World, j Job, r Runner) []model.PlacementReasonDetail {
	var out []model.PlacementReasonDetail
	add := func(code model.PlacementReason, detail string) { out = append(out, reason(code, detail)) }
	c := w.Costs[r.ID]
	class := r.Class
	if class == "" {
		class = c.Class
	}
	paid := class != "owned" && class != "local" && class != "included"
	cost, overflow := worstCost(j.TimeoutMs, c)
	if class == "owned" || class == "local" {
		cost, overflow = 0, false
	}
	if paid {
		if c.RateMicro <= 0 {
			add(model.CostUnknownRate, "missing or nonpositive rate")
		}
		max := w.Policy.MaxPaidTimeoutMs
		if max <= 0 {
			max = 7200000
		}
		if j.TimeoutMs <= 0 || j.TimeoutMs > max {
			add(model.CostUnbounded, "timeout")
		}
		if !w.Policy.PaidEnabled {
			add(model.PaidDisabled, "operator policy")
		}
		p := w.Budgets.Providers[r.Provider]
		if !w.Budgets.Day.Set && !w.Budgets.Month.Set && !p.Set {
			add(model.BudgetUnset, "day, month or provider budget required")
		}
		if c.OwnerCoordinator != "" && c.OwnerCoordinator != w.Policy.CoordinatorID {
			add(model.ProviderOwnedElsewhere, "provider owner")
		}
		if class == "ephemeral" && r.CoordinatorMode != trust.Team {
			add(model.BurstTeamOnly, "team coordinator required")
		}
		for _, b := range []Budget{w.Budgets.Job, w.Budgets.Pipeline, w.Budgets.Day, w.Budgets.Month, p} {
			if b.Set && (overflow || cost > b.Remaining) {
				add(model.BudgetExceeded, "budget")
			}
		}
		if overflow {
			add(model.BudgetExceeded, "saturated cost")
		}
		if w.Policy.ApprovalThresholdMicro > 0 && cost > w.Policy.ApprovalThresholdMicro && !w.Approvals[j.Pipeline.ID] {
			add(model.ApprovalPending, "spend approval")
		}
	}
	if class == "included" && !labelMatches(r.Capability.Identity.Labels, "nself.visibility=public") {
		a, ok := w.Allowances[r.Provider]
		if !ok || !a.Known || !a.CeilingKnown {
			add(model.AllowanceUnknown, "consumption or ceiling unknown")
		}
		remaining, marginOverflow := int64(0), a.MarginMicro < 0
		if !marginOverflow {
			remaining, marginOverflow = Add(a.RemainingEstimated, -a.MarginMicro)
		}
		if ok && a.Known && a.CeilingKnown && (overflow || marginOverflow || remaining <= cost) {
			add(model.AllowanceBelowMargin, "allowance margin")
		}
	}
	if w.Policy.Overrides.MaxCostMicro > 0 && cost > w.Policy.Overrides.MaxCostMicro {
		add(model.OverrideMaxCost, "maximum cost")
	}
	return out
}

func worstCost(timeout int64, c Cost) (int64, bool) {
	if c.RateMicro <= 0 {
		return int64(^uint64(0) >> 1), true
	}
	if timeout < 0 {
		timeout = 0
	}
	inc := c.BillingIncrementMs
	if inc <= 0 {
		inc = 60000
	}
	units := timeout / inc
	if timeout%inc != 0 {
		units++
	}
	billed, of := Mul(units, inc)
	charge, of2 := Mul(billed, c.RateMicro)
	charge, of3 := Add(charge, c.MinimumChargeMicro)
	charge, of4 := Add(charge, c.ExtrasMicro)
	return charge, of || of2 || of3 || of4
}
