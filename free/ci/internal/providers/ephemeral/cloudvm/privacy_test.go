package cloudvm_test

// Purpose: TestPrivacyDefault* and the refusal tests of the selection rules. With the default burst.trust
// (privacy zone provider-allowlist, isolation ephemeral-vm) an enrolled burst node is never admissible
// for a project in the default zone (I03: private jobs never hosted); the operator scope that names the
// zone and the provider admits it. In PERSONAL mode, or without PaidAllowed, the engine never selects
// the provider and no child runs.
// Inputs:  the rig; trust.Admissible over the declaration of the enrolled node. Outputs: pass/fail.

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func privacyRegistry(t *testing.T) trust.Registry {
	t.Helper()
	reg, err := trust.NewRegistry(
		trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}},
		trust.KeySpec{Key: "trust.providers", Owner: "trust", Kind: trust.TightenSubset})
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func reasons(rs []trust.Reason, want ...trust.Reason) bool {
	for _, w := range want {
		if !slices.Contains(rs, w) {
			return false
		}
	}
	return true
}

func TestPrivacyDefaultKeepsPrivateProjectsOffTheBurstNode(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.serve()
	res := r.provision("job-1")
	eventually(t, 2*time.Second, "the listener opens", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(res.Token)
	if _, err := r.join(code, fp, cloudvm.NodeName(res.Token)); err != nil {
		t.Fatal(err)
	}
	n, err := r.reg.Get(ctx, cloudvm.NodeName(res.Token))
	if err != nil {
		t.Fatal(err)
	}
	d := sched.DeclarationFromCapability(n.Capability, false, trust.Team)
	if !d.Hosted || d.Provider != "hetzner" || d.PrivacyZone != "provider-allowlist" || d.Isolation != "ephemeral-vm" {
		t.Fatalf("declaration of the burst node: %+v", d)
	}
	job := trust.JobFacts{Trust: "owner", Network: "restricted"}
	reg := privacyRegistry(t)

	// 1. a project that never named a zone is private-infrastructure: the node is refused
	none, _, err := reg.Merge("pin")
	if err != nil {
		t.Fatal(err)
	}
	if !reasons(trust.Admissible(job, d, none), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("a project in the default zone was admitted on a burst node")
	}
	// 2. the project names private-infrastructure itself: refused as well
	priv, _, err := reg.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"private-infrastructure"`)}})
	if err != nil {
		t.Fatal(err)
	}
	if !reasons(trust.Admissible(job, d, priv), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("a private-infrastructure project was admitted on a burst node")
	}
	// 3. the operator-scope override (team scope): it allows provider-allowlist and its allowlist names the provider
	// (the real policy engine cannot loosen a private repository's baseline yet: debt D-1382)
	allow := trust.Scope{Kind: trust.SourceTeam, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"provider-allowlist"`), "trust.providers": json.RawMessage(`["hetzner"]`)}}
	ok, _, err := reg.Merge("pin", allow)
	if err != nil {
		t.Fatal(err)
	}
	if got := trust.Admissible(job, d, ok); reasons(got, trust.PrivacyHostedNotAuthorized) || reasons(got, trust.PrivacyProviderNotAllowlisted) {
		t.Fatalf("the override must admit the node on privacy: %v", got)
	}
	// 4. the zone without the provider on the allowlist is still refused
	allow.Values["trust.providers"] = json.RawMessage(`["ephemeral"]`)
	other, _, err := reg.Merge("pin", allow)
	if err != nil {
		t.Fatal(err)
	}
	if !reasons(trust.Admissible(job, d, other), trust.PrivacyProviderNotAllowlisted) {
		t.Fatal("a provider that is not on the allowlist was admitted")
	}
}

func TestPrivacyDefaultTrustFieldsComeFromBurstTrust(t *testing.T) {
	r := newRig(t, func(r *rig) {
		r.pol.Burst.Trust.Accepts = []string{"owner"}
		r.pol.Burst.Trust.PrivacyZone = "provider-allowlist"
	})
	ctx := context.Background()
	r.serve()
	res := r.provision("job-1")
	eventually(t, 2*time.Second, "the listener opens", func() bool { return isOpen(r.enrollAdr) })
	code, fp := r.userDataOf(res.Token)
	if _, err := r.join(code, fp, cloudvm.NodeName(res.Token)); err != nil {
		t.Fatal(err)
	}
	n, err := r.reg.Get(ctx, cloudvm.NodeName(res.Token))
	if err != nil {
		t.Fatal(err)
	}
	if got := *n.Capability.Trust.Accepts.Value; len(got) != 1 || got[0] != model.TrustClass("owner") {
		t.Fatalf("accepts %v: the narrowed burst.trust must apply", got)
	}
}

func TestCloudVMNeverSelectedInPersonalOrWithoutPaidAllowed(t *testing.T) {
	cases := map[string]struct {
		tweak  func(*rig)
		reason string
	}{
		"personal mode":    {func(r *rig) { r.team = false }, string(model.BurstTeamOnly)},
		"paid not enabled": {func(r *rig) { r.pol.PaidEnabled = false }, string(model.PaidDisabled)},
		"no budget at all": {func(r *rig) { r.pol.Budgets.Day.Set = false; r.pol.Budgets.Month.Set = false }, string(model.PaidDisabled)},
		"provider not on":  {func(r *rig) { r.pol.Burst.Enabled = map[string]bool{} }, burst.ReasonDisabled},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := newRig(t, c.tweak)
			_, err := r.eng.Provision(context.Background(), r.request("job-1"))
			if burst.CodeOf(err) != burst.CodeProvisionRefused || burst.ReasonOf(err) != c.reason {
				t.Fatalf("want E696 %s, got %v", c.reason, err)
			}
			if n := len(r.br.recorded()); n != 0 || len(r.cloud.All()) != 0 {
				t.Fatalf("a refused selection ran %d children and left %d servers", n, len(r.cloud.All()))
			}
		})
	}
}
