//go:build security

package redteam

// Purpose: red team attacks on source trust for local and same-repo work (TM-01, TM-02, TM-45).
//   - A27: a lockfile-only change (TM-02) is untrusted for secrets; lockfile names the list misses are a gap;
//   - A28: ClassifyLocal against spoofed reflog subjects and missing facts: a collaborator's or stranger's
//     commit is never read as the operator's own;
//   - A29: real git: a collaborator pushes a branch, the operator fetches or fast-forwards it, and the
//     classifier must not call it owner (the local coordinator refuses everything below internal);
//   - A32: the coordinator classifier must not default to owner when no trust hook is registered.
// Inputs:  trust/source.LockfileOnly and ClassifyLocal, run.SourceTrust over throwaway file:// git repos.
// Outputs: pass (control held) or fail (attack worked).
// Constraints: git runs on temp dirs only; no network, no global git config.

import (
	"context"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/trust/source"
)

func TestA27_LockfileOnlyIsUntrusted(t *testing.T) {
	for _, f := range []string{"package-lock.json", "web/pnpm-lock.yaml", "go.sum", "Cargo.lock", `app\Gemfile.lock`, "A/B/YARN.LOCK", "poetry.lock"} {
		if !source.LockfileOnly([]string{f}) {
			t.Errorf("%q is a lockfile and must count", f)
		}
	}
	for _, mixed := range [][]string{{"package-lock.json", "README.md"}, {}, {"package-lock.json/../x.js"}, {"package-lock.json.js"}} {
		if source.LockfileOnly(mixed) {
			t.Errorf("%q must not be lockfile-only", mixed)
		}
	}
}

// A27b: dependency lock files of mainstream ecosystems that the list does not know. A diff of only such files
// is classified like code, so it keeps the trust of its author and can carry secrets.
func TestA27b_LockfileListGaps(t *testing.T) {
	if !openFinding(t, "A11") {
		return
	}
	for _, f := range []string{".terraform.lock.hcl", "go.work.sum", "pixi.lock", "conda-lock.yml", "paket.lock", "rebar.lock", "MODULE.bazel.lock",
		"Cartfile.resolved", "shard.lock", "stack.yaml.lock", "cabal.project.freeze", "pnpm-lock.yml", "npm-shrinkwrap.json", "gradle/dependency-locks/compileClasspath.lockfile"} {
		if !source.LockfileOnly([]string{f}) {
			t.Errorf("lockfile %q is not recognised, so a change to only it keeps its author's trust", f)
		}
	}
}

func TestA28_ClassifyLocalSpoofs(t *testing.T) {
	foreign := "1111111111111111111111111111111111111111"
	base := func() source.GitFacts {
		return source.GitFacts{Head: foreign, DefaultRemoteRef: "refs/remotes/origin/main", OffDefault: []source.Commit{{SHA: foreign}},
			ReflogCreated: map[string]string{foreign: "commit: mine"}}
	}
	if r := source.ClassifyLocal(base()); r.Class != "owner" {
		t.Fatalf("control: a commit first recorded as made here is owner: %s %v", r.Class, r.Reasons)
	}
	for _, subject := range []string{"", "fetch origin: fast-forward", "branch: Created from FETCH_HEAD", "checkout: moving from main to feature", "pull: Fast-forward",
		"pull origin evil (pick): Fast-forward", "merge origin/evil: Fast-forward", "rebase (finish): refs/heads/x onto 123", "rebase (start): checkout origin/evil",
		"reset: moving to origin/evil", "am: commit: fix", "COMMIT: x", "fetch commit: x", "clone: from file:///x", "commit-x: y", "Merge made by: x",
		"update by push", "cherry", "push: commit: x"} {
		g := base()
		g.ReflogCreated[foreign] = subject
		if r := source.ClassifyLocal(g); r.Class == "owner" {
			t.Errorf("reflog subject %q made a foreign commit owner", subject)
		}
	}
	for name, mut := range map[string]func(*source.GitFacts){
		"reflog fact missing":    func(g *source.GitFacts) { g.Missing = []string{source.MissingReflog} },
		"commits fact missing":   func(g *source.GitFacts) { g.Missing = []string{source.MissingCommits} },
		"remote refs missing":    func(g *source.GitFacts) { g.Missing = []string{source.MissingRemoteRefs} },
		"default ref missing":    func(g *source.GitFacts) { g.Missing = []string{source.MissingDefaultRef}; g.DefaultRemoteRef = "" },
		"in progress":            func(g *source.GitFacts) { g.InProgress = true },
		"no head":                func(g *source.GitFacts) { g.Head = "" },
		"no remotes, incomplete": func(g *source.GitFacts) { g.NoRemotes = true; g.Missing = []string{source.MissingRemoteRefs} },
		"one foreign of two": func(g *source.GitFacts) {
			g.OffDefault = append(g.OffDefault, source.Commit{SHA: "2222222222222222222222222222222222222222"})
		},
		"reachable but no default ref": func(g *source.GitFacts) { g.ReachableFromDefault = true; g.DefaultRemoteRef = "" },
	} {
		g := base()
		mut(&g)
		if r := source.ClassifyLocal(g); r.Class == "owner" {
			t.Errorf("%s: classified owner (%v)", name, r.Reasons)
		}
	}
}

func aGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := osexec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A29: a collaborator's pushed branch, fetched or fast-forwarded by the operator, is not the operator's own.
func TestA29_FetchedCollaboratorBranchIsNotOwner(t *testing.T) {
	if _, err := osexec.LookPath("git"); err != nil {
		t.Fatal("git is required for this fixture")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "t")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "t@example.invalid")
	}
	root := t.TempDir()
	origin, op, collab := filepath.Join(root, "origin.git"), filepath.Join(root, "op"), filepath.Join(root, "collab")
	aGit(t, root, "init", "-q", "--bare", "-b", "main", origin)
	aGit(t, root, "clone", "-q", "file://"+origin, op)
	aMust(t, os.WriteFile(filepath.Join(op, "a.txt"), []byte("1"), 0o600))
	aGit(t, op, "add", ".")
	aGit(t, op, "commit", "-q", "-m", "base")
	aGit(t, op, "push", "-q", "origin", "main")
	aGit(t, root, "clone", "-q", "file://"+origin, collab)
	aGit(t, collab, "checkout", "-q", "-b", "evil(pick)")
	aMust(t, os.WriteFile(filepath.Join(collab, "install.sh"), []byte("echo pwn"), 0o600))
	aGit(t, collab, "add", ".")
	aGit(t, collab, "commit", "-q", "-m", "commit: Merge made by the ort strategy")
	aGit(t, collab, "push", "-q", "origin", "evil(pick)")

	classify := func(rev string) source.Result {
		res, err := run.SourceTrust{Getenv: func(string) string { return "" }}.Decide(context.Background(), run.TrustRequest{Root: op, Revision: rev})
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		return res
	}
	aGit(t, op, "fetch", "-q", "origin")
	head := aGit(t, op, "rev-parse", "origin/evil(pick)")
	if res := classify(head); res.Class == "owner" {
		t.Errorf("a fetched collaborator head (never checked out) is owner: %v", res.Reasons)
	}
	aGit(t, op, "checkout", "-q", "-b", "review", "origin/evil(pick)")
	if res := classify(head); res.Class == "owner" {
		t.Errorf("a checked-out collaborator head is owner: %v", res.Reasons)
	}
	aGit(t, op, "checkout", "-q", "main")
	aGit(t, op, "merge", "-q", "--ff-only", "origin/evil(pick)")
	if res := classify(head); res.Class == "owner" {
		t.Errorf("a collaborator head fast-forwarded into main is owner: %v", res.Reasons)
	}
	aGit(t, op, "reset", "-q", "--hard", "HEAD~1")
	aMust(t, os.WriteFile(filepath.Join(op, "b.txt"), []byte("2"), 0o600))
	aGit(t, op, "add", ".")
	aGit(t, op, "commit", "-q", "-m", "mine")
	if res := classify(aGit(t, op, "rev-parse", "HEAD")); res.Class != "owner" {
		t.Errorf("control: the operator's own unpushed commit must be owner: %s %v", res.Class, res.Reasons)
	}
}

// A10: with no trust hook registered the coordinator reads every source as owner.
func TestA32_NoTrustHookDefaultsToOwner(t *testing.T) {
	if !openFinding(t, "A10") {
		return
	}
	b, err := os.ReadFile(filepath.Join(aModuleRoot, "internal/run/coordinator.go"))
	aMust(t, err)
	text := string(b)
	i := strings.Index(text, "func (c *Coordinator) classify(")
	if i < 0 {
		t.Fatal("classify not found")
	}
	body := text[i:min(len(text), i+400)]
	if strings.Contains(body, "if h.Trust == nil {\n\t\treturn \"owner\", nil") {
		t.Error("Coordinator.classify returns owner when no TrustClassifier is registered (fail open); it should refuse")
	}
}
