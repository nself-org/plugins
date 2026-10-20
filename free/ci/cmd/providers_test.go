package main

// Purpose: tests of `nself ci providers authorize|revoke|list|show` (P7-HOST-12): authorization is
// admin-only and audited through the P7-TRUST-26 writer, revoke makes the next admission explain
// privacy.provider_not_allowlisted, a revision's .nself/ci.yaml cannot authorize anything, --allow-private
// writes the zone first and prints (never runs) the SCHED ceiling command, and list/show read without
// writing. The online `test` command is in providers_probe_test.go.
// Inputs:  the policy rig of policy_test.go (a real SQLite store and a git clone), recording writers.
// Outputs: pass or fail; no network. Fake values only.
// Constraints: each test named by the Ticket's verify line asserts PASS and never skips.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	cidoc "github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	pdoctor "github.com/nself-org/plugins/free/ci/internal/providers/doctor"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/roles"
	"github.com/nself-org/plugins/free/ci/internal/store"
	"github.com/nself-org/plugins/free/ci/internal/trust"
)

// providersEnvFor builds the environment of the providers commands over the policy rig; vars is the
// process environment the commands read.
func providersEnvFor(r *policyRig, vars map[string]string) providersEnv {
	return providersEnv{policyEnv: r.env(), Getenv: func(k string) string { return vars[k] }, Now: r.now,
		OpenStore: func() (*store.Store, error) { return store.Open(r.path, store.Options{Clock: r.now}) }}
}

// canonProject is the project-scope subject of a directory, as the commands compute it.
func canonProject(dir string) string {
	return policy.ProjectIDGit(context.Background(), policy.Runner(report.Capture), dir)
}

func (r *policyRig) prov(op string, args ...string) int {
	return runProviders(op, args, providersEnvFor(r, nil))
}

// projectRow is the stored JSON of key at project scope for subject ("" when there is no row).
func projectRow(t *testing.T, r *policyRig, subject, key string) string {
	t.Helper()
	rows, err := r.store().PolicyRows(context.Background(), "project", subject)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.Key == key {
			return string(row.Value)
		}
	}
	return ""
}

// reasonsFor resolves the policy of project as a public repository and asks admission whether a hosted
// runner of provider is admissible for an owner job.
func reasonsFor(t *testing.T, r *policyRig, project, provider string) []trust.Reason {
	t.Helper()
	res := policy.Resolver{Store: r.store(), Run: policy.Runner(report.Capture)}
	got, err := res.Resolve(context.Background(), policy.Repo{Root: r.work, Project: project, Visibility: "public", Class: "owner"}, policy.Revision{Dir: r.work})
	if err != nil {
		t.Fatal(err)
	}
	d := trust.Declaration{Accepts: []model.TrustClass{"owner"}, Isolation: "hosted-disposable", Network: "restricted", PrivacyZone: "hosted-allowed",
		Hosted: true, Provider: provider, UIDSeparation: true}
	return trust.Admissible(trust.JobFacts{Trust: "owner", Network: "restricted"}, d, got.Effective)
}

func has(rs []trust.Reason, want trust.Reason) bool {
	for _, r := range rs {
		if r == want {
			return true
		}
	}
	return false
}

// TestAuthorizeRoles proves acceptance line 1: `providers authorize github-hosted --project p` as a
// non-admin team member is refused by TRUST-26 (E687) and writes nothing; as admin it writes
// trust.providers at project scope with one audit row; `revoke` makes the next admission explain
// privacy.provider_not_allowlisted; a revision's .nself/ci.yaml claiming the authorization changes nothing.
func TestAuthorizeRoles(t *testing.T) {
	r := newPolicyRig(t, "")
	pDir, p2Dir, emptyDir := t.TempDir(), t.TempDir(), t.TempDir()
	p, p2 := canonProject(pDir), canonProject(p2Dir)
	r.token = r.memberToken()
	if code := r.prov("authorize", "github-hosted", "--project", pDir, "--json"); code != 1 || !strings.HasPrefix(r.errCode(), "E687") {
		t.Fatalf("a member must be refused with E687, exit %d: %s", code, r.out.String())
	}
	if projectRow(t, r, p, "trust.providers") != "" || r.auditCount("policy.set") != 0 {
		t.Fatal("a refused authorization wrote a row or an audit row")
	}
	r.token = ""
	if code := r.prov("authorize", "github-hosted", "--project", pDir, "--json"); code != 0 {
		t.Fatalf("admin authorize exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	if got := projectRow(t, r, p, "trust.providers"); got != `["github-hosted"]` || r.auditCount("policy.set") != 1 {
		t.Fatalf("one project row and one audit row expected: %q, %d audit rows", got, r.auditCount("policy.set"))
	}
	if d := r.data(); d["op"] != "authorize" || d["provider"] != "github-hosted" || d["project"] != p {
		t.Fatalf("data: %v", d)
	}
	if code := r.prov("authorize", "gitlab", "--project", pDir); code != 0 || projectRow(t, r, p, "trust.providers") != `["github-hosted","gitlab"]` {
		t.Fatalf("a second provider is added in order: exit %d %q", code, projectRow(t, r, p, "trust.providers"))
	}
	for name, args := range map[string][]string{
		"no project":       {"authorize", "github-hosted"},
		"unknown provider": {"authorize", "nope", "--project", p},
		"no provider":      {"authorize", "--project", p},
	} {
		if code := r.prov(args[0], args[1:]...); code != 1 {
			t.Fatalf("%s: exit %d, want 1", name, code)
		}
	}
	if projectRow(t, r, p, "trust.providers") != `["github-hosted","gitlab"]` {
		t.Fatal("a refused command changed the row")
	}
	for _, bad := range []string{"proj-1", "has\nnewline", "x/../y", strings.Repeat("a", 2000), ""} {
		if code := r.prov("authorize", "github-hosted", "--project", bad); code != 1 {
			t.Fatalf("project %.20q must be refused, exit %d", bad, code)
		}
	}
	// A revoke with nothing to remove writes nothing (an empty row would also drop outer scopes' providers);
	// a non-admin is still refused.
	audits := r.auditCount("policy.set")
	if code := r.prov("revoke", "github-hosted", "--project", emptyDir, "--json"); code != 0 || r.data()["changed"] != false || projectRow(t, r, canonProject(emptyDir), "trust.providers") != "" || r.auditCount("policy.set") != audits {
		t.Fatalf("a no-op revoke must write nothing: exit %d: %s", code, r.out.String())
	}
	r.token = r.memberToken()
	if code := r.prov("revoke", "github-hosted", "--project", emptyDir, "--json"); code != 1 || !strings.HasPrefix(r.errCode(), "E687") {
		t.Fatalf("a member's no-op revoke must still be refused, exit %d", code)
	}
	r.token = ""
	r.token = r.memberToken()
	if code := r.prov("revoke", "github-hosted", "--project", pDir); code != 1 || projectRow(t, r, p, "trust.providers") != `["github-hosted","gitlab"]` {
		t.Fatalf("a member must not revoke: exit %d", code)
	}
	r.token = ""
	// Admission needs trust.privacy_zone provider-allowlist. The writer refuses that row today (see
	// TestAllowPrivate), and a row stored below the writer makes every later write fail validation, so
	// it is added last, and a second project shows the authorized side.
	zone := func(project string) {
		t.Helper()
		err := r.store().ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: "trust.privacy_zone", Value: json.RawMessage(`"provider-allowlist"`), Actor: "test"}, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(pDir, link); err != nil {
		t.Fatal(err)
	}
	if code := r.prov("revoke", "github-hosted", "--project", link, "--json"); code != 0 || projectRow(t, r, p, "trust.providers") != `["gitlab"]` {
		t.Fatalf("revoke: exit %d, row %q", code, projectRow(t, r, p, "trust.providers"))
	}
	zone(p)
	if !has(reasonsFor(t, r, p, "github-hosted"), trust.PrivacyProviderNotAllowlisted) {
		t.Fatal("after revoke the next snapshot must explain privacy.provider_not_allowlisted")
	}
	if has(reasonsFor(t, r, p, "gitlab"), trust.PrivacyProviderNotAllowlisted) {
		t.Fatal("revoking one provider must leave the others")
	}
	if code := r.prov("authorize", "github-hosted", "--project", p2Dir); code != 0 {
		t.Fatalf("authorize exited %d", code)
	}
	zone(p2)
	if has(reasonsFor(t, r, p2, "github-hosted"), trust.PrivacyProviderNotAllowlisted) {
		t.Fatal("an authorized provider must be admissible")
	}
	if n := r.auditCount("policy.set"); n != 4 {
		t.Fatalf("authorize, authorize, revoke and authorize are audited once each, got %d rows", n)
	}
	// The project is a directory: a linked worktree maps to its main worktree's id, ~/x and ./x are paths,
	// and a failing git is refused (no row admission would read).
	wt := filepath.Join(t.TempDir(), "wt")
	policyGit(t, r.work, "worktree", "add", "-q", "--detach", wt)
	if code := r.prov("authorize", "gitlab", "--project", wt); code != 0 || !strings.Contains(projectRow(t, r, r.project(), "trust.providers"), "gitlab") {
		t.Fatalf("a linked worktree must key the main worktree's row: exit %d, %q", code, projectRow(t, r, r.project(), "trust.providers"))
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := r.prov("authorize", "gitlab", "--project", "~/proj"); code != 0 || projectRow(t, r, canonProject(filepath.Join(home, "proj")), "trust.providers") != `["gitlab"]` {
		t.Fatalf("~/proj: exit %d", code)
	}
	t.Chdir(home)
	if code := r.prov("authorize", "gitlab", "--project", "./proj"); code != 0 {
		t.Fatalf("./proj: exit %d", code)
	}
	if code := r.prov("authorize", "gitlab", "--project", "a\x00b"); code != 1 {
		t.Fatalf("a NUL byte must be refused, exit %d", code)
	}
	noGit := providersEnvFor(r, nil)
	noGit.Run = func(context.Context, ...string) (string, int, error) { return "", 0, os.ErrNotExist }
	if code := runProviders("authorize", []string{"gitlab", "--project", pDir}, noGit); code != 1 || !strings.Contains(r.errb.String(), "git could not be run") {
		t.Fatalf("a failing git must refuse the write, exit %d: %s", code, r.errb.String())
	}
	// A revision claiming the authorization changes nothing.
	own := r.project()
	for _, args := range [][]string{{"authorize", "github-hosted", "--project", own}, {"revoke", "github-hosted", "--project", own}} {
		if code := r.prov(args[0], args[1:]...); code != 0 {
			t.Fatalf("%v exited %d", args, code)
		}
	}
	policyGit(t, r.work, "checkout", "-q", "-b", "claim")
	claim := "version: 1\npolicy:\n  trust:\n    providers: [github-hosted]\n"
	if err := os.WriteFile(filepath.Join(r.work, ".nself/ci.yaml"), []byte(claim), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := pdoctor.Load(context.Background(), r.work, report.Capture, r.store())
	if cfg.Err != nil || slices.Contains(cfg.Authorized, "github-hosted") {
		t.Fatalf("a revision's claim must not authorize: err %v, authorized %v", cfg.Err, cfg.Authorized)
	}
}

// TestAllowPrivate proves acceptance line 2: `--allow-private` also writes trust.privacy_zone
// provider-allowlist (first, so a failure between the two writes admits nothing) and prints, without
// running, the `ci policy set --scope project economics.ceilings.github-hosted <n>` command. Against the
// real writer a project row loosens a private repository's baseline zone (D-1382): both rows land, in
// order, and the effective zone becomes provider-allowlist.
func TestAllowPrivate(t *testing.T) {
	r := newPolicyRig(t, "")
	pDir := t.TempDir()
	p := canonProject(pDir)
	var calls []policy.Change
	env := providersEnvFor(r, nil)
	env.Apply = func(_ context.Context, _ roles.Actor, c policy.Change) (policy.Result, error) {
		calls = append(calls, c)
		return policy.Result{}, nil
	}
	if code := runProviders("authorize", []string{"github-hosted", "--project", pDir, "--allow-private"}, env); code != 0 {
		t.Fatalf("exit %d: %s %s", code, r.out.String(), r.errb.String())
	}
	if len(calls) != 2 || calls[0].Key != "trust.privacy_zone" || string(calls[0].Value) != `"provider-allowlist"` ||
		calls[1].Key != "trust.providers" || string(calls[1].Value) != `["github-hosted"]` {
		t.Fatalf("the zone is written first, then the allowlist: %+v", calls)
	}
	for _, c := range calls {
		if c.Scope != "project" || c.Project != p || strings.HasPrefix(c.Key, "economics.") {
			t.Fatalf("every write is project scoped and none sets a spend key: %+v", c)
		}
	}
	out := r.out.String()
	if !strings.Contains(out, "nself ci policy set --scope project economics.ceilings.github-hosted <micro-currency-units>") || !strings.Contains(out, "not run for you") {
		t.Fatalf("the ceiling command must be printed: %s", out)
	}
	calls = nil
	env = providersEnvFor(r, nil)
	env.Apply = func(_ context.Context, _ roles.Actor, c policy.Change) (policy.Result, error) {
		calls = append(calls, c)
		return policy.Result{}, nil
	}
	if code := runProviders("authorize", []string{"gitlab", "--project", pDir, "--json"}, env); code != 0 || len(calls) != 1 || calls[0].Key != "trust.providers" {
		t.Fatalf("without the flag only the allowlist is written: exit %d, %+v", code, calls)
	}
	if d := r.data(); d["ceiling_command"] != nil || d["privacy_zone"] != nil {
		t.Fatalf("no zone and no ceiling hint without the flag: %v", d)
	}
	// The real writer.
	if code := r.prov("authorize", "github-hosted", "--project", pDir, "--allow-private", "--json"); code != 0 {
		t.Fatalf("exit %d: %s %s", code, r.out.String(), r.errb.String())
	}
	if got := projectRow(t, r, p, "trust.privacy_zone"); got != `"provider-allowlist"` {
		t.Fatalf("zone row %q", got)
	}
	if got := projectRow(t, r, p, "trust.providers"); got != `["github-hosted"]` {
		t.Fatalf("providers row %q", got)
	}
	if d := r.data(); d["effective_zone"] != "provider-allowlist" {
		t.Fatalf("effective zone: %v", d)
	}
}

// TestProvidersListShow proves the read commands: list names every provider with its health, credential
// source and authorization and writes nothing; show prints capabilities, health, authorization and the
// stored facts of one provider, and fails for an unknown one.
func TestProvidersListShow(t *testing.T) {
	r := newPolicyRig(t, "")
	if code := r.do("set", "--scope", "user", "providers.github.token_env", "NSELF_TEST_GH"); code != 0 {
		t.Fatalf("set exited %d: %s", code, r.errb.String())
	}
	if code := r.prov("authorize", "gitlab", "--project", r.project()); code != 0 {
		t.Fatalf("authorize exited %d", code)
	}
	vars := map[string]string{"NSELF_TEST_GH": "github_pat_PLANTED0001"}
	audit := r.auditCount("policy.set")
	if code := runProviders("list", []string{"--json"}, providersEnvFor(r, vars)); code != 0 {
		t.Fatalf("list exited %d: %s", code, r.errb.String())
	}
	rows := map[string]map[string]any{}
	for _, p := range r.data()["providers"].([]any) {
		m := p.(map[string]any)
		rows[m["id"].(string)] = m
	}
	for _, id := range []string{"github-hosted", "gitlab"} {
		if rows[id] == nil || rows[id]["health"].(map[string]any)["state"] != "up" {
			t.Fatalf("provider %s missing or not up: %v", id, rows)
		}
	}
	cred := rows["github-hosted"]["credential"].(map[string]any)
	if cred["source"] != "env:NSELF_TEST_GH" || cred["present"] != true || cred["kind"] != "fine-grained" || rows["github-hosted"]["authorized"] != false || rows["gitlab"]["authorized"] != true {
		t.Fatalf("list rows: %v", rows)
	}
	if r.auditCount("policy.set") != audit {
		t.Fatal("list wrote an audit row")
	}
	if code := runProviders("list", nil, providersEnvFor(r, vars)); code != 0 || !strings.Contains(r.out.String(), "github-hosted") {
		t.Fatalf("text list: exit %d: %s", code, r.out.String())
	}
	// show: a stored fact appears; with no repository Describe sends nothing and finds no capability.
	fact := store.ProviderFact{Provider: "gitlab", Scope: "health", Key: "note", Value: json.RawMessage(`{"seen":true}`), ObservedAt: time.Unix(1_800_000_000, 0)}
	if _, err := r.store().ProviderFactPut(context.Background(), fact); err != nil {
		t.Fatal(err)
	}
	if code := runProviders("show", []string{"gitlab", "--json"}, providersEnvFor(r, vars)); code != 0 {
		t.Fatalf("show exited %d: %s %s", code, r.out.String(), r.errb.String())
	}
	d := r.data()
	facts := d["facts"].([]any)
	if len(d["capabilities"].([]any)) != 0 || len(facts) != 1 || facts[0].(map[string]any)["key"] != "note" || d["economics_note"] == nil ||
		d["authorization"].(map[string]any)["authorized"] != true {
		t.Fatalf("show data: %v", d)
	}
	if code := runProviders("show", []string{"gitlab"}, providersEnvFor(r, vars)); code != 0 || !strings.Contains(r.out.String(), "fact          health/note") {
		t.Fatalf("text show: exit %d: %s", code, r.out.String())
	}
	var c model.Capability
	c.Identity.ID = "x:one"
	if s := capSummary(c); s.ID != "x:one" || s.OS != "unknown" || s.State != "unknown" {
		t.Fatalf("a capability with unknown facts reads as unknown: %+v", s)
	}
	if code := runProviders("show", []string{"nope"}, providersEnvFor(r, vars)); code != 1 {
		t.Fatalf("show of an unknown provider exited %d", code)
	}
}

// TestProvidersHelpAndDoctorSelector proves the surface half of acceptance line 3: the providers help has
// no doctor subcommand and points at `ci doctor --providers`, and that selector reaches the section (CI-DP
// ids in the JSON report, exit code one of the three the doctor defines).
func TestProvidersHelpAndDoctorSelector(t *testing.T) {
	if strings.Contains(providersHelp[:strings.Index(providersHelp, "Provider checks")], "doctor") {
		t.Fatal("the command list must not name a doctor subcommand")
	}
	if !strings.Contains(providersHelp, "nself ci doctor --providers") {
		t.Fatal("the help must point at the doctor")
	}
	for k := range v15Subcommands {
		if strings.HasPrefix(k, "providers ") && strings.Contains(k, "doctor") {
			t.Fatalf("a doctor subcommand is registered: %s", k)
		}
	}
	r := newPolicyRig(t, "")
	t.Setenv("NSELF_CI_HOME", filepath.Dir(r.path))
	var out, errb strings.Builder
	code := cidoc.Run([]string{"--providers", "--json", "--repo", r.work}, &out, &errb, cidoc.DefaultDeps())
	if code != cidoc.ExitClean && code != cidoc.ExitFailed && code != cidoc.ExitWarnings {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	var env struct {
		Data struct {
			Checks []cidoc.Check `json:"checks"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out.String()), &env); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, c := range env.Data.Checks {
		if strings.HasPrefix(c.ID, "CI-DP") {
			n++
			if c.Status != cidoc.Pass && c.Remediation == "" {
				t.Fatalf("%s lacks a remediation", c.ID)
			}
		}
	}
	if n < 10 {
		t.Fatalf("only %d CI-DP checks in the report", n)
	}
}
