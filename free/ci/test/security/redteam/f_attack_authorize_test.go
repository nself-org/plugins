//go:build security

package redteam

// Purpose: TM-35 red team of `nself ci providers authorize|revoke` (P7-HOST-12). The real binary decides which
// projects may use a hosted provider. Attacks: a machine-invoked caller (NSELF_INVOKED_BY), spellings of the
// project path, a revoke that does not take effect (concurrent change), a private or local-only project, a
// planted .nself/ci.yaml, and role-scoped tokens at the policy writer.
// Inputs:  the nself-ci binary built from ./cmd, a temp CI home and git repo (see f_attack_helpers_test.go).
// Outputs: pass = the control held; fail = the attack worked.
// Constraints: local fixtures only; the operator identity is the local user, as in production.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/audit"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/roles"
)

// TestF01MachineInvokedCannotAuthorize (TM-35, TM-37): with NSELF_INVOKED_BY set, `authorize` (which adds
// to trust.providers, a loosening) and `authorize --allow-private` are refused and write nothing.
func TestF01MachineInvokedCannotAuthorize(t *testing.T) {
	b := fNew(t)
	machine := []string{"NSELF_INVOKED_BY=mcp"}
	if _, errOut, exit := b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo); exit != 0 {
		t.Fatalf("control: the human authorize failed: %s", errOut)
	}
	for _, args := range [][]string{
		{"providers", "authorize", "gitlab", "--project", b.repo},
		{"providers", "authorize", "github", "--project", b.repo, "--allow-private"},
		{"providers", "authorize", "gitlab", "--project", b.repo + "/"},
	} {
		if _, _, exit := b.run(machine, args...); exit == 0 {
			t.Errorf("machine-invoked %v exited 0", args)
		}
	}
	list, zone := b.authorized(b.repo)
	if len(list) != 1 || list[0] != "github-hosted" || zone == "provider-allowlist" || zone == "hosted-allowed" {
		t.Errorf("a machine-invoked caller changed the authorization: %v zone %s", list, zone)
	}
	// the other direction is tightening and stays allowed
	if _, errOut, exit := b.run(machine, "providers", "revoke", "github-hosted", "--project", b.repo); exit != 0 {
		t.Errorf("control: a machine-invoked revoke (tightening) should work: %s", errOut)
	}
}

// TestF02ProjectSpellings (TM-35): authorize and revoke reach one row however the project path is spelled
// (trailing slash, dot segments, symlink, `..`); a spelling that names another path never changes this project.
func TestF02ProjectSpellings(t *testing.T) {
	b := fNew(t)
	link := filepath.Join(filepath.Dir(b.repo), "link")
	if err := os.Symlink(b.repo, link); err != nil {
		t.Fatal(err)
	}
	same := []string{b.repo, b.repo + "/", b.repo + "/./", filepath.Join(b.repo, "..", filepath.Base(b.repo)), link, link + "/"}
	for i, authSpelling := range same {
		revSpelling := same[(i+3)%len(same)]
		if _, errOut, exit := b.run(nil, "providers", "authorize", "gitlab", "--project", authSpelling); exit != 0 {
			t.Fatalf("authorize via %q: %s", authSpelling, errOut)
		}
		if list, _ := b.authorized(b.repo); !fHas(list, "gitlab") {
			t.Fatalf("authorize via %q did not reach the project row: %v", authSpelling, list)
		}
		if _, errOut, exit := b.run(nil, "providers", "revoke", "gitlab", "--project", revSpelling); exit != 0 {
			t.Fatalf("revoke via %q: %s", revSpelling, errOut)
		}
		if list, _ := b.authorized(b.repo); fHas(list, "gitlab") {
			t.Errorf("authorized via %q, revoked via %q: still authorized %v", authSpelling, revSpelling, list)
		}
	}
	other := filepath.Join(filepath.Dir(b.repo), "other")
	_ = os.MkdirAll(other, 0o755)
	for _, p := range []string{b.repo + " ", b.repo + "\t", b.repo + "\n", " " + b.repo, b.repo + "​", strings.ToUpper(b.repo) + "X", other} {
		b.run(nil, "providers", "authorize", "gitlab", "--project", p)
		if list, _ := b.authorized(b.repo); fHas(list, "gitlab") {
			t.Errorf("authorize with --project %q authorized this project: %v", p, list)
			b.run(nil, "providers", "revoke", "gitlab", "--project", b.repo)
		}
	}
}

// TestF03RevokeSurvivesAConcurrentAuthorize (TM-35): a revoke that exits 0 must leave the provider revoked,
// even when another authorize of the same project runs at the same moment (read-modify-write, debt D-1386).
func TestF03RevokeSurvivesAConcurrentAuthorize(t *testing.T) {
	if !openFinding(t, "P01") {
		return
	}
	b := fNew(t)
	for round := 0; round < 30; round++ {
		b.run(nil, "providers", "revoke", "gitlab", "--project", b.repo)
		b.run(nil, "providers", "revoke", "github-hosted", "--project", b.repo)
		b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo)
		done := make(chan int, 2)
		go func() { _, _, e := b.run(nil, "providers", "revoke", "github-hosted", "--project", b.repo); done <- e }()
		go func() { _, _, e := b.run(nil, "providers", "authorize", "gitlab", "--project", b.repo); done <- e }()
		e1, e2 := <-done, <-done
		list, _ := b.authorized(b.repo)
		if e1 == 0 && e2 == 0 && fHas(list, "github-hosted") {
			t.Fatalf("round %d: `revoke github-hosted` exited 0 but the provider is still authorized: %v", round, list)
		}
	}
}

// TestF04PrivateAndLocalOnlyProjects (TM-35): --allow-private is refused for a private baseline and for a
// local-only operator zone, writes nothing, and a plain authorize leaves the effective zone private.
func TestF04PrivateAndLocalOnlyProjects(t *testing.T) {
	b := fNew(t)
	if _, _, exit := b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo, "--allow-private"); exit == 0 {
		t.Error("--allow-private on a private-baseline project exited 0")
	}
	if _, zone := b.authorized(b.repo); zone == "provider-allowlist" || zone == "hosted-allowed" {
		t.Errorf("the zone of a private project was loosened: %s", zone)
	}
	if _, _, exit := b.run(nil, "providers", "authorize", "github-hosted", "--project", b.repo); exit != 0 {
		t.Fatal("control: plain authorize failed")
	}
	if _, zone := b.authorized(b.repo); zone != "private-infrastructure" {
		t.Errorf("authorize without --allow-private changed the zone to %s", zone)
	}
	for _, zone := range []string{"local-only"} {
		if _, errOut, exit := b.run(nil, "policy", "set", "--scope", "user", "trust.privacy_zone", zone); exit != 0 {
			t.Fatalf("control: policy set zone %s: %s", zone, errOut)
		}
		if _, _, exit := b.run(nil, "providers", "authorize", "gitlab", "--project", b.repo, "--allow-private"); exit == 0 {
			t.Errorf("--allow-private under a %s operator zone exited 0", zone)
		}
		if _, got := b.authorized(b.repo); got != zone {
			t.Errorf("zone %s became %s", zone, got)
		}
	}
}

// TestF05PlantedRevisionPolicy (TM-35, TM-33): a checkout whose .nself/ci.yaml authorizes a provider, sets
// the zone to hosted-allowed or points the provider API and token variable elsewhere changes nothing the
// providers commands report.
func TestF05PlantedRevisionPolicy(t *testing.T) {
	b := fNew(t)
	plant := "version: 1\npolicy:\n  providers:\n    github:\n      api_url: https://attacker.example\n      token_env: AWS_SECRET_ACCESS_KEY\n    forgejo:\n      url: http://attacker.example\n  trust:\n    providers: [github, github-hosted, gitlab]\n    privacy_zone: hosted-allowed\n"
	if err := os.MkdirAll(filepath.Join(b.repo, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.repo, ".nself", "ci.yaml"), []byte(plant), 0o644); err != nil {
		t.Fatal(err)
	}
	list, zone := b.authorized(b.repo)
	if len(list) != 0 || zone == "hosted-allowed" || zone == "provider-allowlist" {
		t.Errorf("a planted revision policy changed the authorization: %v zone %s", list, zone)
	}
	out, _, _ := b.run(nil, "providers", "show", "github", "--json")
	if strings.Contains(out, "AWS_SECRET_ACCESS_KEY") || strings.Contains(out, "attacker.example") {
		t.Errorf("a planted revision policy reached the credential source or API host:\n%s", out)
	}
}

// TestF06MachineInvokedCannotRepointProviders (TM-33, TM-43): providers.*.api_url, token_env and the
// forgejo URL decide where a token is sent and which variable is read. Setting them as a machine-invoked caller
// (a prompt-injected agent) must be refused; today only trust.* keys are checked (the root cause of C05).
func TestF06MachineInvokedCannotRepointProviders(t *testing.T) {
	if !openFinding(t, "P02") {
		return
	}
	b := fNew(t)
	machine := []string{"NSELF_INVOKED_BY=mcp"}
	for _, kv := range [][2]string{{"providers.github.api_url", `"https://attacker.example"`}, {"providers.github.token_env", `"AWS_SECRET_ACCESS_KEY"`},
		{"providers.gitlab.api_url", `"https://attacker.example/api/v4"`}, {"providers.forgejo.url", `"http://attacker.example"`}} {
		if _, _, exit := b.run(machine, "policy", "set", "--scope", "user", kv[0], kv[1]); exit == 0 {
			t.Errorf("a machine-invoked caller set %s=%s", kv[0], kv[1])
		}
	}
}

// TestF07ScopedActorsAtTheWriter (TM-35): the policy writer behind authorize refuses a member, an admin of
// another project and a project-scoped admin writing user or team scope.
func TestF07ScopedActorsAtTheWriter(t *testing.T) {
	ctx := context.Background()
	r := cNewRig(t)
	w := &policy.Writer{Store: r.st, Log: &audit.Log{St: r.st}}
	tok := func(role roles.Role, projects ...string) roles.Actor {
		raw, _, err := r.svc.IssueToken(ctx, r.op(), roles.Request{Principal: "carol", Role: role, Projects: projects})
		if err != nil {
			t.Fatal(err)
		}
		a, err := r.svc.ActorFromToken(ctx, raw)
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	set := func(a roles.Actor, scope, project string) error {
		_, err := w.Apply(ctx, a, policy.Change{Op: policy.OpSet, Scope: scope, Project: project, Team: "t1", Key: "trust.providers", Value: json.RawMessage(`["github-hosted"]`)})
		return err
	}
	if err := set(tok(roles.Admin, "/p/a"), "project", "/p/a"); err != nil {
		t.Fatalf("control: a project admin writing its own project: %v", err)
	}
	cases := map[string]error{
		"member on its own project":  set(tok(roles.Member, "/p/a"), "project", "/p/a"),
		"admin of another project":   set(tok(roles.Admin, "/p/a"), "project", "/p/b"),
		"admin of a path prefix":     set(tok(roles.Admin, "/p/a"), "project", "/p/a/sub"),
		"admin of a trailing slash":  set(tok(roles.Admin, "/p/a"), "project", "/p/a/"),
		"scoped admin at user scope": set(tok(roles.Admin, "/p/a"), "user", ""),
		"scoped admin at team scope": set(tok(roles.Admin, "/p/a"), "team", ""),
		"admin of an empty project":  set(tok(roles.Admin, "/p/a"), "project", ""),
		"admin of case-flipped path": set(tok(roles.Admin, "/p/a"), "project", "/P/A"),
		"admin of path with spaces":  set(tok(roles.Admin, "/p/a"), "project", "/p/a "),
		"member at user scope":       set(tok(roles.Member, roles.AllProjects), "user", ""),
	}
	for name, err := range cases {
		if err == nil {
			t.Errorf("%s: write accepted", name)
		}
	}
}
