package trust_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/nodes/deployguard"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// pbSet builds the exclusion set the way production does: through the NODE-14 reader over the fixture runner.
func pbSet(t *testing.T) deployguard.Set {
	t.Helper()
	root := "/proj"
	runner := &deployguard.FixtureRunner{HelpSide: "read", HostsByRoot: map[string][]string{root: {deployguard.FixtureHostV4, deployguard.FixtureHostV6, deployguard.FixtureHostName}}}
	entries, err := deployguard.Reader{Run: runner.Run}.Read(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return deployguard.NewSet(entries)
}

// pbHost is one node spelling and what the NODE-14 matcher says about it.
type pbHost struct {
	name    string
	matched bool
}

// pbHosts returns every fixture spelling (all must match) plus unrelated hosts (none may match).
func pbHosts(t *testing.T) ([]pbHost, deployguard.Set) {
	t.Helper()
	set := pbSet(t)
	var hosts []pbHost
	for _, s := range deployguard.FixtureSpellings() {
		if !set.Match(deployguard.Facts{Names: []string{s}}).Matched {
			t.Fatalf("fixture spelling %q does not match the fixture set", s)
		}
		hosts = append(hosts, pbHost{s, true})
	}
	for _, s := range []string{deployguard.FixtureOther, "builder.example.test", "2001:db8::99"} {
		if set.Match(deployguard.Facts{Names: []string{s}}).Matched {
			t.Fatalf("unrelated host %q matches the fixture set", s)
		}
		hosts = append(hosts, pbHost{s, false})
	}
	return hosts, set
}

// pbFact is the deploy_host fact NODE-14 would store for the node.
func pbFact(set deployguard.Set, h pbHost, override bool) trust.DeployHost {
	f := deployguard.DeployHostFor(set.Match(deployguard.Facts{Names: []string{h.name}}), model.CapabilityDeployHost{Override: override})
	return trust.DeployHost{Matched: f.Matched, Override: f.Override}
}

func pbOwnClass(c model.TrustClass) bool { return c == "owner" || c == "internal" }
func pbVM(i model.Isolation) bool        { return trust.RankIsolation(i) >= trust.RankIsolation("vm") }
func pbRelease(s []model.SecretClass) bool {
	for _, c := range s {
		if c == "release" || c == "deploy" {
			return true
		}
	}
	return false
}

func TestInvariant_I15(t *testing.T) {
	hosts, set := pbHosts(t)
	n := 0
	seen := map[string]int{}
	t.Cleanup(func() {
		for _, k := range []string{"deploy-refused", "override-bad", "override-ok", "audit-admitted", "floor-applies", "floor-skipped"} {
			if seen[k] == 0 {
				t.Errorf("I15 never visited the %s case: the variants do not cover the product", k)
			}
		}
	})
	invariants.Check(t, func(w invariants.World) error {
		i := n
		n++
		h := hosts[i%len(hosts)]
		override := i/len(hosts)%2 == 1
		coord := i/(2*len(hosts))%2 == 1
		d := declaration(w)
		d.DeployHost = pbFact(set, h, override)
		d.CoordinatorHost = coord
		rs := trust.Admissible(job(w), d, trust.Effective{})
		own, vm := pbOwnClass(w.Trust), pbVM(w.Isolation)
		switch {
		case h.matched && !override:
			seen["deploy-refused"]++
			if !includes(rs, trust.BoundaryDeployHost) {
				return fmt.Errorf("deploy host %q selected without override (%s/%s)", h.name, w.Trust, w.Isolation)
			}
		case h.matched:
			bad := !own || !vm || pbRelease(w.Secrets)
			seen["override-ok"] += map[bool]int{true: 0, false: 1}[bad]
			seen["override-bad"] += map[bool]int{true: 1, false: 0}[bad]
			if bad && !includes(rs, trust.BoundaryOverrideMinimum) {
				return fmt.Errorf("override admitted trust=%s isolation=%s secrets=%v", w.Trust, w.Isolation, w.Secrets)
			}
			if !bad && includes(rs, trust.BoundaryOverrideMinimum) {
				return fmt.Errorf("override refused a job at the minimum")
			}
			if includes(rs, trust.BoundaryDeployHost) {
				return fmt.Errorf("overridden host refused as plain deploy host")
			}
		default:
			if includes(rs, trust.BoundaryDeployHost) || includes(rs, trust.BoundaryOverrideMinimum) {
				return fmt.Errorf("non-deploy host %q refused by the deploy boundary", h.name)
			}
		}
		if trust.RequiresAudit(d) != (h.matched && override) {
			return fmt.Errorf("audit requirement wrong for host %q override=%v", h.name, override)
		}
		if len(rs) == 0 && h.matched && override {
			seen["audit-admitted"]++
		}
		floor := coord && (w.Mode == "team" || !own) && !vm
		if coord {
			seen["floor-skipped"] += map[bool]int{true: 0, false: 1}[floor]
			seen["floor-applies"] += map[bool]int{true: 1, false: 0}[floor]
		}
		if floor != includes(rs, trust.BoundaryCoordinatorHost) {
			return fmt.Errorf("coordinator floor=%v mode=%s trust=%s isolation=%s coord=%v", floor, w.Mode, w.Trust, w.Isolation, coord)
		}
		return nil
	})
}

func pbDecl(iso model.Isolation) trust.Declaration {
	return trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator", "untrusted"}, Isolation: iso, Network: "lan", SecretClasses: []model.SecretClass{"none", "project", "environment", "team", "release", "deploy"}, UIDSeparation: true, NoLANRoute: true, Ownership: "operator", PersonalOwnNode: true}
}
func pbJob(class model.TrustClass, secrets ...model.SecretClass) trust.JobFacts {
	return trust.JobFacts{Trust: class, SecretClasses: secrets, Network: "restricted", PersonalOwnRun: true}
}
func pbBoundary(rs []trust.Reason) []trust.Reason {
	var out []trust.Reason
	for _, r := range rs {
		switch r {
		case trust.BoundaryDeployHost, trust.BoundaryOverrideMinimum, trust.BoundaryCoordinatorHost:
			out = append(out, r)
		}
	}
	return out
}

func TestProdBoundary_DeployHostNeedsOverride(t *testing.T) {
	d := pbDecl("ephemeral-vm")
	d.DeployHost = trust.DeployHost{Matched: true}
	for _, class := range []model.TrustClass{"owner", "internal", "collaborator", "untrusted", "bogus"} {
		rs := pbBoundary(trust.Admissible(pbJob(class), d, trust.Effective{}))
		if len(rs) != 1 || rs[0] != trust.BoundaryDeployHost {
			t.Fatalf("%s: got %v want [boundary.deploy_host]", class, rs)
		}
	}
	d.DeployHost = trust.DeployHost{Override: true}
	if rs := pbBoundary(trust.Admissible(pbJob("untrusted"), d, trust.Effective{})); len(rs) != 0 {
		t.Fatalf("override flag without a match refused: %v", rs)
	}
}

func TestProdBoundary_OverrideMinimum(t *testing.T) {
	cases := []struct {
		name    string
		class   model.TrustClass
		iso     model.Isolation
		secrets []model.SecretClass
		refused bool
	}{
		{"owner vm", "owner", "vm", nil, false},
		{"internal ephemeral-vm", "internal", "ephemeral-vm", []model.SecretClass{"project", "team"}, false},
		{"owner container", "owner", "container", nil, true},
		{"owner process", "owner", "process", nil, true},
		{"owner sandboxed", "owner", "sandboxed-container", nil, true},
		{"collaborator vm", "collaborator", "vm", nil, true},
		{"untrusted vm", "untrusted", "ephemeral-vm", nil, true},
		{"unknown class vm", "bogus", "vm", nil, true},
		{"empty class vm", "", "vm", nil, true},
		{"owner vm release", "owner", "vm", []model.SecretClass{"release"}, true},
		{"internal vm deploy", "internal", "vm", []model.SecretClass{"deploy"}, true},
		{"owner vm unknown secret", "owner", "vm", []model.SecretClass{"bogus"}, true},
		{"owner unknown isolation", "owner", "bogus", nil, true},
		{"owner empty isolation", "owner", "", nil, true},
	}
	for _, c := range cases {
		d := pbDecl(c.iso)
		d.DeployHost = trust.DeployHost{Matched: true, Override: true}
		rs := pbBoundary(trust.Admissible(pbJob(c.class, c.secrets...), d, trust.Effective{}))
		if c.refused && (len(rs) != 1 || rs[0] != trust.BoundaryOverrideMinimum) {
			t.Errorf("%s: got %v want [boundary.override_minimum]", c.name, rs)
		}
		if !c.refused && len(rs) != 0 {
			t.Errorf("%s: got %v want none", c.name, rs)
		}
	}
}

func TestProdBoundary_RequiresAudit(t *testing.T) {
	for _, c := range []struct {
		matched, override, want bool
	}{{true, true, true}, {true, false, false}, {false, true, false}, {false, false, false}} {
		d := pbDecl("vm")
		d.DeployHost = trust.DeployHost{Matched: c.matched, Override: c.override}
		if got := trust.RequiresAudit(d); got != c.want {
			t.Errorf("matched=%v override=%v: RequiresAudit=%v want %v", c.matched, c.override, got, c.want)
		}
	}
	d := pbDecl("vm")
	d.DeployHost = trust.DeployHost{Matched: true, Override: true}
	if rs := trust.Admissible(pbJob("owner"), d, trust.Effective{}); len(rs) != 0 || !trust.RequiresAudit(d) {
		t.Fatalf("admissible owner placement on an overridden host: reasons %v audit %v", rs, trust.RequiresAudit(d))
	}
}

func TestProdBoundary_CoordinatorHost(t *testing.T) {
	cases := []struct {
		name    string
		mode    trust.CoordinatorMode
		class   model.TrustClass
		iso     model.Isolation
		refused bool
	}{
		{"personal owner process", trust.Personal, "owner", "process", false},
		{"personal internal process", trust.Personal, "internal", "process", false},
		{"personal internal container", trust.Personal, "internal", "container", false},
		{"personal collaborator container", trust.Personal, "collaborator", "container", true},
		{"personal collaborator vm", trust.Personal, "collaborator", "vm", false},
		{"personal untrusted sandboxed", trust.Personal, "untrusted", "sandboxed-container", true},
		{"personal unknown class", trust.Personal, "bogus", "container", true},
		{"personal empty class", trust.Personal, "", "container", true},
		{"personal owner empty isolation", trust.Personal, "owner", "", false},
		{"team owner empty isolation", trust.Team, "owner", "", true},
		{"team owner unknown isolation", trust.Team, "owner", "bogus", true},
		{"team owner process", trust.Team, "owner", "process", true},
		{"team owner container", trust.Team, "owner", "container", true},
		{"team internal sandboxed", trust.Team, "internal", "sandboxed-container", true},
		{"team owner vm", trust.Team, "owner", "vm", false},
		{"team untrusted ephemeral-vm", trust.Team, "untrusted", "ephemeral-vm", false},
		{"unknown mode owner container", "bogus", "owner", "container", true},
		{"empty mode owner process", "", "owner", "process", true},
		{"empty mode owner vm", "", "owner", "vm", false},
	}
	for _, c := range cases {
		d := pbDecl(c.iso)
		d.CoordinatorHost, d.CoordinatorMode = true, c.mode
		rs := pbBoundary(trust.Admissible(pbJob(c.class), d, trust.Effective{}))
		if c.refused && (len(rs) != 1 || rs[0] != trust.BoundaryCoordinatorHost) {
			t.Errorf("%s: got %v want [boundary.coordinator_host]", c.name, rs)
		}
		if !c.refused && len(rs) != 0 {
			t.Errorf("%s: got %v want none", c.name, rs)
		}
		d.CoordinatorHost = false
		if rs := pbBoundary(trust.Admissible(pbJob(c.class), d, trust.Effective{})); len(rs) != 0 {
			t.Errorf("%s: not a coordinator host but refused: %v", c.name, rs)
		}
	}
}

func TestProdBoundary_BothBoundariesReport(t *testing.T) {
	d := pbDecl("container")
	d.DeployHost = trust.DeployHost{Matched: true, Override: true}
	d.CoordinatorHost, d.CoordinatorMode = true, trust.Team
	rs := pbBoundary(trust.Admissible(pbJob("collaborator", "release"), d, trust.Effective{}))
	if len(rs) != 2 || rs[0] != trust.BoundaryOverrideMinimum || rs[1] != trust.BoundaryCoordinatorHost {
		t.Fatalf("got %v", rs)
	}
}

func TestProdBoundary_ReasonsKnown(t *testing.T) {
	for _, r := range []trust.Reason{trust.BoundaryDeployHost, trust.BoundaryOverrideMinimum, trust.BoundaryCoordinatorHost} {
		if trust.ReasonMeaning[r] == "" {
			t.Errorf("%s has no meaning", r)
		}
	}
}

// TestProdBoundary_ReasonsSurfaceE689 (D-1250): the three boundary reasons map to the registered
// E689; other reasons keep no code of their own.
func TestProdBoundary_ReasonsSurfaceE689(t *testing.T) {
	for _, r := range []trust.Reason{trust.BoundaryDeployHost, trust.BoundaryOverrideMinimum, trust.BoundaryCoordinatorHost} {
		if got := trust.ReasonCode(r); got != "E689" {
			t.Errorf("%s: code %q, want E689", r, got)
		}
	}
	if got := trust.ReasonCode(trust.IsolationBelowMinimum); got != "" {
		t.Errorf("isolation.below_minimum: code %q, want none", got)
	}
}
