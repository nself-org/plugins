package main

// Purpose: P7-AUTO-13 through the real CLI path. `nself ci --where <node>` on a
// tree with uncommitted work runs runV15 -> parseBare (--where) -> the
// Coordinator -> the registered OffloadHook -> offload.Take -> the ssh driver's
// bundle -> the node's checkout, and the node's job sees the user's tree.
// Inputs:  a committed repository (newEvRepo) made dirty, one registered node.
// Outputs: exit codes, stderr, what the node saw, the repository before/after.
// Constraints: NSELF_V15=1 (newEvRepo); no Docker, no network, no real ssh.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/where"
)

const (
	oflEnvSecret   = "ENV_VALUE_MUST_NOT_TRAVEL"
	oflLocalSecret = "ENV_LOCAL_VALUE_MUST_NOT_TRAVEL"
)

func resetWhere(t *testing.T) {
	t.Cleanup(func() { whereValue, whereChosen = "", where.Target{} })
}

// dirtyNodeRepo is a committed repository with an ignore file, made dirty the
// way a laptop is: an edited tracked file, a new file, an ignored .env and an
// untracked .env.local; node n1 is registered and reachable. HOME is private:
// git reads the user's global excludes, which may already ignore .env.local.
func dirtyNodeRepo(t *testing.T) (*evRepo, *oflNode, string) {
	t.Helper()
	resetWhere(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	r := newEvRepo(t)
	r.commit("f.txt", "committed\n")
	head := r.commit(".gitignore", ".env\nignored.txt\n")
	n := oflRegisterNode(t, r.home, r.dir, "n1", "f.txt", "new.txt", ".env", ".env.local", "ignored.txt")
	put(t, r.dir, "f.txt", "edited on the laptop\n")
	put(t, r.dir, "new.txt", "untracked and not ignored\n")
	put(t, r.dir, ".env", oflEnvSecret+"\n")
	put(t, r.dir, ".env.local", oflLocalSecret+"\n")
	put(t, r.dir, "ignored.txt", "ignored\n")
	return r, n, head
}

// oflRepoState is everything about the user's repository an offload must leave alone.
func oflRepoState(t *testing.T, dir string) string {
	t.Helper()
	parts := []string{
		git(t, dir, "status", "--porcelain=v2", "--ignored"),
		git(t, dir, "for-each-ref"),
		git(t, dir, "ls-files", "-s"),
		git(t, dir, "rev-parse", "HEAD"),
	}
	for _, name := range []string{"f.txt", "new.txt", ".env", ".env.local", "ignored.txt", ".gitignore"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		parts = append(parts, name+"="+string(b))
	}
	return strings.Join(parts, "\n--\n")
}

func TestCLIWhereNodeOffloadsDirtyTree(t *testing.T) {
	r, n, head := dirtyNodeRepo(t)
	before := oflRepoState(t, r.dir)
	out, errOut, code := r.ci("--where", "n1")
	if code != 0 {
		t.Fatalf("exit %d\nstdout %s\nstderr %s", code, out, errOut)
	}
	if whereChosen.Kind != where.KindNode {
		t.Fatalf("--where n1 resolved to %q, want a node target", whereChosen.Kind)
	}
	s := n.wait(t)
	if s.err != nil {
		t.Fatalf("the node could not stage the checkout: %v", s.err)
	}
	if s.head == "" || s.head == head {
		t.Fatalf("node HEAD %q: want the snapshot commit, not the committed HEAD %s", s.head, head)
	}
	if parent := git(t, r.dir, "rev-parse", "--verify", "-q", s.head+"^"); parent != head {
		t.Errorf("snapshot parent %s, want HEAD %s", parent, head)
	}
	if s.files["f.txt"] != "edited on the laptop\n" || s.files["new.txt"] != "untracked and not ignored\n" {
		t.Errorf("the node did not get the user's tree: %q", s.files)
	}
	for _, name := range []string{".env", ".env.local", "ignored.txt"} {
		if c, ok := s.files[name]; ok {
			t.Errorf("%s reached the node (%d bytes)", name, len(c))
		}
	}
	if !strings.Contains(errOut, "not sent to the node") || !strings.Contains(errOut, ".env.local") {
		t.Errorf("the excluded untracked .env.local is not named:\nstderr %s\nstdout %s", errOut, out)
	}
	for _, secret := range []string{oflEnvSecret, oflLocalSecret} {
		if strings.Contains(out+errOut, secret) {
			t.Errorf("secret content %q reached the output", secret)
		}
	}
	if after := oflRepoState(t, r.dir); after != before {
		t.Errorf("the user's repository changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
	if refs := git(t, r.dir, "for-each-ref", "refs/nself-ci/"); refs != "" {
		t.Errorf("private refs left behind:\n%s", refs)
	}
}

func TestCLIWhereNodeTrackedSecretRefused(t *testing.T) {
	resetWhere(t)
	r := newEvRepo(t)
	r.commit("deploy.pem", "-----BEGIN NOT A REAL KEY-----\n")
	oflRegisterNode(t, r.home, r.dir, "n1")
	put(t, r.dir, "f.txt", "uncommitted\n")
	before := oflRepoState2(t, r.dir)
	out, errOut, code := r.ci("--where", "n1")
	if code != 2 || !strings.Contains(errOut, "E807") || !strings.Contains(errOut, "deploy.pem") {
		t.Fatalf("a tracked secret-shaped file must refuse the offload with E807 and exit 2: exit %d\nstdout %s\nstderr %s", code, out, errOut)
	}
	if strings.Contains(out+errOut, "NOT A REAL KEY") {
		t.Error("the refusal printed the file's content")
	}
	if after := oflRepoState2(t, r.dir); after != before {
		t.Errorf("the user's repository changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

// oflRepoState2 is the git part of oflRepoState, for a repository without the probe files.
func oflRepoState2(t *testing.T, dir string) string {
	t.Helper()
	return strings.Join([]string{git(t, dir, "status", "--porcelain=v2", "--ignored"), git(t, dir, "for-each-ref"),
		git(t, dir, "ls-files", "-s")}, "\n--\n")
}

// TestOffloadOnlyForTargetsThatReachANode: a run with no --where, `local`,
// `hosted`, or pool/auto/all with only hosted providers registered keeps the
// HEAD path and never runs git for a snapshot.
func TestOffloadOnlyForTargetsThatReachANode(t *testing.T) {
	resetWhere(t)
	was := offloadCapture
	t.Cleanup(func() { offloadCapture = was })
	offloadCapture = func(context.Context, ...string) (string, int, error) {
		t.Fatal("a snapshot was attempted for a run no node can take")
		return "", 1, nil
	}
	ctx := context.Background()
	withKnown(t, where.Known{Providers: []string{"github"}}, "")
	for _, kind := range []string{"", where.KindLocal, where.KindHosted, where.KindPool, where.KindAuto, where.KindAll} {
		whereChosen = where.Target{Kind: kind}
		if s, err := buildOffload(ctx, t.TempDir()); s != nil || err != nil {
			t.Errorf("kind %q, providers only: snapshot %v err %v, want the HEAD path", kind, s, err)
		}
	}
	withKnown(t, where.Known{Nodes: []string{"n1"}, Providers: []string{"github"}}, "")
	for _, kind := range []string{where.KindNode, where.KindPool, where.KindAuto, where.KindAll} {
		if !offloadWanted(ctx, kind) {
			t.Errorf("kind %q may run on a node and must offload a dirty tree", kind)
		}
	}
	for _, kind := range []string{"", where.KindLocal, where.KindHosted} {
		if offloadWanted(ctx, kind) {
			t.Errorf("kind %q never runs on a node", kind)
		}
	}
}

// TestOffloadCleanTreeKeepsHEAD: a node target on a clean tree (the hook also
// runs when only the config lives outside the tree) creates no snapshot and no ref.
func TestOffloadCleanTreeKeepsHEAD(t *testing.T) {
	resetWhere(t)
	r := newEvRepo(t)
	whereChosen = where.Target{Kind: where.KindNode}
	s, err := buildOffload(context.Background(), r.dir)
	if s != nil || err != nil {
		t.Fatalf("clean tree: snapshot %v err %v", s, err)
	}
	if refs := git(t, r.dir, "for-each-ref", "refs/nself-ci/"); refs != "" {
		t.Errorf("a clean tree left a ref:\n%s", refs)
	}
}
