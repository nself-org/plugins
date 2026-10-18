package planview

// Purpose: the capability of the machine `ci plan` and `ci explain` run on,
// so a plan can place work locally when no node is registered (PERSONAL mode)
// and still list registered nodes as alternatives.
// Inputs: LocalFacts probed by the caller (platform, CPU, memory, tools).
// Outputs: a model.Capability for the runner "local".
// Constraints: pure; every value is a fact the caller observed (source
// "probe"); the local owner's own checkout is the only work it is declared to
// accept (trust class owner, process isolation, no uid separation).

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// LocalFacts are the observations the local capability is built from.
type LocalFacts struct {
	Project  string
	Platform string // os/arch
	CPUs     int
	MemMB    int64
	Docker   bool
	Tools    []string
	Now      time.Time
}

func fact[T any](v T, now time.Time) model.Fact[T] {
	return model.Fact[T]{Value: &v, Source: "probe", ObservedAt: now, Confidence: "known"}
}

// assigned is a fact the coordinator decides rather than the machine reports.
func assigned[T any](v T, now time.Time) model.Fact[T] {
	f := fact(v, now)
	f.Source = "assigned"
	return f
}

// LocalCapability describes this machine as the runner "local". Facts the
// caller did not observe are explicit unknowns, so the document is a valid
// ci.runner-capability/v1.
func LocalCapability(f LocalFacts) (model.Capability, error) {
	c := localDraft(f)
	raw, err := json.Marshal(c)
	if err != nil {
		return model.Capability{}, err
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return model.Capability{}, err
	}
	unknownFacts(doc, f.Now.UTC())
	if raw, err = json.Marshal(doc); err != nil {
		return model.Capability{}, err
	}
	var out model.Capability
	if err = json.Unmarshal(raw, &out); err != nil { // validates against the schema
		return model.Capability{}, err
	}
	return out, nil
}

// unknownFacts turns every empty fact object into an observed unknown.
func unknownFacts(v any, now time.Time) {
	switch x := v.(type) {
	case map[string]any:
		if src, ok := x["source"].(string); ok && src == "" {
			x["source"], x["observed_at"], x["confidence"], x["value"] = "probe", now.Format(time.RFC3339Nano), "unknown", nil
			return
		}
		for _, e := range x {
			unknownFacts(e, now)
		}
	case []any:
		for _, e := range x {
			unknownFacts(e, now)
		}
	}
}

func localDraft(f LocalFacts) model.Capability {
	os, arch, _ := strings.Cut(f.Platform, "/")
	now := f.Now.UTC()
	c := model.Capability{Schema: "ci.runner-capability/v1"}
	c.Identity = model.CapabilityIdentity{ID: "local", Name: "this machine", Provider: "local", Transport: "local", Labels: []string{},
		Ownership: assigned("operator", now)}
	c.Platform.OS, c.Platform.Arch = fact(os, now), fact(arch, now)
	c.Resources.CPU, c.Resources.MemMB = fact(float64(f.CPUs), now), fact(f.MemMB, now)
	c.Tools.Docker = fact(f.Docker, now)
	c.Tools.Toolchains, c.Tools.Custom = map[string]model.Fact[string]{}, map[string]model.Fact[string]{}
	for _, t := range f.Tools {
		c.Tools.Toolchains[t] = fact("present", now)
	}
	c.Availability.State, c.Availability.Slots, c.Availability.Running = fact("online", now), fact(max(1, f.CPUs/2), now), fact(0, now)
	c.Location.Kind = fact("local", now)
	c.Economics.Class = fact("local", now)
	c.Trust.Accepts = assigned([]model.TrustClass{"owner"}, now)
	c.Trust.Isolation = assigned(model.Isolation("process"), now)
	c.Trust.Network = assigned([]model.NetworkScope{"none", "internet", "restricted", "lan", "privileged"}, now)
	c.Trust.SecretClasses = assigned([]model.SecretClass{"none", "project", "environment", "team", "release", "deploy"}, now)
	c.Trust.PrivacyZone = assigned(model.PrivacyZone("local-only"), now)
	c.Protocol = model.CapabilityProtocol{Versions: []int{1}, Min: 1}
	c.Lifecycle = model.CapabilityLifecycle{AuthorizedProjects: []string{f.Project}, Eligible: true}
	c.Identity.HostKeyFingerprints = []string{}
	c.Verified = model.CapabilityVerified{By: "self", At: now}
	return c
}

// MarkPersonalLocal declares the runner "local" the operator's own node of a
// PERSONAL coordinator, which is what lets the owner's own checkout run there
// under process isolation (trust.Admissible owner exception). world.Build
// leaves these two facts unset (debt: move them into the builder, P7-SCHED-04
// needs the same). A runner that hosts a coordinator is left alone. Call it
// only when LocalCapability supplied the runner: a registered node that
// happens to be called "local" was never probed and must not get the exception.
func MarkPersonalLocal(w *sched.World) {
	for i := range w.Runners {
		r := &w.Runners[i]
		if r.ID != "local" || r.CoordinatorHost {
			continue
		}
		r.Decl.PersonalOwnNode = true
		r.CoordinatorMode = trust.Personal
		r.Decl.CoordinatorMode = trust.Personal
	}
}
