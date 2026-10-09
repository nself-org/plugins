package trust_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/trust"
	"math/rand"
	"strings"
	"testing"
)

func TestAdmissibleOrdersAndDecode(t *testing.T) {
	for i, v := range []model.TrustClass{"untrusted", "collaborator", "internal", "owner"} {
		if trust.RankTrust(v) != i {
			t.Fatalf("trust %s", v)
		}
	}
	for i, v := range []model.Isolation{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"} {
		if trust.RankIsolation(v) != i {
			t.Fatalf("isolation %s", v)
		}
	}
	if trust.RankIsolation("hosted-disposable") != 4 {
		t.Fatal("hosted rank")
	}
	for i, v := range []model.NetworkScope{"none", "restricted", "internet", "lan", "privileged"} {
		if trust.RankNetwork(v) != i {
			t.Fatalf("network %s", v)
		}
	}
	for i, v := range []model.PrivacyZone{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"} {
		if trust.RankPrivacy(v) != i {
			t.Fatalf("privacy %s", v)
		}
	}
	if trust.DecodeTrust("alien") != "untrusted" || trust.DecodeIsolation("alien") != "ephemeral-vm" || trust.DecodeNetwork("alien") != "none" || trust.DecodePrivacy("alien") != "local-only" {
		t.Fatal("unknown values not restrictive")
	}
	for _, x := range []struct {
		raw  string
		want trust.CoordinatorMode
	}{{"personal", trust.Personal}, {"team", trust.Team}, {"", trust.Team}, {"unknown", trust.Team}} {
		if got := trust.DecodeCoordinatorMode(x.raw); got != x.want {
			t.Fatalf("mode %q: %s", x.raw, got)
		}
	}
}
func TestAdmissibleMinIsolationBranches(t *testing.T) {
	personal := trust.RunnerFacts{Ownership: "operator", PersonalOwnNode: true}
	for _, tc := range []struct {
		name   string
		job    trust.JobFacts
		runner trust.RunnerFacts
		want   model.Isolation
	}{
		{"owner personal", trust.JobFacts{Trust: "owner", PersonalOwnRun: true}, personal, "process"},
		{"internal personal", trust.JobFacts{Trust: "internal", PersonalOwnRun: true}, personal, "process"},
		{"owner remote", trust.JobFacts{Trust: "owner"}, trust.RunnerFacts{}, "container"},
		{"collaborator", trust.JobFacts{Trust: "collaborator"}, personal, "container"},
		{"release", trust.JobFacts{Trust: "owner", SecretClasses: []model.SecretClass{"release"}}, trust.RunnerFacts{}, "vm"},
		{"deploy", trust.JobFacts{Trust: "owner", SecretClasses: []model.SecretClass{"deploy"}}, trust.RunnerFacts{}, "vm"},
		{"untrusted", trust.JobFacts{Trust: "untrusted"}, trust.RunnerFacts{NoLANRoute: true}, "sandboxed-container"},
		{"hosted", trust.JobFacts{Trust: "untrusted"}, trust.RunnerFacts{Hosted: true}, "hosted-disposable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := trust.MinIsolation(tc.job, tc.runner); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	if trust.DefaultNetwork("owner") != "lan" || trust.DefaultNetwork("internal") != "lan" || trust.DefaultNetwork("collaborator") != "internet" || trust.DefaultNetwork("untrusted") != "restricted" {
		t.Fatal("network defaults")
	}
	release := trust.JobFacts{Trust: "owner", SecretClasses: []model.SecretClass{"release"}}
	if got := trust.SecretEligibility(release, trust.RunnerFacts{Isolation: "vm"}); len(got) != 0 {
		t.Fatalf("VM release secret rejected: %v", got)
	}
	if got := trust.SecretEligibility(release, trust.RunnerFacts{Isolation: "container"}); !includes(got, trust.TrustReleaseNeedsDedicated) {
		t.Fatalf("weak release: %v", got)
	}
}
func TestAdmissibleAllReasons(t *testing.T) {
	j := trust.JobFacts{Trust: "untrusted", SecretClasses: []model.SecretClass{"release"}, Network: "lan"}
	d := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "process", Network: "none", PrivacyZone: "private-infrastructure", Hosted: true, UIDSeparation: false, NoLANRoute: false, Running: trust.Running{SecretBearing: true}}
	rs := trust.Admissible(j, d, trust.Effective{})
	for _, want := range []trust.Reason{trust.TrustNotAccepted, trust.PrivacyHostedNotAuthorized, trust.TrustReleaseNeedsDedicated, trust.IsolationBelowMinimum, trust.IsolationUIDSeparation, trust.IsolationCotenancy, trust.NetworkScopeNotOffered, trust.NetworkLANForUntrusted, trust.SecretsUntrusted, trust.SecretsClassNotDeclared} {
		if !includes(rs, want) {
			t.Errorf("missing %s: %v", want, rs)
		}
	}
	owner := trust.JobFacts{Trust: "owner", PersonalOwnRun: true, SecretClasses: []model.SecretClass{"project"}, Network: "none"}
	d = trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "process", Network: "none", SecretClasses: []model.SecretClass{"project"}, Ownership: "operator", CoordinatorMode: trust.Personal, UIDSeparation: false, PersonalOwnNode: true}
	if rs := trust.Admissible(owner, d, trust.Effective{}); len(rs) != 0 {
		t.Fatalf("personal exception: %v", rs)
	}
	d.CoordinatorMode = trust.Team
	if !includes(trust.Admissible(owner, d, trust.Effective{}), trust.IsolationUIDSeparation) {
		t.Fatal("team without UID separation")
	}
	d.UIDSeparation = true
	d.Running.Untrusted = true
	if !includes(trust.Admissible(owner, d, trust.Effective{}), trust.IsolationCotenancy) {
		t.Fatal("secret cotenancy")
	}
	d.Hosted = true
	d.Isolation = "hosted-disposable"
	d.Running.Untrusted = false
	if !includes(trust.Admissible(owner, d, trust.Effective{}), trust.SecretsNotInPolicy) {
		t.Fatal("hosted secret admitted")
	}
	untrusted := trust.JobFacts{Trust: "untrusted", Network: "restricted"}
	hosted := trust.Declaration{Accepts: []model.TrustClass{"untrusted"}, Isolation: "hosted-disposable", Network: "restricted", PrivacyZone: "hosted-allowed", Hosted: true, UIDSeparation: true}
	if includes(trust.Admissible(untrusted, hosted, trust.Effective{}), trust.NetworkLANForUntrusted) {
		t.Fatal("hosted disposable needs no local LAN probe")
	}
}
func TestAdmissibleDefaultPolicy(t *testing.T) {
	r, err := trust.NewRegistry(
		trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"},
		trust.KeySpec{Key: "trust.network", Owner: "trust", Kind: trust.TightenLower, Order: []string{"none", "restricted", "internet", "lan", "privileged"}, Default: "privileged"},
		trust.KeySpec{Key: "trust.isolation_min", Owner: "trust", Kind: trust.TightenHigher, Order: []string{"process", "container", "sandboxed-container", "vm", "ephemeral-vm"}, Default: "process"},
		trust.KeySpec{Key: "trust.secrets", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"project"}},
		trust.KeySpec{Key: "trust.approval.first_time", Owner: "trust", Kind: trust.TightenOnlyOn, Default: false},
	)
	if err != nil {
		t.Fatal(err)
	}
	d := trust.Declaration{Accepts: []model.TrustClass{"owner", "untrusted"}, Isolation: "hosted-disposable", Network: "restricted", PrivacyZone: "hosted-allowed", Hosted: true, UIDSeparation: true, NoLANRoute: true}
	public, _, err := r.Merge("pin", trust.DefaultPolicy("public"))
	if err != nil {
		t.Fatal(err)
	}
	owner := trust.JobFacts{Trust: "owner", Network: "restricted"}
	fork := trust.JobFacts{Trust: "untrusted", Network: "restricted"}
	if !includes(trust.Admissible(owner, d, public), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("public default hosted owner")
	}
	if includes(trust.Admissible(fork, d, public), trust.PrivacyHostedNotAuthorized) {
		t.Fatal("public default denied fork")
	}
	for _, visibility := range []string{"private", "unknown"} {
		p, _, err := r.Merge("pin", trust.DefaultPolicy(visibility))
		if err != nil {
			t.Fatal(err)
		}
		if !includes(trust.Admissible(fork, d, p), trust.PrivacyHostedNotAuthorized) {
			t.Fatalf("%s default admitted hosted", visibility)
		}
	}
}
func TestDedicated(t *testing.T) {
	d := trust.Declaration{Ownership: "operator", Accepts: []model.TrustClass{"owner"}, Projects: 1, Dedicated: true}
	if !trust.IsDedicatedTrusted(d) {
		t.Fatal("dedicated")
	}
	for _, mutate := range []func(*trust.Declaration){func(x *trust.Declaration) { x.Source = "hello" }, func(x *trust.Declaration) { x.Source = "probe" }, func(x *trust.Declaration) { x.Shared = true }, func(x *trust.Declaration) { x.Projects = 2 }, func(x *trust.Declaration) { x.Ownership = "other" }, func(x *trust.Declaration) { x.Accepts = append(x.Accepts, "untrusted") }} {
		copy := d
		mutate(&copy)
		if trust.IsDedicatedTrusted(copy) {
			t.Fatalf("accepted %+v", copy)
		}
	}
	d.Dedicated = false
	d.PersonalOwnNode = true
	d.PersonalOwnRun = true
	d.ReleaseJob = true
	if !trust.IsDedicatedTrusted(d) {
		t.Fatal("own release")
	}
}
func TestMergeKinds(t *testing.T) {
	order := []string{"none", "restricted", "internet", "lan"}
	for _, tc := range []struct {
		kind     trust.Tightening
		old, new any
		ok       bool
	}{
		{trust.TightenLower, "lan", "none", true}, {trust.TightenLower, "none", "lan", false},
		{trust.TightenHigher, "none", "lan", true}, {trust.TightenHigher, "lan", "none", false},
		{trust.TightenSubset, []string{"a", "b"}, []string{"a"}, true}, {trust.TightenSubset, []string{"a"}, []string{"a", "b"}, false},
		{trust.TightenSuperset, []string{"a"}, []string{"a", "b"}, true}, {trust.TightenSuperset, []string{"a", "b"}, []string{"a"}, false},
		{trust.TightenOnlyOn, false, true, true}, {trust.TightenOnlyOn, true, false, false},
		{trust.TightenFixed, "a", "a", true}, {trust.TightenFixed, "a", "b", false},
	} {
		r, _ := trust.NewRegistry(trust.KeySpec{Key: "x", Owner: "test", Kind: tc.kind, Order: order})
		raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
		_, _, err := r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"x": raw(tc.old)}}, trust.Scope{Kind: trust.SourceJob, Values: map[string]json.RawMessage{"x": raw(tc.new)}})
		if (err == nil) != tc.ok {
			t.Errorf("kind %d old %v new %v err %v", tc.kind, tc.old, tc.new, err)
		}
	}
}
func TestMergeSpendAndRevision(t *testing.T) {
	r, _ := trust.NewRegistry(trust.KeySpec{Key: "economics.max", Owner: "sched", Kind: trust.TightenLower, Order: []string{"0", "1", "2"}, Spend: true, Default: "2"}, trust.KeySpec{Key: "trust.providers", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"a", "b"}})
	base := trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"economics.max": jsonString("1")}}
	e, d, err := r.Merge("pin", base)
	if err != nil {
		t.Fatal(err)
	}
	for _, scope := range []trust.Source{trust.SourceProtectedBranch, trust.SourceRevision} {
		changed := trust.Scope{Kind: scope, Values: map[string]json.RawMessage{"economics.max": jsonString("0")}}
		next, d2, err := r.Merge("pin", base, changed)
		if err != nil || d2 != d || len(next.Ignored()) != 1 {
			t.Fatalf("spend %s: %v %s %v", scope, err, d2, next.Ignored())
		}
	}
	_, _, err = r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"trust.providers": json.RawMessage(`["a"]`)}}, trust.Scope{Kind: trust.SourceJob, Values: map[string]json.RawMessage{"trust.providers": json.RawMessage(`["a","b"]`)}})
	if err == nil || !strings.Contains(err.Error(), "E670") || !strings.Contains(err.Error(), "project") || !strings.Contains(err.Error(), "job") {
		t.Fatalf("loosening error: %v", err)
	}
	_, _, err = r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"alien": json.RawMessage(`true`)}})
	if err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("unknown: %v", err)
	}
	_, _, err = r.Merge("pin", trust.Scope{Kind: trust.SourceProject, Values: map[string]json.RawMessage{"economics.max": json.RawMessage(`{`)}})
	if err == nil || !strings.Contains(err.Error(), "E672") {
		t.Fatalf("invalid: %v", err)
	}
	_ = e
}

func TestMergeRevisionFields(t *testing.T) {
	r, err := trust.NewRegistry(
		trust.KeySpec{Key: "trust.privacy_zone", Owner: "trust", Kind: trust.TightenLower, Order: []string{"local-only", "private-infrastructure", "provider-allowlist", "hosted-allowed"}, Default: "hosted-allowed"},
		trust.KeySpec{Key: "trust.network", Owner: "trust", Kind: trust.TightenLower, Order: []string{"none", "restricted", "internet", "lan", "privileged"}, Default: "lan"},
		trust.KeySpec{Key: "trust.providers", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"a", "b"}},
		trust.KeySpec{Key: "trust.secrets", Owner: "trust", Kind: trust.TightenSubset, Default: []string{"project", "deploy"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	branch := trust.Scope{Kind: trust.SourceProtectedBranch, Values: map[string]json.RawMessage{"trust.privacy_zone": jsonString("private-infrastructure"), "trust.network": jsonString("restricted"), "trust.providers": json.RawMessage(`["a"]`), "trust.secrets": json.RawMessage(`["project"]`)}}
	base, d0, err := r.Merge("pin")
	if err != nil {
		t.Fatal(err)
	}
	e, d, err := r.Merge("pin", branch)
	if err != nil || d == d0 {
		t.Fatalf("protected branch had no effect: %v", err)
	}
	for key := range branch.Values {
		before, _, _ := base.Value(key)
		after, source, _ := e.Value(key)
		if fmt.Sprint(before) == fmt.Sprint(after) || source != trust.SourceProtectedBranch {
			t.Fatalf("branch %s: %v -> %v (%s)", key, before, after, source)
		}
	}
	rev := trust.Scope{Kind: trust.SourceRevision, Values: map[string]json.RawMessage{"trust.privacy_zone": jsonString("hosted-allowed"), "trust.network": jsonString("lan"), "trust.providers": json.RawMessage(`["a","b"]`), "trust.secrets": json.RawMessage(`["project","deploy"]`)}}
	next, d2, err := r.Merge("pin", branch, rev)
	if err != nil || d2 != d || len(next.Ignored()) != 4 {
		t.Fatalf("revision widened: %v %s %v", err, d2, next.Ignored())
	}
	for _, ignored := range next.Ignored() {
		if ignored.Scope != trust.SourceRevision || !strings.Contains(ignored.Reason, "E671") {
			t.Fatalf("bad warning %+v", ignored)
		}
	}
}
func TestCanonical(t *testing.T) {
	want := []byte(`{"a":1,"b":{"c":"<>&","d":true}}`)
	if b, err := trust.Canonical(map[string]any{"b": map[string]any{"d": true, "c": "<>&"}, "a": 1}); err != nil || !bytes.Equal(b, want) {
		t.Fatalf("canonical %s %v", b, err)
	}
	unicode, err := trust.Canonical("a\u2028b\u2029c")
	if err != nil || !bytes.Equal(unicode, []byte("\"a\u2028b\u2029c\"")) {
		t.Fatalf("unnecessary unicode escape: %s %v", unicode, err)
	}
	for _, v := range []any{1.5, json.Number("1.5")} {
		if _, err := trust.Canonical(v); err == nil {
			t.Errorf("float accepted: %v", v)
		}
	}
	r := rand.New(rand.NewSource(71))
	for i := 0; i < 1000; i++ {
		keys := r.Perm(16)
		m := map[string]any{}
		for _, k := range keys {
			m[fmt.Sprintf("k%02d", k)] = k
		}
		b, err := trust.Canonical(m)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			want = b
		} else if !bytes.Equal(b, want) {
			t.Fatal("map order changed")
		}
	}
}
