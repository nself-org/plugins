//go:build security

package redteam

// Purpose: isolating regression tests added after the mutation review of the HELD claims (P7-TRUST-18).
// Each test pins ONE control that another layer or fixture used to mask:
//   - A01: the netscope host-address refusal, with host addresses outside every on-link prefix;
//   - A20: the deploy-host refusal in the scheduler and in the trust boundary, each on its own;
//   - B-ns: the cache namespace canonical encoding round trip and the untrusted+protected rule.
// Inputs:  netscope.Allowed, sched.Place, trust.Admissible, authz.Namespace over hand-built values.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: pure functions; no network, no disk.

import (
	"net/netip"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/netscope"
	"github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// A01: a job asks for an address the host itself answers on. No on-link prefix covers it, so only the
// host-address rule can refuse it.
func TestA01_HostAddressRefusedWithoutAnOnLinkPrefix(t *testing.T) {
	f := netscope.Facts{HostAddrs: []netip.Addr{netip.MustParseAddr("203.0.115.7"), netip.MustParseAddr("::ffff:203.0.115.8")}}
	for _, scope := range []model.NetworkScope{"restricted", "internet"} {
		if v := netscope.Allowed(scope, netip.MustParseAddr("203.0.115.9"), f); !v.Allowed {
			t.Fatalf("control (%s): an address that is not a host address must be allowed, got %+v", scope, v)
		}
		for _, host := range []string{"203.0.115.7", "::ffff:203.0.115.7", "203.0.115.8"} {
			v := netscope.Allowed(scope, netip.MustParseAddr(host), f)
			if v.Allowed || v.Reason != netscope.ReasonHostAddr {
				t.Errorf("%s: host address %s: got %+v, want a %s refusal", scope, host, v, netscope.ReasonHostAddr)
			}
		}
	}
}

// A20 (scheduler layer): a runner that matched a deploy host is never eligible, whatever the trust layer says.
func TestA20_SchedulerRefusesADeployHostOnItsOwn(t *testing.T) {
	res := sched.Place(aWorld(nil, aRunner("deploy", "ephemeral-vm", true), aRunner("plain", "ephemeral-vm", false)), []sched.Job{aJob("k", 1)})
	if len(res.Placements) != 1 || res.Placements[0].RunnerID != "plain" {
		t.Fatalf("control: the plain runner must be the one placement: %+v", res.Placements)
	}
	seen := false
	for _, x := range res.Explanations {
		for _, alt := range x.Alternatives {
			has := false
			for _, r := range alt.Reasons {
				has = has || r.Code == string(model.NodeDeployHost)
			}
			switch {
			case alt.RunnerID == "deploy":
				seen = true
				if alt.Eligible || !has {
					t.Errorf("deploy host alternative: eligible=%v reasons=%v, want ineligible with %s", alt.Eligible, alt.Reasons, model.NodeDeployHost)
				}
			case has:
				t.Errorf("runner %s carries %s without being a deploy host", alt.RunnerID, model.NodeDeployHost)
			}
		}
	}
	if !seen {
		t.Fatal("the explanation lists no deploy host alternative")
	}
}

// A20 (trust boundary layer): the same refusal from trust.Admissible alone, plus the owner override control.
func TestA20_TrustBoundaryRefusesADeployHostOnItsOwn(t *testing.T) {
	eff := aPolicy(t)
	job := trust.JobFacts{Trust: "owner", Network: "restricted"}
	d := aSandboxNode()
	d.Isolation, d.Ownership = "vm", "operator"
	if rs := trust.Admissible(job, d, eff); len(rs) != 0 {
		t.Fatalf("control: a vm node that is not a deploy host admits the owner job, got %v", rs)
	}
	d.DeployHost = trust.DeployHost{Matched: true}
	if rs := trust.Admissible(job, d, eff); !aHas(rs, trust.BoundaryDeployHost) {
		t.Errorf("a matched deploy host without override was not refused at the boundary: %v", rs)
	}
	d.DeployHost = trust.DeployHost{Matched: true, Override: true}
	if rs := trust.Admissible(job, d, eff); aHas(rs, trust.BoundaryDeployHost) {
		t.Errorf("an overridden deploy host must not carry the plain deploy-host reason: %v", rs)
	}
}

// B-ns: the canonical bytes are a stored field. They must decode back to the same namespace for any project
// text, including NUL bytes, and an untrusted namespace is never protected in any path.
func TestB_Namespace_CanonicalRoundTripAndUntrustedNeverProtected(t *testing.T) {
	for _, ns := range []authz.Namespace{
		{Project: "proj", Trust: "owner", Protection: authz.Protected},
		{Project: "a\x00\x00\x00\x00b", Trust: "collaborator", Protection: authz.Unprotected},
		{Project: "p", Trust: "untrusted", Protection: authz.Unprotected, HeadRepo: "x\x00\x00\x00\x03y", PR: "7"},
	} {
		back, err := authz.DecodeNamespace(ns.Canonical())
		if err != nil || back != ns || !ns.Equal(back) {
			t.Errorf("round trip of %+v gave %+v, %v", ns, back, err)
		}
	}
	bad := authz.Namespace{Project: "p", Trust: "untrusted", Protection: authz.Protected, HeadRepo: "fork/x", PR: "9"}
	if bad.Valid() || bad.Equal(bad) {
		t.Errorf("an untrusted+protected namespace is valid: %+v", bad)
	}
	if _, err := authz.DecodeNamespace(bad.Canonical()); err == nil {
		t.Error("DecodeNamespace accepted the canonical bytes of an untrusted+protected namespace")
	}
	if authz.IsSnapshotSource(bad) {
		t.Error("an untrusted+protected namespace is a snapshot source")
	}
	good := bad
	good.Protection = authz.Unprotected
	if !good.Valid() {
		t.Errorf("control: the unprotected untrusted namespace must be valid: %+v", good)
	}
}
