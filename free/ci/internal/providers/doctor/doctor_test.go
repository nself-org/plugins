package doctor

// Purpose: tests of the providers section of `nself ci doctor --providers` (P7-HOST-12): each CI-DP check
// fires on its own fixture, the section sends no request, the credential kinds warn the way review F18
// asks, and no credential value reaches a check.
// Inputs:  a real SQLite store, a local git repository with a GitHub origin URL (no network: the section
// refuses fetch), a fake `gh` and a map as the environment.
// Outputs: pass or fail. Fake values only.
// Constraints: a missing prerequisite is a t.Fatal, never a skip; the tests named by the Ticket's verify
// line are top-level.

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	cidoctor "github.com/nself-org/plugins/free/ci/internal/doctor"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/health"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/report"
	"github.com/nself-org/plugins/free/ci/internal/store"
	githubtmpl "github.com/nself-org/plugins/free/ci/templates/github"
)

const (
	plantedToken  = "github_pat_PLANTEDvalue0001"
	plantedSecret = "PLANTED-hook-secret-0002"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// newRepo makes a repository whose origin is a GitHub URL (never contacted) with main as the default branch.
func newRepo(t *testing.T, remote string) string {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "base")
	if remote != "" {
		git(t, dir, "remote", "add", "origin", remote)
		git(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
		git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	return dir
}

// publish puts content at the workflow path on origin/<ref> (a remote-tracking ref made locally).
func publish(t *testing.T, dir, ref, content string) {
	t.Helper()
	git(t, dir, "checkout", "-q", "-b", "tmp-"+strings.ReplaceAll(ref, "/", "-"), "main")
	path := filepath.Join(dir, githubtmpl.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "workflow on "+ref)
	git(t, dir, "update-ref", "refs/remotes/origin/"+ref, "HEAD")
	git(t, dir, "checkout", "-q", "main")
}

func renderWorkflow(t *testing.T) string {
	t.Helper()
	commits, err := githubtmpl.SourceCommits()
	if err != nil || len(commits) == 0 {
		t.Fatalf("no pinned agent commit: %v", err)
	}
	doc, err := githubtmpl.Render(githubtmpl.Options{AgentSource: githubtmpl.SourceMode, AgentCommit: commits[len(commits)-1]})
	if err != nil {
		t.Fatal(err)
	}
	return string(doc)
}

// rig is one doctor run's world.
type rig struct {
	t       *testing.T
	root    string
	st      *store.Store
	vars    map[string]string
	gh      func() (string, int)
	mu      sync.Mutex
	argvs   [][]string
	checked []cidoctor.Check
}

func newRig(t *testing.T, remote string) *rig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return &rig{t: t, root: newRepo(t, remote), st: st, vars: map[string]string{}, gh: func() (string, int) { return "", 1 }}
}

func (r *rig) capture(ctx context.Context, argv ...string) (string, int, error) {
	r.mu.Lock()
	r.argvs = append(r.argvs, append([]string(nil), argv...))
	r.mu.Unlock()
	if len(argv) > 0 && argv[0] == "gh" {
		out, exit := r.gh()
		return out, exit, nil
	}
	return report.Capture(ctx, argv...)
}

func (r *rig) row(key, value string) {
	r.t.Helper()
	err := r.st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "user", Subject: policy.LocalSubject, Key: key, Value: json.RawMessage(value), Actor: "test"}, nil)
	if err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) run() []cidoctor.Check {
	r.t.Helper()
	env := cidoctor.Env{Root: r.root, Capture: r.capture, Getenv: func(k string) string { return r.vars[k] }}
	start := time.Now()
	r.checked = RunWith(context.Background(), env, r.st)
	if d := time.Since(start); d > cidoctor.SectionBudget {
		r.t.Fatalf("the section took %s, over its %s budget", d, cidoctor.SectionBudget)
	}
	return r.checked
}

func find(t *testing.T, checks []cidoctor.Check, id, subject string) cidoctor.Check {
	t.Helper()
	for _, c := range checks {
		if c.ID == id && c.Subject == subject {
			return c
		}
	}
	t.Fatalf("no %s check with subject %q in %+v", id, subject, checks)
	return cidoctor.Check{}
}

func want(t *testing.T, c cidoctor.Check, status cidoctor.Status, contains string) {
	t.Helper()
	if c.Status != status || !strings.Contains(c.Message, contains) {
		t.Fatalf("%s: want %s containing %q, got %s %q", c.ID, status, contains, c.Status, c.Message)
	}
	if c.Status != cidoctor.Pass && c.Remediation == "" {
		t.Fatalf("%s: a non-passing check needs a remediation", c.ID)
	}
}

const ghRemote = "https://github.com/octo/widgets.git"

// TestDoctorChecks proves acceptance line 3: `ci doctor --providers` lists CI-DP checks with ids,
// statuses and remediations, and each check fires on its fixture: missing token env, gh unauthenticated,
// template drift on the untrusted ref, missing webhook secret env, fork approval weaker than
// all_external_contributors with JIT enabled, provider down. The section sends no request.
func TestDoctorChecks(t *testing.T) {
	t.Run("a clean setup passes and exits 0", func(t *testing.T) {
		r := newRig(t, ghRemote)
		doc := renderWorkflow(t)
		publish(t, r.root, "main", doc)
		publish(t, r.root, "nself-ci/untrusted", doc)
		r.gh = func() (string, int) { return "  - Token: github_pat_************************************\n", 0 }
		checks := r.run()
		for _, c := range checks {
			if c.ID == "" || !strings.HasPrefix(c.ID, "CI-DP") || c.Status != cidoctor.Pass {
				t.Fatalf("a clean setup must have only passing CI-DP checks: %+v", c)
			}
		}
		if code := cidoctor.ExitCode(checks); code != cidoctor.ExitClean {
			t.Fatalf("exit %d", code)
		}
		for _, id := range []string{"CI-DP001", "CI-DP002", "CI-DP005", "CI-DP006", "CI-DP007", "CI-DP013"} {
			find(t, checks, id, "")
		}
	})
	t.Run("missing token env", func(t *testing.T) {
		r := newRig(t, ghRemote)
		r.row("providers.github.token_env", `"NSELF_TEST_GH_TOKEN"`)
		c := find(t, r.run(), "CI-DP001", "")
		want(t, c, cidoctor.Fail, "NSELF_TEST_GH_TOKEN")
		r.vars["NSELF_TEST_GH_TOKEN"] = plantedToken
		want(t, find(t, r.run(), "CI-DP001", ""), cidoctor.Pass, "env:NSELF_TEST_GH_TOKEN")
	})
	t.Run("gh unauthenticated", func(t *testing.T) {
		r := newRig(t, ghRemote)
		want(t, find(t, r.run(), "CI-DP001", ""), cidoctor.Warn, "gh is not authenticated")
	})
	t.Run("template drift on the untrusted ref", func(t *testing.T) {
		r := newRig(t, ghRemote)
		doc := renderWorkflow(t)
		publish(t, r.root, "main", doc)
		publish(t, r.root, "nself-ci/untrusted", strings.Replace(doc, "# template: v1", "# template: v0", 1))
		checks := r.run()
		want(t, find(t, checks, "CI-DP006", ""), cidoctor.Pass, "is ok")
		want(t, find(t, checks, "CI-DP007", ""), cidoctor.Warn, "is drifted")
		if cidoctor.ExitCode(checks) == cidoctor.ExitClean {
			t.Fatal("a drifted copy must not exit 0")
		}
	})
	t.Run("workflow missing or not generated", func(t *testing.T) {
		r := newRig(t, ghRemote)
		publish(t, r.root, "main", "name: hand written\n")
		checks := r.run()
		want(t, find(t, checks, "CI-DP006", ""), cidoctor.Warn, "not-generated")
		want(t, find(t, checks, "CI-DP007", ""), cidoctor.Warn, "missing")
	})
	t.Run("missing webhook secret env", func(t *testing.T) {
		r := newRig(t, ghRemote)
		r.row("providers.github.webhook_secrets", `{"octo/widgets":"NSELF_TEST_HOOK"}`)
		want(t, find(t, r.run(), "CI-DP008", "octo/widgets"), cidoctor.Fail, "NSELF_TEST_HOOK")
		r.vars["NSELF_TEST_HOOK"] = plantedSecret
		want(t, find(t, r.run(), "CI-DP008", ""), cidoctor.Pass, "all webhook secret variables are set")
	})
	t.Run("fork approval weaker than all_external_contributors with JIT enabled", func(t *testing.T) {
		r := newRig(t, ghRemote)
		put := func(policy string) {
			raw, _ := json.Marshal(map[string]string{"approval_policy": policy})
			if _, err := r.st.ProviderFactPut(context.Background(), store.ProviderFact{Provider: "github", Scope: "octo/widgets", Key: "fork_pr_approval_policy", Value: raw, ObservedAt: time.Now()}); err != nil {
				t.Fatal(err)
			}
		}
		put("first_time_contributors")
		want(t, find(t, r.run(), "CI-DP010", ""), cidoctor.Pass, "not enabled") // JIT is off: nothing to warn about
		r.row("providers.github.jit.label", `"nself"`)
		want(t, find(t, r.run(), "CI-DP010", ""), cidoctor.Warn, "first_time_contributors")
		put("all_external_contributors")
		want(t, find(t, r.run(), "CI-DP010", ""), cidoctor.Pass, "all external contributors")
	})
	t.Run("provider down", func(t *testing.T) {
		r := newRig(t, ghRemote)
		tr := health.New(health.Options{Store: r.st})
		for i := 0; i < health.DownAfter; i++ {
			tr.Observe(rest.Outcome{Provider: "github-hosted", Class: providers.Outage, Status: 503})
		}
		c := find(t, r.run(), "CI-DP011", "github")
		want(t, c, cidoctor.Fail, "is down")
		if cidoctor.ExitCode(r.checked) != cidoctor.ExitFailed {
			t.Fatal("a down provider must exit 10")
		}
	})
	t.Run("forgejo url", func(t *testing.T) {
		r := newRig(t, ghRemote)
		r.row("providers.forgejo.url", `"http://forge.example.com:3844"`)
		want(t, find(t, r.run(), "CI-DP012", ""), cidoctor.Warn, "plain http")
	})
	t.Run("the section sends no request", func(t *testing.T) {
		r := newRig(t, ghRemote)
		r.run()
		for _, argv := range r.argvs {
			for _, a := range argv {
				if a == "fetch" || a == "push" || a == "pull" || a == "ls-remote" || a == "clone" {
					t.Fatalf("the section ran %v", argv)
				}
			}
			if argv[0] == "gh" && (len(argv) < 3 || argv[1] != "auth" || argv[2] != "status") {
				t.Fatalf("the only gh call is `gh auth status`, got %v", argv)
			}
		}
	})
	t.Run("a repository with no GitHub remote has nothing to check", func(t *testing.T) {
		r := newRig(t, "")
		checks := r.run()
		want(t, find(t, checks, "CI-DP006", ""), cidoctor.Pass, "no GitHub remote")
		want(t, find(t, checks, "CI-DP007", ""), cidoctor.Pass, "no GitHub remote")
	})
}

// TestTokenKindWarning proves acceptance line 7 (review F18): the doctor warns for a gho_ or ghp_
// GitHub credential and does not for github_pat_.
func TestTokenKindWarning(t *testing.T) {
	for kind, prefix := range map[string]string{KindClassic: "ghp_", KindOAuth: "gho_", KindFineGrained: "github_pat_"} {
		if got := TokenKind(prefix + "PLANTED0001"); got != kind {
			t.Fatalf("TokenKind(%s…) = %s, want %s", prefix, got, kind)
		}
	}
	for _, c := range []struct {
		name   string
		env    string // the token in providers.github.token_env, "" for gh
		gh     string
		status cidoctor.Status
	}{
		{"classic PAT in env", "ghp_PLANTED0001", "", cidoctor.Warn},
		{"gh OAuth login", "", "  - Token: gho_************************************\n", cidoctor.Warn},
		{"gh login, kind not printed", "", "logged in\n", cidoctor.Warn},
		{"fine-grained PAT in env", plantedToken, "", cidoctor.Pass},
		{"gh login with a fine-grained token", "", "  - Token: github_pat_************************************\n", cidoctor.Pass},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t, "")
			if c.env != "" {
				r.row("providers.github.token_env", `"NSELF_TEST_GH_TOKEN"`)
				r.vars["NSELF_TEST_GH_TOKEN"] = c.env
			} else {
				r.gh = func() (string, int) { return c.gh, 0 }
			}
			got := find(t, r.run(), "CI-DP002", "")
			if got.Status != c.status {
				t.Fatalf("CI-DP002 %s %q, want %s", got.Status, got.Message, c.status)
			}
			if c.status == cidoctor.Warn && !strings.Contains(got.Remediation, "fine-grained") {
				t.Fatalf("the warning must point at a fine-grained token: %q", got.Remediation)
			}
			for _, ch := range r.checked {
				if strings.Contains(ch.Message+ch.Detail+ch.Remediation, "PLANTED") {
					t.Fatalf("a credential value reached %s: %q", ch.ID, ch.Message)
				}
			}
		})
	}
}
