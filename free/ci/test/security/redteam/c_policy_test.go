//go:build security

package redteam

// Purpose: red team group C, policy and spend (TM-31 policy widened by the PR, TM-35 privacy zone,
// TM-37 machine-invoked loosening, TM-38 spend abuse, TM-44 owner exception). The attacker plays each
// non-operator scope (revision, protected branch, pipeline, job, run) with widening values for every
// trust and spend key, plants a PR worktree whose ci.yaml widens the policy in a real git fixture,
// writes policy as a machine-invoked caller, and tries the I17 exception with one condition flipped.
// Inputs:  the production key registry, trust.Merge, trust.Admissible, policy.Resolver and Writer.
// Outputs: pass (the control held) or fail (the attack worked).
// Constraints: local git repositories in t.TempDir only; no network; deterministic.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	_ "github.com/nself-org/plugins/free/ci/internal/sched"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

func cRegistry(t *testing.T) trust.Registry {
	t.Helper()
	reg, err := trust.NewRegistry(policy.Default.Keys()...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func cScope(kind trust.Source, kv map[string]string) trust.Scope {
	vals := map[string]json.RawMessage{}
	for k, v := range kv {
		vals[k] = json.RawMessage(v)
	}
	return trust.Scope{Kind: kind, Values: vals}
}

// cWidening is one widening value per key (trust keys and spend keys).
var cWidening = map[string]string{
	"trust.isolation_min": `"process"`, "trust.network": `"privileged"`, "trust.secrets": `["project","release","deploy"]`,
	"trust.accepts": `["untrusted","collaborator","internal","owner"]`, "trust.machine_invoked_cap": `"owner"`, "trust.approval.first_time": `false`,
	"trust.privacy_zone": `"hosted-allowed"`, "trust.providers": `["github-hosted","evil"]`, "trust.self_reported_gates": `["release"]`,
	"economics.paid_enabled": `true`, "economics.budgets.day": `999999999`, "economics.budgets.provider.github": `999999999`, "economics.approval_threshold_micro": `999999999`,
	"economics.providers.github": `true`, "burst.providers.aws": `true`, "economics.max_paid_timeout": `99999999`, "notify.allow_private": `true`, "sched.overrides.min_isolation": `"process"`,
}

// TestC31AnyOtherScopeCannotWiden (TM-31, TM-38): the base is an operator project scope that sets the
// strict value of every trust key; a revision, protected-branch, pipeline, job or run scope that sets
// the loosest value must either be refused or leave the effective value as it was. Spend keys from any
// of them are never applied.
func TestC31AnyOtherScopeCannotWiden(t *testing.T) {
	reg := cRegistry(t)
	base := cScope(trust.SourceProject, map[string]string{"trust.isolation_min": `"vm"`, "trust.network": `"restricted"`, "trust.secrets": `[]`, "trust.accepts": `["owner","internal"]`,
		"trust.machine_invoked_cap": `"collaborator"`, "trust.approval.first_time": `true`, "trust.privacy_zone": `"private-infrastructure"`, "trust.providers": `["github-hosted"]`, "trust.self_reported_gates": `[]`,
		"economics.paid_enabled": `false`, "sched.overrides.min_isolation": `"vm"`})
	want, _, err := reg.Merge("", trust.DefaultPolicy("private"), base)
	if err != nil {
		t.Fatal(err)
	}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourceProtectedBranch, trust.SourcePipeline, trust.SourceJob, trust.SourceRun} {
		for key, val := range cWidening {
			got, _, err := reg.Merge("", trust.DefaultPolicy("private"), base, cScope(kind, map[string]string{key: val}))
			if err != nil {
				continue // a refused merge is a safe outcome
			}
			a, _, _ := want.Value(key)
			b, src, _ := got.Value(key)
			if !jsonEqual(a, b) {
				t.Errorf("scope %s set %s=%s: effective value %v (from %s), was %v", kind, key, val, b, src, a)
			}
			if flags := hostedFlag(got); flags {
				t.Errorf("scope %s set %s: the merge now authorizes hosted execution", kind, key)
			}
		}
	}
}

func jsonEqual(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

// hostedFlag reports whether the effective policy authorizes a hosted node for an owner job.
func hostedFlag(e trust.Effective) bool {
	hosted := trust.Declaration{Hosted: true, Accepts: []model.TrustClass{"owner"}, Isolation: "hosted-disposable", Network: "internet", UIDSeparation: true, PrivacyZone: "hosted-allowed"}
	for _, r := range trust.Admissible(trust.JobFacts{Trust: "owner", Network: "internet"}, hosted, e) {
		if r == trust.PrivacyHostedNotAuthorized {
			return false
		}
	}
	return true
}

// TestC35PrivacyZone (TM-35): a private repository, an unknown or respelled visibility, a revision
// that asks for hosted-allowed and a local-only zone each keep hosted nodes out; the public default
// authorizes hosted work for untrusted jobs only.
func TestC35PrivacyZone(t *testing.T) {
	reg := cRegistry(t)
	merge := func(vis string, extra ...trust.Scope) trust.Effective {
		e, _, err := reg.Merge("", append([]trust.Scope{trust.DefaultPolicy(vis)}, extra...)...)
		if err != nil {
			t.Fatalf("merge %q: %v", vis, err)
		}
		return e
	}
	hosted := trust.Declaration{Hosted: true, Provider: "github-hosted", Accepts: []model.TrustClass{"untrusted", "collaborator", "internal", "owner"}, Isolation: "hosted-disposable", Network: "restricted", UIDSeparation: true, NoLANRoute: true}
	blocked := func(e trust.Effective, class model.TrustClass) bool {
		for _, r := range trust.Admissible(trust.JobFacts{Trust: class, Network: "restricted"}, hosted, e) {
			if r == trust.PrivacyHostedNotAuthorized || r == trust.PrivacyLocalOnly || r == trust.PrivacyProviderNotAllowlisted {
				return true
			}
		}
		return false
	}
	if blocked(merge("public"), "untrusted") {
		t.Fatal("control: the public default refused hosted work for an untrusted job")
	}
	for _, vis := range []string{"private", "", "unknown", "Public", "PUBLIC", " public", "public ", "public\n", "internal", "publıc", "pub1ic"} {
		if !blocked(merge(vis), "untrusted") && !blocked(merge(vis), "owner") {
			t.Errorf("visibility %q authorized hosted work", vis)
		}
		if !blocked(merge(vis), "owner") {
			t.Errorf("visibility %q authorized hosted work for an owner job", vis)
		}
	}
	if !blocked(merge("public"), "owner") {
		t.Error("the public default authorized hosted work for an owner job")
	}
	for _, kind := range []trust.Source{trust.SourceRevision, trust.SourcePipeline, trust.SourceJob, trust.SourceRun} {
		e, _, err := reg.Merge("", trust.DefaultPolicy("private"), cScope(kind, map[string]string{"trust.privacy_zone": `"hosted-allowed"`}))
		if err == nil && !blocked(e, "owner") {
			t.Errorf("a %s scope authorized hosted work on a private repository", kind)
		}
	}
	local := merge("public", cScope(trust.SourceProject, map[string]string{"trust.privacy_zone": `"local-only"`}))
	lan := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "vm", Network: "lan", UIDSeparation: true}
	got := trust.Admissible(trust.JobFacts{Trust: "owner", Network: "lan"}, lan, local)
	found := false
	for _, r := range got {
		found = found || r == trust.PrivacyLocalOnly
	}
	if !found {
		t.Errorf("local-only work was admitted on a node that is not the operator's own: %v", got)
	}
}

// TestC44OwnerException (TM-44, I17): the non-separated-UID exception holds only with every condition;
// flipping any one of them brings the UID-separation reason back.
func TestC44OwnerException(t *testing.T) {
	good := func() (trust.JobFacts, trust.Declaration) {
		return trust.JobFacts{Trust: "owner", Network: "lan", PersonalOwnRun: true},
			trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "process", Network: "lan", Ownership: "operator", PersonalOwnNode: true, CoordinatorMode: trust.Personal, NoLANRoute: false}
	}
	has := func(rs []trust.Reason, want trust.Reason) bool {
		for _, r := range rs {
			if r == want {
				return true
			}
		}
		return false
	}
	j, d := good()
	if has(trust.Admissible(j, d, trust.Effective{}), trust.IsolationUIDSeparation) {
		t.Fatal("control: the exception did not apply to the honest case")
	}
	for name, bend := range map[string]func(*trust.JobFacts, *trust.Declaration){
		"team coordinator":     func(j *trust.JobFacts, d *trust.Declaration) { d.CoordinatorMode = trust.Team },
		"unknown mode":         func(j *trust.JobFacts, d *trust.Declaration) { d.CoordinatorMode = "Personal" },
		"empty mode":           func(j *trust.JobFacts, d *trust.Declaration) { d.CoordinatorMode = "" },
		"internal trust":       func(j *trust.JobFacts, d *trust.Declaration) { j.Trust = "internal" },
		"upper case owner":     func(j *trust.JobFacts, d *trust.Declaration) { j.Trust = "OWNER" },
		"empty trust":          func(j *trust.JobFacts, d *trust.Declaration) { j.Trust = "" },
		"team ownership":       func(j *trust.JobFacts, d *trust.Declaration) { d.Ownership = "team" },
		"empty ownership":      func(j *trust.JobFacts, d *trust.Declaration) { d.Ownership = "" },
		"not the operator run": func(j *trust.JobFacts, d *trust.Declaration) { j.PersonalOwnRun = false },
		"not the operator node": func(j *trust.JobFacts, d *trust.Declaration) {
			d.PersonalOwnNode = false
		},
		"container isolation": func(j *trust.JobFacts, d *trust.Declaration) { d.Isolation = "container" },
		"empty isolation":     func(j *trust.JobFacts, d *trust.Declaration) { d.Isolation = "" },
	} {
		j, d := good()
		bend(&j, &d)
		if !has(trust.Admissible(j, d, trust.Effective{}), trust.IsolationUIDSeparation) {
			t.Errorf("%s: the exception still applied", name)
		}
	}
	// the exception never carries secrets: a job with a secret class on a node that declares none is refused
	j, d = good()
	j.SecretClasses = []model.SecretClass{"release"}
	rs := trust.Admissible(j, d, trust.Effective{})
	if !has(rs, trust.SecretsClassNotDeclared) && !has(rs, trust.TrustReleaseNeedsDedicated) {
		t.Errorf("a release-secret job was admitted under the exception: %v", rs)
	}
}

func cMachineWriter(t *testing.T, machine bool) (*policy.Writer, roles.Actor) {
	t.Helper()
	r := cNewRig(t)
	return &policy.Writer{Store: r.st, Log: &audit.Log{St: r.st}, Machine: machine}, r.op()
}

// TestC37MachineInvokedCannotLoosen (TM-37): under NSELF_INVOKED_BY a write that raises the cap, unsets
// a trust key, loosens another trust key or pins is refused with the machine reason; spend keys are
// checked in the same way (a capped caller turning paid placement on is a loosening of policy).
func TestC37MachineInvokedCannotLoosen(t *testing.T) {
	ctx := context.Background()
	r := cNewRig(t)
	op := r.op()
	human := &policy.Writer{Store: r.st, Log: &audit.Log{St: r.st}}
	machine := &policy.Writer{Store: r.st, Log: &audit.Log{St: r.st}, Machine: true}
	set := func(w *policy.Writer, key, val string) error {
		_, err := w.Apply(ctx, op, policy.Change{Op: policy.OpSet, Scope: "user", Key: key, Value: json.RawMessage(val)})
		return err
	}
	if err := set(human, "trust.machine_invoked_cap", `"collaborator"`); err != nil {
		t.Fatalf("control: %v", err)
	}
	if err := set(human, "trust.isolation_min", `"vm"`); err != nil {
		t.Fatalf("control: %v", err)
	}
	for name, err := range map[string]error{
		"enable paid spend":        set(machine, "economics.paid_enabled", `true`),
		"raise daily budget":       set(machine, "economics.budgets.day", `1000000000`),
		"raise approval threshold": set(machine, "economics.approval_threshold_micro", `1000000000`),
		"raise the cap":            set(machine, "trust.machine_invoked_cap", `"owner"`),
		"raise the cap, same":      set(machine, "trust.machine_invoked_cap", `"internal"`),
		"lower isolation":          set(machine, "trust.isolation_min", `"process"`),
		"widen secrets":            set(machine, "trust.secrets", `["project","environment","team","release","deploy"]`),
		"first-time approval":      set(machine, "trust.approval.first_time", `false`),
		"unset the cap": func() error {
			_, e := machine.Apply(ctx, op, policy.Change{Op: policy.OpUnset, Scope: "user", Key: "trust.machine_invoked_cap"})
			return e
		}(),
		"pin": func() error {
			_, e := machine.Apply(ctx, op, policy.Change{Op: policy.OpPin, Project: "p", SHA: strings.Repeat("a", 40), Root: t.TempDir()})
			return e
		}(),
		"clear pin": func() error {
			_, e := machine.Apply(ctx, op, policy.Change{Op: policy.OpClear, Project: "p"})
			return e
		}(),
	} {
		if err == nil || roles.ReasonOf(err) != policy.ReasonMachineInvoked && !strings.HasPrefix(err.Error(), "E670") {
			t.Errorf("%s: machine-invoked write not refused (err: %v)", name, err)
		}
	}
}

// TestC38MachineInvokedCannotEnableSpend (TM-38, C05): the machine-invoked refusal covers trust.* only;
// a capped caller can set paid placement and budgets, which loosen policy the same way.
func TestC38MachineInvokedCannotEnableSpend(t *testing.T) {
	ctx := context.Background()
	w, op := cMachineWriter(t, true)
	for key, val := range map[string]string{"economics.paid_enabled": `true`, "economics.budgets.day": `1000000000`, "economics.approval_threshold_micro": `1000000000`} {
		if _, err := w.Apply(ctx, op, policy.Change{Op: policy.OpSet, Scope: "user", Key: key, Value: json.RawMessage(val)}); err == nil {
			t.Errorf("a machine-invoked caller set %s=%s", key, val)
		}
	}
}

// TestC38SpendValuesAreBounded (TM-38): negative, overflowing, fractional and non-numeric budgets are
// refused at the writer, so no negative reservation or wrapped ceiling reaches admission.
func TestC38SpendValuesAreBounded(t *testing.T) {
	ctx := context.Background()
	w, op := cMachineWriter(t, false)
	for _, key := range []string{"economics.budgets.day", "economics.budgets.job", "economics.approval_threshold_micro", "economics.max_paid_timeout", "economics.margin_micro"} {
		for _, val := range []string{`-1`, `-9223372036854775808`, `9223372036854775808`, `1e3`, `0.5`, `"12"`, `null`, `true`, `[]`, `{}`, `"NaN"`, `1e400`} {
			if _, err := w.Apply(ctx, op, policy.Change{Op: policy.OpSet, Scope: "user", Key: key, Value: json.RawMessage(val)}); err == nil {
				t.Errorf("%s=%s was stored", key, val)
			}
		}
	}
}
