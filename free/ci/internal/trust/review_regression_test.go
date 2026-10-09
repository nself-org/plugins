package trust_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func TestReviewUnknownRequirementAndClaim(t *testing.T) {
	if got := trust.DecodeIsolation("future"); got != "ephemeral-vm" {
		t.Fatalf("unknown isolation requirement: %s", got)
	}
	job := trust.JobFacts{Trust: "owner", Network: "none", SecretClasses: []model.SecretClass{"future-release"}}
	d := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "container", Network: "none", SecretClasses: job.SecretClasses, UIDSeparation: true}
	r := trust.Admissible(job, d, trust.Effective{})
	if !includes(r, trust.TrustReleaseNeedsDedicated) || !includes(r, trust.IsolationBelowMinimum) {
		t.Fatalf("unknown secret escaped release floor: %v", r)
	}
	d.Isolation = "future"
	if !includes(trust.Admissible(trust.JobFacts{Trust: "owner", Network: "none"}, d, trust.Effective{}), trust.IsolationBelowMinimum) {
		t.Fatal("unknown runner claim admitted")
	}
}

func TestReviewHostedRequiresProjectAuthorization(t *testing.T) {
	d := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "hosted-disposable", Network: "none", PrivacyZone: "hosted-allowed", Hosted: true, UIDSeparation: true}
	if !includes(trust.Admissible(trust.JobFacts{Trust: "owner", Network: "none"}, d, trust.Effective{}), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("runner claim authorized hosting")
	}
}

func TestReviewRegistryDefaultNeverAuthorizesHosted(t *testing.T) {
	r, err := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	if err != nil {
		t.Fatal(err)
	}
	d := trust.Declaration{Accepts: []model.TrustClass{"untrusted", "collaborator", "internal", "owner"}, Isolation: "hosted-disposable", Network: "restricted", Hosted: true, UIDSeparation: true}
	for _, class := range d.Accepts {
		e, _, err := r.Merge("pin")
		if err != nil {
			t.Fatal(err)
		}
		if !includes(trust.Admissible(trust.JobFacts{Trust: class, Network: "restricted"}, d, e), trust.PrivacyHostedNotAuthorized) {
			t.Fatalf("registry default authorized hosted %s", class)
		}
	}
	for _, source := range []trust.Source{trust.SourceDefault, trust.SourcePipeline, trust.SourceJob, trust.SourceRevision} {
		e, _, err := r.Merge("pin", trust.Scope{Kind: source, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
		if err != nil {
			t.Fatal(err)
		}
		if !includes(trust.Admissible(trust.JobFacts{Trust: "untrusted", Network: "restricted"}, d, e), trust.PrivacyHostedNotAuthorized) {
			t.Fatalf("%s scope authorized hosted without project or public visibility", source)
		}
	}
	publicScope := trust.DefaultPolicy("public", "untrusted")
	publicScope.Values = map[string]json.RawMessage{"trust.privacy_zone": publicScope.Values["trust.privacy_zone"]}
	public, _, err := r.Merge("pin", publicScope)
	if err != nil {
		t.Fatal(err)
	}
	if includes(trust.Admissible(trust.JobFacts{Trust: "untrusted", Network: "restricted"}, d, public), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("public visibility failed to authorize untrusted hosted")
	}
	project, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
	if err != nil {
		t.Fatal(err)
	}
	if includes(trust.Admissible(trust.JobFacts{Trust: "owner", Network: "restricted"}, d, project), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("explicit project authorization rejected owner hosted")
	}
}

func TestReviewDefaultPolicyOmittedClassIsUntrusted(t *testing.T) {
	want := map[string]string{"trust.isolation_min": `"sandboxed-container"`, "trust.network": `"restricted"`, "trust.secrets": `[]`, "trust.approval.first_time": `true`}
	for key, value := range want {
		if got := string(trust.DefaultPolicy("public").Values[key]); got != value {
			t.Errorf("omitted class %s = %q, want %q", key, got, value)
		}
	}
}

func TestReviewRetryUnknownPriorNeverLowers(t *testing.T) {
	for _, previous := range []model.Isolation{"future-isolation", ""} {
		for _, required := range []model.Isolation{"process", "container", "vm"} {
			if got := trust.RetryIsolation(previous, required); got != "ephemeral-vm" {
				t.Errorf("retry %q -> %q lowered to %q", previous, required, got)
			}
		}
	}
}

func TestReviewRevisionPinAndMissingKey(t *testing.T) {
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "trust.network", Owner: "trust", Kind: trust.TightenLower, Order: []string{"none", "restricted", "lan"}, Default: "lan"}, trust.KeySpec{Key: "trust.providers", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"a", "b"}})
	_, digest, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{"trust.network": json.RawMessage(`"restricted"`)}})
	if err != nil {
		t.Fatal(err)
	}
	next, got, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{"trust.network": json.RawMessage(`"restricted"`)}}, trust.Scope{Kind: trust.SourceRevision, PinnedCommit: "evil", Values: map[string]json.RawMessage{"trust.network": json.RawMessage(`"lan"`)}})
	if err != nil || got != digest || len(next.Ignored()) != 1 {
		t.Fatalf("pin changed: %s %s %v", got, digest, err)
	}
	r2, _ := trust.NewRegistry(trust.KeySpec{Key: "sched.*", Owner: "sched", Kind: trust.TightenLower, Order: []string{"none", "restricted", "lan"}, Default: "lan"})
	e, _, err := r2.Merge("pin", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"sched.network": json.RawMessage(`"none"`)}})
	v, src, ok := e.Value("sched.network")
	if err != nil || !ok || v != "none" || src != trust.SourceRevision {
		t.Fatalf("missing key tightening: %v %s %v", v, src, err)
	}
	e, _, err = r2.Merge("pin", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"sched.network": json.RawMessage(`"future"`)}})
	v, _, ok = e.Value("sched.network")
	if err != nil || !ok || v != "none" {
		t.Fatalf("unknown network ceiling: %v %v", v, err)
	}
	_, baselineDigest, err := r2.Merge("pin")
	if err != nil {
		t.Fatal(err)
	}
	_, widenedDigest, err := r2.Merge("pin", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"sched.network": json.RawMessage(`"lan"`)}})
	if err != nil || widenedDigest == baselineDigest {
		t.Fatalf("registered default equality should be permitted: %v", err)
	}
	r3, _ := trust.NewRegistry(trust.KeySpec{Key: "sched.*", Owner: "sched", Kind: trust.TightenLower, Order: []string{"none", "restricted", "lan"}, Default: "restricted"})
	_, baselineDigest, err = r3.Merge("pin")
	if err != nil {
		t.Fatal(err)
	}
	widened, widenedDigest, err := r3.Merge("pin", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"sched.network": json.RawMessage(`"lan"`)}})
	if err != nil || widenedDigest != baselineDigest || len(widened.Ignored()) != 1 || !strings.Contains(widened.Ignored()[0].Reason, "E671") {
		t.Fatalf("missing key widened default: %v %s %v", err, widenedDigest, widened.Ignored())
	}
	_, _, err = r.Merge("pin", trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"trust.providers": json.RawMessage(`["a","c"]`)}})
	if err != nil {
		t.Fatalf("revision widening should be ignored: %v", err)
	}
}

func TestReviewDefaultPolicyTrustedJobs(t *testing.T) {
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "trust.secrets", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"project"}}, trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"}, trust.KeySpec{Key: "trust.approval.first_time", Owner: "trust", Kind: trust.TightenOnlyOn, Default: false})
	_, _, err := r.Merge("pin", trust.DefaultPolicy("public", "owner"), trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.secrets": json.RawMessage(`["project"]`)}})
	if err != nil {
		t.Fatalf("default blocked trusted project secret: %v", err)
	}
	for _, key := range []string{"trust.isolation_min", "trust.network", "trust.secrets"} {
		if _, ok := trust.DefaultPolicy("public").Values[key]; !ok {
			t.Fatalf("omitted class missing untrusted preset: %s", key)
		}
		if _, ok := trust.DefaultPolicy("public", "owner").Values[key]; ok {
			t.Fatalf("owner received untrusted preset: %s", key)
		}
		if _, ok := trust.DefaultPolicy("public", "untrusted").Values[key]; !ok {
			t.Fatalf("untrusted missing preset: %s", key)
		}
	}
	if _, ok := trust.DefaultPolicy("public", "owner").Values["trust.approval.first_time"]; ok {
		t.Fatal("owner received first-time contributor gate")
	}
	if _, ok := trust.DefaultPolicy("public", "untrusted").Values["trust.approval.first_time"]; !ok {
		t.Fatal("public untrusted missing first-time gate")
	}
}

func TestReviewPersonalReleaseAndWildcard(t *testing.T) {
	d := trust.Declaration{Ownership: "operator", Accepts: []model.TrustClass{"owner", "internal"}, Projects: 1, PersonalOwnNode: true, PersonalOwnRun: true, ReleaseJob: true}
	if !trust.IsDedicatedTrusted(d) {
		t.Fatal("own personal release rejected")
	}
	d.Accepts = append(d.Accepts, "untrusted")
	if trust.IsDedicatedTrusted(d) {
		t.Fatal("untrusted acceptance allowed release exception")
	}
	r, err := trust.NewRegistry(trust.KeySpec{Key: "sched.*", Owner: "sched", Kind: trust.TightenAny})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Find("sched.unregistered.nested"); ok {
		t.Fatal("wildcard matched two segments")
	}
	if _, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"sched.unregistered.nested": json.RawMessage(`true`)}}); err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("unknown key: %v", err)
	}
}

func TestReviewDigestBindsHostedAuthorization(t *testing.T) {
	r, err := trust.NewRegistry(trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"})
	if err != nil {
		t.Fatal(err)
	}
	_, none, err := r.Merge("pin")
	if err != nil {
		t.Fatal(err)
	}
	e, project, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.privacy_zone": json.RawMessage(`"hosted-allowed"`)}})
	if err != nil {
		t.Fatal(err)
	}
	d := trust.Declaration{Accepts: []model.TrustClass{"untrusted"}, Isolation: "hosted-disposable", Network: "restricted", Hosted: true, UIDSeparation: true}
	if includes(trust.Admissible(trust.JobFacts{Trust: "untrusted", Network: "restricted"}, d, e), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("project scope did not authorize hosting")
	}
	if none == project {
		t.Fatalf("digest %s does not distinguish an unauthorized policy from a project-authorized one", none)
	}
}
