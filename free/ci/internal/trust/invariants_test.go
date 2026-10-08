package trust_test

import (
	"encoding/json"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/invariants"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"testing"
)

func includes(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}
func declaration(w invariants.World) trust.Declaration {
	return trust.Declaration{Accepts: []model.TrustClass{"owner", "internal", "collaborator"}, Isolation: w.Isolation, Network: w.Network, SecretClasses: w.Secrets, PrivacyZone: w.Privacy, Ownership: w.Ownership, UIDSeparation: true, NoLANRoute: true, CoordinatorMode: trust.DecodeCoordinatorMode(w.Mode)}
}
func job(w invariants.World) trust.JobFacts {
	return trust.JobFacts{Trust: w.Trust, SecretClasses: w.Secrets, Network: w.Network, Mode: trust.DecodeCoordinatorMode(w.Mode)}
}
func TestInvariant_I01(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		if w.Trust != "untrusted" {
			return nil
		}
		d := declaration(w)
		if !includes(trust.Admissible(job(w), d, trust.Effective{}), trust.TrustNotAccepted) {
			return fmt.Errorf("untrusted accepted")
		}
		return nil
	})
}
func TestInvariant_I03(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		d := declaration(w)
		d.Hosted = true
		d.Provider = "external"
		p := effectivePrivacy(t, w.Privacy)
		rs := trust.Admissible(job(w), d, p)
		if w.Privacy == "local-only" && !includes(rs, trust.PrivacyLocalOnly) {
			return fmt.Errorf("local-only hosted")
		}
		if w.Privacy == "private-infrastructure" && !includes(rs, trust.PrivacyHostedNotAuthorized) {
			return fmt.Errorf("private hosted")
		}
		if w.Privacy == "provider-allowlist" && !includes(rs, trust.PrivacyProviderNotAllowlisted) {
			return fmt.Errorf("unlisted provider")
		}
		return nil
	})
}
func TestInvariant_I04(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		if !hasRelease(w.Secrets) {
			return nil
		}
		d := declaration(w)
		d.Dedicated = false
		if trust.RankIsolation(w.Isolation) >= trust.RankIsolation("vm") {
			return nil
		}
		if !includes(trust.Admissible(job(w), d, trust.Effective{}), trust.TrustReleaseNeedsDedicated) {
			return fmt.Errorf("release below VM")
		}
		return nil
	})
}
func TestInvariant_I09(t *testing.T) {
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "trust.isolation_min", Owner: "trust", Kind: trust.TightenHigher, Order: []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}, Default: "process"})
	invariants.Check(t, func(w invariants.World) error {
		if w.Isolation == "hosted-disposable" {
			return nil
		}
		parent := trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.isolation_min": jsonString("container")}}
		child := trust.Scope{Kind: trust.SourceJob, Values: map[string]json.RawMessage{"trust.isolation_min": jsonString(string(w.Isolation))}}
		e, _, err := r.Merge("pin", parent, child)
		if trust.RankIsolation(w.Isolation) < trust.RankIsolation("container") {
			if err == nil {
				return fmt.Errorf("loosened without E670: %v", e)
			}
			return nil
		}
		if err != nil {
			return err
		}
		v, _, _ := e.Value("trust.isolation_min")
		if trust.RankIsolation(model.Isolation(v.(string))) < trust.RankIsolation("container") {
			return fmt.Errorf("effective loosened")
		}
		return nil
	})
}
func TestInvariant_I10(t *testing.T) {
	invariants.Check(t, func(w invariants.World) error {
		j := job(w)
		d := declaration(w)
		r := trust.RunnerFacts{Ownership: d.Ownership, Dedicated: d.Dedicated, UIDSeparation: d.UIDSeparation}
		first := trust.MinIsolation(j, r)
		j.Trust = "untrusted"
		next := trust.RetryIsolation(first, trust.MinIsolation(j, r))
		if trust.RankIsolation(next) < trust.RankIsolation(first) {
			return fmt.Errorf("retry lowered isolation")
		}
		return nil
	})
}
func TestInvariant_I16(t *testing.T) {
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "trust.network", Owner: "trust", Kind: trust.TightenLower, Order: []string{"none", "restricted", "internet", "lan", "privileged"}, Default: "lan"})
	invariants.Check(t, func(w invariants.World) error {
		base := trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{"trust.network": jsonString("restricted")}}
		e, d, err := r.Merge("pin", base)
		if err != nil {
			return err
		}
		if trust.RankNetwork(w.Network) <= trust.RankNetwork("restricted") {
			return nil
		}
		rev := trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"trust.network": jsonString(string(w.Network))}}
		e2, d2, err := r.Merge("pin", base, rev)
		if err != nil {
			return err
		}
		if d != d2 {
			return fmt.Errorf("revision widened digest")
		}
		v, _, _ := e2.Value("trust.network")
		old, _, _ := e.Value("trust.network")
		if v != old || len(e2.Ignored()) != 1 {
			return fmt.Errorf("revision widened effective")
		}
		return nil
	})
}
func hasRelease(s []model.SecretClass) bool {
	for _, x := range s {
		if x == "release" || x == "deploy" {
			return true
		}
	}
	return false
}
func jsonString(s string) json.RawMessage { b, _ := json.Marshal(s); return b }
func effectivePrivacy(t *testing.T, zone model.PrivacyZone) trust.Effective {
	t.Helper()
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	e, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": jsonString(string(zone))}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
