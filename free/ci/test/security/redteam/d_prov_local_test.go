//go:build security

package redteam

// Purpose: TM-32 red team, local half. A stranger's commit reaches the operator's checkout through
// routes the builder suite did not use: a replay of the stranger's single commit (rebase, amend,
// cherry-pick), a fetch refspec that maps pull requests into an unlisted namespace, a ref named like
// an owner ref, a git grafts file, and a malformed revision. The real SourceTrust hook runs against
// real git repositories.
// Inputs:  throwaway git repositories under t.TempDir().
// Outputs: pass = the stranger's code is not classified above what its provenance earns (control HELD).
// Constraints: local git only, no network; committer identity is the operator's on every commit.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

type dFx struct {
	t                         *testing.T
	op, origin, stranger, tmp string
}

func dGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0", "GIT_EDITOR=true")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (x *dFx) commit(dir, file string) string {
	x.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(file+"\n"), 0o644); err != nil {
		x.t.Fatal(err)
	}
	dGit(x.t, dir, "add", file)
	dGit(x.t, dir, "commit", "-q", "-m", "add "+file)
	return dGit(x.t, dir, "rev-parse", "HEAD")
}

func newDFx(t *testing.T) *dFx {
	t.Helper()
	x := &dFx{t: t, tmp: t.TempDir()}
	x.origin, x.op, x.stranger = filepath.Join(x.tmp, "origin.git"), filepath.Join(x.tmp, "op"), filepath.Join(x.tmp, "stranger")
	dGit(t, x.tmp, "init", "-q", "--bare", "-b", "main", x.origin)
	dGit(t, x.tmp, "clone", "-q", x.origin, x.op)
	cfg := func(dir, name string) {
		dGit(t, dir, "config", "user.name", name)
		dGit(t, dir, "config", "user.email", "operator@example.test")
		dGit(t, dir, "config", "commit.gpgsign", "false")
	}
	cfg(x.op, "Operator")
	x.commit(x.op, "base.txt")
	dGit(t, x.op, "push", "-q", "-u", "origin", "main")
	dGit(t, x.op, "remote", "set-head", "origin", "main")
	dGit(t, x.tmp, "clone", "-q", x.origin, x.stranger)
	cfg(x.stranger, "Mallory")
	dGit(t, x.stranger, "checkout", "-q", "-b", "pr")
	x.commit(x.stranger, "evil.txt") // the stranger's ONE commit on top of main
	return x
}

// advanceMain moves origin/main one commit ahead so a replay of the stranger's commit is not a fast-forward.
func (x *dFx) advanceMain() {
	x.t.Helper()
	other := filepath.Join(x.tmp, "other")
	dGit(x.t, x.tmp, "clone", "-q", x.origin, other)
	dGit(x.t, other, "config", "user.name", "Other")
	dGit(x.t, other, "config", "user.email", "other@example.test")
	dGit(x.t, other, "config", "commit.gpgsign", "false")
	x.commit(other, "newer.txt")
	dGit(x.t, other, "push", "-q", "origin", "main")
	dGit(x.t, x.op, "fetch", "-q", "origin")
}

func (x *dFx) decide(rev string) source.Result {
	x.t.Helper()
	if rev == "" {
		rev = dGit(x.t, x.op, "rev-parse", "HEAD")
	}
	h := run.SourceTrust{Getenv: func(string) string { return "" }}
	res, err := h.Decide(context.Background(), run.TrustRequest{Root: x.op, Revision: rev})
	if err != nil {
		x.t.Fatalf("Decide: %v", err)
	}
	return res
}

func (x *dFx) fetchStranger() string {
	dGit(x.t, x.op, "fetch", "-q", x.stranger, "pr")
	return dGit(x.t, x.op, "rev-parse", "FETCH_HEAD")
}

// D01: the operator replays the stranger's single commit (rebase onto newer main). The tree content
// is still the stranger's, so the class must stay below owner.
func TestD01_RebaseOfStrangerCommitStaysForeign(t *testing.T) {
	if !openFinding(t, "D01") {
		return
	}
	x := newDFx(t)
	sha := x.fetchStranger()
	x.advanceMain()
	dGit(t, x.op, "checkout", "-q", "-b", "x", sha)
	dGit(t, x.op, "rebase", "-q", "origin/main")
	if got := x.decide(""); got.Class == "owner" {
		t.Fatalf("stranger commit replayed by rebase is %s %v", got.Class, got.Reasons)
	}
}

// D02: the operator amends the stranger's single commit (a message tidy-up). Same content, new SHA.
func TestD02_AmendOfStrangerCommitStaysForeign(t *testing.T) {
	if !openFinding(t, "D01") {
		return
	}
	x := newDFx(t)
	dGit(t, x.op, "checkout", "-q", "-b", "pr", x.fetchStranger())
	dGit(t, x.op, "commit", "-q", "--amend", "-m", "tidy message")
	if got := x.decide(""); got.Class == "owner" {
		t.Fatalf("stranger commit amended by the operator is %s %v", got.Class, got.Reasons)
	}
}

// D03: the operator cherry-picks the stranger's commit onto a branch of main.
func TestD03_CherryPickOfStrangerCommitStaysForeign(t *testing.T) {
	if !openFinding(t, "D01") {
		return
	}
	x := newDFx(t)
	sha := x.fetchStranger()
	x.advanceMain()
	dGit(t, x.op, "checkout", "-q", "-b", "y", "origin/main")
	dGit(t, x.op, "cherry-pick", sha)
	if got := x.decide(""); got.Class == "owner" {
		t.Fatalf("stranger commit cherry-picked is %s %v", got.Class, got.Reasons)
	}
}

// D04: fetch refspec maps pull requests to refs/remotes/origin/gh-pr/<n>/head, a glob in the middle
// of the destination. A stranger's PR must not read as a branch of the origin remote.
func TestD04_PullRefspecWithMidPathGlobIsNotAnOriginBranch(t *testing.T) {
	if !openFinding(t, "D04") {
		return
	}
	x := newDFx(t)
	dGit(t, x.stranger, "push", "-q", "origin", "pr:refs/pull/7/head")
	dGit(t, x.op, "config", "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/gh-pr/*/head")
	dGit(t, x.op, "fetch", "-q", "origin")
	dGit(t, x.op, "checkout", "-q", "-b", "prx", "origin/gh-pr/7/head")
	if got := x.decide(""); got.Class != "untrusted" {
		t.Fatalf("stranger pull request mapped into an unlisted namespace is %s %v", got.Class, got.Reasons)
	}
}

// D05: refs named like owner refs (a branch and a tag called origin/main, a tag called main) that
// point at the stranger's commit must not read as the origin default branch.
func TestD05_RefsNamedLikeOwnerRefs(t *testing.T) {
	x := newDFx(t)
	sha := x.fetchStranger()
	dGit(t, x.op, "branch", "origin/main", sha)
	dGit(t, x.op, "tag", "origin/main", sha)
	dGit(t, x.op, "tag", "refs/remotes/origin/main", sha)
	dGit(t, x.op, "checkout", "-q", "--detach", sha)
	if got := x.decide(""); got.Class != "untrusted" {
		t.Fatalf("stranger commit with owner-looking ref names is %s %v", got.Class, got.Reasons)
	}
}

// D06: .git/info/grafts claims the stranger commit is the parent of origin/main, so it would look
// reachable from the default branch.
func TestD06_GraftsFileCannotMakeStrangerReachable(t *testing.T) {
	if !openFinding(t, "D06") {
		return
	}
	x := newDFx(t)
	sha := x.fetchStranger()
	tip := dGit(t, x.op, "rev-parse", "origin/main")
	graft := filepath.Join(x.op, ".git", "info", "grafts")
	if err := os.MkdirAll(filepath.Dir(graft), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(graft, []byte(tip+" "+sha+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dGit(t, x.op, "checkout", "-q", "--detach", sha)
	if got := x.decide(""); got.Class != "untrusted" {
		t.Fatalf("grafted stranger commit is %s %v", got.Class, got.Reasons)
	}
}

// D07: revisions that are not a lower-case full SHA, and one that is absent, never earn owner.
func TestD07_MalformedOrAbsentRevision(t *testing.T) {
	x := newDFx(t)
	head := dGit(t, x.op, "rev-parse", "origin/main")
	for name, rev := range map[string]string{
		"upper-case": strings.ToUpper(head), "abbreviated": head[:12], "ref name": "origin/main", "trailing newline": head + "\n",
		"absent 40": strings.Repeat("a", 40), "absent 64": strings.Repeat("b", 64), "with option": "--all", "space": head + " ",
	} {
		if got := x.decide(rev); got.Class == "owner" {
			t.Errorf("%s revision %q classified %s %v", name, rev, got.Class, got.Reasons)
		}
	}
}

func dF[T any](v T, src string) model.Fact[T] { return model.Fact[T]{Value: &v, Source: src} }

// D08: forge-side facts. A tag on a fork SHA, payload-claimed reachability, a spoofed fact source or
// auth label, and a stranger-looking runner event never earn more than the API facts give.
func TestD08_ForgeFactsForTagsAndSpoofedSources(t *testing.T) {
	base := func(kind model.TriggerKind) model.TriggerFacts {
		return model.TriggerFacts{Schema: "ci.trigger-facts/v1", Kind: kind, Auth: "per-repo", Repo: "o/r",
			ActorPermission: dF("admin", "api"), SHAReachable: dF(true, "api"), ProtectedRef: dF(true, "api"), FirstTime: dF(false, "api")}
	}
	if got := source.Classify(base("tag")); got.Class != "owner" {
		t.Fatalf("positive control: a reachable tag by an admin is %s %v", got.Class, got.Reasons)
	}
	cases := map[string]func(*model.TriggerFacts){
		"tag, reachability only claimed by the payload": func(f *model.TriggerFacts) { f.SHAReachable = dF(true, "payload") },
		"tag, reachability from the caller":             func(f *model.TriggerFacts) { f.SHAReachable = dF(true, "caller") },
		"tag on a fork SHA (api says unreachable)":      func(f *model.TriggerFacts) { f.SHAReachable = dF(false, "api") },
		"tag, no reachability fact at all":              func(f *model.TriggerFacts) { f.SHAReachable = model.Fact[bool]{} },
		"tag, head repo is a fork":                      func(f *model.TriggerFacts) { f.HeadRepo = dF("mallory/r", "api") },
		"tag, payload says fork":                        func(f *model.TriggerFacts) { f.Fork = dF(true, "payload") },
		"permission from a spoofed source label":        func(f *model.TriggerFacts) { f.ActorPermission = dF("admin", "API") },
		"permission with padding":                       func(f *model.TriggerFacts) { f.ActorPermission = dF(" admin", "api") },
		"auth label in another case":                    func(f *model.TriggerFacts) { f.Auth = "Per-Repo" },
		"auth shared":                                   func(f *model.TriggerFacts) { f.Auth = "shared" },
		"unknown kind in another case":                  func(f *model.TriggerFacts) { f.Kind = "Tag" },
		"bot actor":                                     func(f *model.TriggerFacts) { f.Actor = dF("dependabot[bot]", "payload"); f.Bot = dF(true, "payload") },
	}
	for name, edit := range cases {
		f := base("tag")
		edit(&f)
		if name == "bot actor" {
			f = base("push")
			edit(&f)
		}
		if got := source.Classify(f); got.Class == "owner" {
			t.Errorf("%s classified owner %v", name, got.Reasons)
		}
	}
	rd := base("runner_demand")
	rd.GroupProtectedOnly, rd.HeadRepo = dF(true, "api"), dF("o/r", "api")
	for _, ev := range []string{"Push", "push ", "pull_request_target", "issue_comment", "workflow_run", ""} {
		rd.RunEvent = dF(ev, "api")
		if got := source.Classify(rd); got.Class == "owner" {
			t.Errorf("runner demand event %q classified owner %v", ev, got.Reasons)
		}
	}
}
