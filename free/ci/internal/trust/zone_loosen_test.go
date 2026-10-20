package trust_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// zoneRegistry is the privacy-zone key as the policy keyset registers it.
func zoneRegistry(t *testing.T) trust.Registry {
	t.Helper()
	r, err := trust.NewRegistry(
		trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"},
		trust.KeySpec{Key: "trust.providers", Owner: "trust", Kind: trust.TightenSubset},
	)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func zoneRow(kind trust.Source, zone string) trust.Scope {
	return trust.Scope{Kind: kind, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"` + zone + `"`)}}
}

func hostedAdmitted(e trust.Effective) bool {
	d := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "hosted-disposable", Network: "none", Hosted: true, UIDSeparation: true}
	return !includes(trust.Admissible(trust.JobFacts{Trust: "owner", Network: "none"}, d, e), trust.PrivacyHostedNotAuthorized)
}

// TestOperatorRowLoosensPrivateBaseline is the D-1382 regression: a user, team or project row may loosen
// a private repository's baseline zone (and authorize hosted providers); a revision and a run may not.
func TestOperatorRowLoosensPrivateBaseline(t *testing.T) {
	r := zoneRegistry(t)
	base := trust.DefaultPolicy("private", "owner")
	for _, kind := range []trust.Source{trust.SourceUser, trust.SourceTeam, trust.SourceProject} {
		for _, zone := range []string{"provider-allowlist", "hosted-allowed"} {
			e, _, err := r.Merge("pin", base, zoneRow(kind, zone))
			if err != nil {
				t.Fatalf("%s row %s on a private baseline: %v", kind, zone, err)
			}
			if got, _, _ := e.Value("trust.privacy_zone"); got != zone {
				t.Fatalf("%s row %s: effective zone %v", kind, zone, got)
			}
			if !hostedAdmitted(e) {
				t.Fatalf("%s row %s did not authorize hosted", kind, zone)
			}
		}
	}
	// The decision is bound in the digest.
	_, plain, _ := r.Merge("pin", base)
	_, authorized, _ := r.Merge("pin", base, zoneRow(trust.SourceProject, "provider-allowlist"))
	if plain == authorized {
		t.Fatal("digest does not bind the loosened zone")
	}
}

// TestLoosenedZoneStaysTightenOnlyAfterwards: once an operator loosened the baseline, later scopes and the
// revision and run can only tighten, and an operator scope cannot loosen a second time.
func TestLoosenedZoneStaysTightenOnlyAfterwards(t *testing.T) {
	r := zoneRegistry(t)
	base := trust.DefaultPolicy("private", "owner")
	user := zoneRow(trust.SourceUser, "provider-allowlist")
	// A revision claiming a looser zone changes nothing.
	e, _, err := r.Merge("pin", base, user, zoneRow(trust.SourceRevision, "hosted-allowed"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _, _ := e.Value("trust.privacy_zone"); got != "provider-allowlist" || len(e.Ignored()) != 1 {
		t.Fatalf("revision widened the zone: %v %v", got, e.Ignored())
	}
	// A revision alone on a private baseline cannot loosen it either.
	e, _, err = r.Merge("pin", base, zoneRow(trust.SourceRevision, "hosted-allowed"))
	if err != nil || hostedAdmitted(e) {
		t.Fatalf("revision alone authorized hosted: %v", err)
	}
	// A second operator scope cannot loosen past the first.
	if _, _, err = r.Merge("pin", base, user, zoneRow(trust.SourceProject, "hosted-allowed")); err == nil || !strings.HasPrefix(err.Error(), "E670") {
		t.Fatalf("project loosened past the user row: %v", err)
	}
	// It can tighten.
	e, _, err = r.Merge("pin", base, user, zoneRow(trust.SourceProject, "private-infrastructure"))
	if err != nil || hostedAdmitted(e) {
		t.Fatalf("tightening failed: %v", err)
	}
	// The protected-branch scope never loosens the baseline.
	if _, _, err = r.Merge("pin", base, zoneRow(trust.SourceProtectedBranch, "hosted-allowed")); err == nil {
		t.Fatal("protected-branch scope loosened the baseline")
	}
}
