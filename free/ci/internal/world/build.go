// Package world assembles one immutable placement snapshot from durable and injected facts.
package world

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/registry"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// Deps carries previously observed local capacity; Build never runs a probe.
type Deps struct {
	Store          *store.Store
	Registry       *registry.Registry
	Local          *model.Capability
	Jobs           []sched.Job
	LocalCapacity  *ciexec.Capacity
	Policy         func(context.Context, string) (sched.Policy, error)
	Clock          func() time.Time
	LocalAddresses func(context.Context) ([]string, error)
	ResolveHost    func(context.Context, string) ([]string, error)
}

// Build returns a complete world and its digest, or no world on any read or hook error.
func Build(ctx context.Context, d Deps, pipeline sched.Pipeline) (sched.World, string, error) {
	if d.Store == nil || d.Policy == nil {
		return sched.World{}, "", failed("store and policy required", nil)
	}
	snapshot, err := d.Store.WorldSnapshot(ctx)
	if err != nil {
		return sched.World{}, "", failed("store snapshot", err)
	}
	policy, err := d.Policy(ctx, pipeline.ID)
	if err != nil {
		return sched.World{}, "", failed("policy", err)
	}
	now := time.Now
	if d.Clock != nil {
		now = d.Clock
	}
	w := sched.World{NowMs: now().UnixMilli(), Policy: policy, Previous: snapshot.Previous, Counts: sched.Counts{Runners: map[string]int{}, Providers: map[string]int{}}, InteractiveQueued: map[string]bool{}}
	if len(d.Jobs) > 0 {
		w.Previous = map[string]string{}
		for _, j := range d.Jobs {
			if runner := snapshot.PreviousByJob[j.ID]; runner != "" {
				w.Previous[j.Key] = runner
			}
		}
	}
	labels, factsFn := currentHooks()
	facts := CoordinatorFacts{}
	if factsFn != nil {
		facts, err = factsFn(ctx)
		if err != nil {
			facts = CoordinatorFacts{Serving: true, Mode: "team"}
		}
	}
	var mode trust.CoordinatorMode
	if facts.Serving {
		mode = trust.DecodeCoordinatorMode(facts.Mode)
	}
	localMachine := ""
	if d.Local != nil && d.Local.Identity.MachineID.Value != nil {
		localMachine = *d.Local.Identity.MachineID.Value
	}
	addresses := map[string]bool{}
	if facts.Serving {
		addresses, err = localAddresses(ctx, d)
		if err != nil {
			return sched.World{}, "", failed("local addresses", err)
		}
	}
	for _, node := range snapshot.Nodes {
		var cap model.Capability
		if err = json.Unmarshal(node.Capability, &cap); err != nil {
			return sched.World{}, "", failed("node "+node.ID, err)
		}
		cap.Availability.State.Value = &node.State
		cap.Availability.State.Confidence = "known"
		cap.Availability.State.Source = "assigned"
		host := false
		if facts.Serving {
			host, err = coordinatorNode(ctx, d, cap, localMachine, addresses, facts.AgentPeers)
			if err != nil {
				return sched.World{}, "", failed("coordinator endpoint "+node.ID, err)
			}
		}
		runner := makeRunner(cap, host, mode, snapshot)
		if labels != nil {
			extra, e := labels(ctx, runner.ID)
			if e != nil {
				return sched.World{}, "", failed("labels "+runner.ID, e)
			}
			runner.Capability.Identity.Labels = mergeLabels(runner.Capability.Identity.Labels, extra)
		}
		w.Runners = append(w.Runners, runner)
	}
	if d.Local != nil {
		cap := *d.Local
		cap.Identity.ID = "local"
		if d.LocalCapacity != nil {
			applyLocalCapacity(&cap, *d.LocalCapacity)
		}
		local := makeRunner(cap, facts.Serving, mode, snapshot)
		if labels != nil {
			extra, e := labels(ctx, "local")
			if e != nil {
				return sched.World{}, "", failed("local labels", e)
			}
			local.Capability.Identity.Labels = mergeLabels(local.Capability.Identity.Labels, extra)
		}
		w.Runners = append(w.Runners, local)
	}
	for i := range w.Runners {
		w.Runners[i].QueuedAhead = int64(snapshot.Queued[snapshot.Projects[pipeline.ID]])
	}
	sort.Slice(w.Runners, func(i, j int) bool { return w.Runners[i].ID < w.Runners[j].ID })
	for _, a := range snapshot.Active {
		w.Counts.Global++
		if a.PipelineID == pipeline.ID {
			w.Counts.Pipeline++
		}
		if a.Project == snapshot.Projects[pipeline.ID] {
			w.Counts.Project++
		}
		if a.RunnerID != "" {
			w.Counts.Runners[a.RunnerID]++
			w.Counts.Runner++
		}
		provider := a.Provider
		if provider == "" {
			for _, r := range w.Runners {
				if r.ID == a.RunnerID {
					provider = r.Provider
					break
				}
			}
		}
		if provider != "" {
			w.Counts.Providers[provider]++
			w.Counts.Provider++
		}
	}
	for _, j := range d.Jobs {
		count := 0
		for _, a := range snapshot.Active {
			if a.JobID == j.ID {
				count++
			}
		}
		if count > w.Counts.Job {
			w.Counts.Job = count
		}
	}
	if len(d.Jobs) == 0 {
		w.Counts.Job = w.Counts.Global
	}
	// No identity is stored on attempts yet. Global active work is a proven upper bound
	// for any user's or team's work, so their limits can only fail closed.
	w.Counts.User, w.Counts.Team = w.Counts.Global, w.Counts.Global
	if snapshot.Queued[snapshot.Projects[pipeline.ID]] > 0 {
		// Queued attempts carry no priority band yet. Treat them as interactive
		// for every runner, a conservative bound for background admission.
		for _, r := range w.Runners {
			w.InteractiveQueued[r.ID] = true
		}
	}
	if err = runContributors(ctx, &w); err != nil {
		return sched.World{}, "", err
	}
	digest, err := Digest(w)
	if err != nil {
		return sched.World{}, "", err
	}
	return w, digest, nil
}

func makeRunner(c model.Capability, host bool, mode trust.CoordinatorMode, s store.SchedWorld) sched.Runner {
	if !host {
		mode = ""
	}
	r := sched.Runner{ID: c.Identity.ID, Capability: c, Decl: sched.DeclarationFromCapability(c, host, mode), Provider: c.Identity.Provider, CoordinatorHost: host, CoordinatorMode: mode}
	if c.Location.Kind.Value != nil {
		r.Location = *c.Location.Kind.Value
	}
	if c.Economics.Class.Value != nil {
		r.Class = *c.Economics.Class.Value
	}
	if c.Availability.State.Value != nil {
		r.Active = *c.Availability.State.Value == "online"
	}
	reserved := s.Reservations[r.ID]
	active := 0
	for _, a := range s.Active {
		if a.RunnerID == r.ID {
			active++
		}
	}
	if c.Availability.Slots.Value != nil {
		r.Avail.Slots = max(0, *c.Availability.Slots.Value-active-int(reserved.Slots))
	}
	if c.Resources.CPU.Value != nil {
		r.Avail.CPU = max(0, int64(*c.Resources.CPU.Value)-reserved.CPU)
	}
	if c.Resources.MemMB.Value != nil {
		r.Avail.MemMB = max(0, *c.Resources.MemMB.Value-reserved.MemMB)
	}
	return r
}
func applyLocalCapacity(c *model.Capability, p ciexec.Capacity) {
	if p.CPUs > 0 {
		n := float64(p.CPUs)
		c.Resources.CPU.Value = &n
		if c.Availability.Slots.Value == nil {
			c.Availability.Slots.Value = &p.CPUs
		}
	}
	if p.MemoryMB > 0 {
		n := int64(p.MemoryMB)
		c.Resources.MemMB.Value = &n
	}
}
func mergeLabels(a, b []string) []string {
	set := map[string]bool{}
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		set[v] = true
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
