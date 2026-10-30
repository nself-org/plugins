package offload

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func capture(ctx context.Context, argv ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var b bytes.Buffer
	cmd.Stdout, cmd.Stderr = &b, &b
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		return b.String(), ee.ExitCode(), nil
	}
	return b.String(), 0, err
}

func git(t *testing.T, root string, args ...string) string {
	t.Helper()
	argv := append([]string{"env", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null", "git", "-C", root, "-c", "user.name=t", "-c", "user.email=t@localhost"}, args...)
	out, code, err := capture(context.Background(), argv...)
	if err != nil || code != 0 {
		t.Fatalf("git %v: %v %d %s", args, err, code, out)
	}
	return strings.TrimSpace(out)
}

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// hermeticGit keeps the host's git configuration out of the test: Take runs
// git with the process environment, and a global excludes file (for example
// one listing .env.local) would change what counts as untracked.
func hermeticGit(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	global := filepath.Join(home, "gitconfig")
	if err := os.WriteFile(global, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
}

func newRepo(t *testing.T) string {
	hermeticGit(t)
	root := t.TempDir()
	git(t, root, "init", "-q")
	write(t, root, "a.txt", "one\n")
	write(t, root, ".gitignore", "ignored.txt\n.env\n")
	git(t, root, "add", ".")
	git(t, root, "commit", "-q", "-m", "base")
	return root
}

func state(t *testing.T, root string) string {
	return git(t, root, "status", "--porcelain=v2", "--ignored") + git(t, root, "for-each-ref") + git(t, root, "ls-files", "-s")
}

func TestSnapshotLeavesIndexAndRefs(t *testing.T) {
	root := newRepo(t)
	write(t, root, "a.txt", "two\n")
	write(t, root, "new.txt", "n\n")
	before := state(t, root)
	s, err := Take(context.Background(), capture, root, t.TempDir(), "r1")
	if err != nil {
		t.Fatal(err)
	}
	if s.Clean || s.Ref != RefPrefix+"r1" {
		t.Fatalf("snapshot %+v", s)
	}
	if got := git(t, root, "show", s.SHA+":a.txt"); got != "two" {
		t.Fatalf("tracked change missing: %q", got)
	}
	if git(t, root, "show", s.SHA+":new.txt") != "n" {
		t.Fatal("untracked file missing")
	}
	if git(t, root, "rev-parse", s.SHA+"^") != git(t, root, "rev-parse", "HEAD") {
		t.Fatal("parent is not HEAD")
	}
	if err := Release(context.Background(), capture, root, s); err != nil {
		t.Fatal(err)
	}
	if after := state(t, root); after != before {
		t.Fatalf("user state changed:\n%s\n---\n%s", before, after)
	}
}

func TestSnapshotExcludesIgnoredAndEnv(t *testing.T) {
	root := newRepo(t)
	write(t, root, "ignored.txt", "x")
	write(t, root, ".env", "x")
	write(t, root, ".env.local", "x")
	write(t, root, "ok.txt", "x")
	s, err := Take(context.Background(), capture, root, t.TempDir(), "r2")
	if err != nil {
		t.Fatal(err)
	}
	defer Release(context.Background(), capture, root, s)
	files := git(t, root, "ls-tree", "-r", "--name-only", s.SHA)
	for _, bad := range []string{"ignored.txt", ".env"} {
		for _, f := range strings.Split(files, "\n") {
			if f == bad {
				t.Fatalf("%s was sent", bad)
			}
		}
	}
	if !strings.Contains(files, "ok.txt") || len(s.Excluded) != 1 || s.Excluded[0] != ".env.local" {
		t.Fatalf("files %q excluded %v", files, s.Excluded)
	}
}

func TestTrackedSecretRefused(t *testing.T) {
	root := newRepo(t)
	write(t, root, "deploy.pem", "x")
	git(t, root, "add", "-f", "deploy.pem")
	git(t, root, "commit", "-q", "-m", "pem")
	write(t, root, "a.txt", "changed\n")
	_, err := Take(context.Background(), capture, root, t.TempDir(), "r3")
	if err == nil || !strings.HasPrefix(err.Error(), CodeTracked) || !strings.Contains(err.Error(), "deploy.pem") {
		t.Fatalf("err = %v", err)
	}
	if git(t, root, "for-each-ref", RefPrefix) != "" {
		t.Fatal("ref left behind")
	}
}

func TestSnapshotRefCleanup(t *testing.T) {
	root := newRepo(t)
	s, err := Take(context.Background(), capture, root, t.TempDir(), "r4")
	if err != nil || !s.Clean || s.Ref != "" {
		t.Fatalf("clean tree: %+v %v", s, err)
	}
	write(t, root, "a.txt", "dirty\n")
	s, err = Take(context.Background(), capture, root, t.TempDir(), "r5")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Release(context.WithoutCancel(ctx), capture, root, s); err != nil {
		t.Fatal(err)
	}
	if err := Release(context.Background(), capture, root, s); err != nil {
		t.Fatal("second release must be a no-op")
	}
	if git(t, root, "for-each-ref", RefPrefix) != "" {
		t.Fatal("private ref remains")
	}
}

func TestSecretPathAndBadID(t *testing.T) {
	for p, want := range map[string]bool{".env": true, "x/.env.local": true, "a.pem": true, "k.KEY": true, "id_rsa": true, ".npmrc": true, "credentials.json": true, "main.go": false, "environment.go": false} {
		if SecretPath(p) != want {
			t.Errorf("SecretPath(%q) != %v", p, want)
		}
	}
	if _, err := Take(context.Background(), capture, ".", t.TempDir(), "../x"); err == nil {
		t.Fatal("bad id accepted")
	}
}

// A file added with `git add` and not committed is part of the user's tree: the
// snapshot carries its working-tree content. A staged file deleted from the
// working tree is not carried. The user's index file is byte-identical after.
func TestSnapshotCarriesStagedNewFile(t *testing.T) {
	root := newRepo(t)
	write(t, root, "staged.txt", "staged\n")
	write(t, root, "edited.txt", "staged version\n")
	write(t, root, "gone.txt", "g\n")
	git(t, root, "add", "staged.txt", "edited.txt", "gone.txt")
	write(t, root, "edited.txt", "working tree version\n")
	if err := os.Remove(filepath.Join(root, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	indexPath := filepath.Join(root, ".git", "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Take(context.Background(), capture, root, t.TempDir(), "r5")
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	defer Release(context.Background(), capture, root, s)
	files := "\n" + git(t, root, "ls-tree", "-r", "--name-only", s.SHA) + "\n"
	if !strings.Contains(files, "\nstaged.txt\n") {
		t.Errorf("a staged new file is missing from the snapshot:%s", files)
	}
	if got := git(t, root, "show", s.SHA+":edited.txt"); got != "working tree version" {
		t.Errorf("edited.txt = %q, want the working-tree content", got)
	}
	if strings.Contains(files, "\ngone.txt\n") {
		t.Errorf("a staged file deleted from the working tree was sent:%s", files)
	}
	if !bytes.Equal(before, after) {
		t.Error("the user's index file changed")
	}
}
