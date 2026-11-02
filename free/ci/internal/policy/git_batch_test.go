//go:build !windows

package policy

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/gitbatch"
)

// osRunner is a Runner on plain git: the reference the batch answers must equal.
func osRunner(ctx context.Context, argv ...string) (string, int, error) {
	out, err := osexec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
	var ee *osexec.ExitError
	if errors.As(err, &ee) {
		return string(out), ee.ExitCode(), nil
	}
	return string(out), 0, err
}

func TestBranchObjectReadsAgreeWithGit(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".nself"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".nself", "ci.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"},
		{"-c", "user.name=t", "-c", "user.email=t@example.invalid", "-c", "commit.gpgsign=false", "commit", "-q", "-m", "x"}} {
		cmd := osexec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	g := gitRunner{run: osRunner, root: dir}
	set := gitbatch.NewSet(context.Background(), gitbatch.Options{})
	defer set.Close()
	plain, batched := context.Background(), gitbatch.WithSet(context.Background(), set)

	sha, _ := g.git(plain, "rev-parse", "HEAD")
	bad := strings.Repeat("2", 40)
	for _, name := range []string{"HEAD^{commit}", sha + "^{commit}", "refs/remotes/origin/nope^{commit}", bad + "^{commit}"} {
		wantOut, wantOK := g.verifyCommit(plain, name)
		gotOut, gotOK := g.verifyCommit(batched, name)
		if wantOK != gotOK || lastLine(wantOut) != lastLine(gotOut) {
			t.Fatalf("verifyCommit(%s): git (%q,%v), batch (%q,%v)", name, wantOut, wantOK, gotOut, gotOK)
		}
	}
	for _, c := range []struct{ sha, rel string }{{sha, ".nself/ci.yaml"}, {sha, ".nself"}, {sha, "absent"}, {bad, ".nself/ci.yaml"}} {
		wantOut, wantOK := g.lsTree(plain, c.sha, c.rel)
		gotOut, gotOK := g.lsTree(batched, c.sha, c.rel)
		if wantOK != gotOK || (wantOK && wantOut != gotOut) {
			t.Fatalf("lsTree(%s %s): git (%q,%v), batch (%q,%v)", c.sha, c.rel, wantOut, wantOK, gotOut, gotOK)
		}
	}
	listed, _ := g.lsTree(plain, sha, ".nself/ci.yaml")
	wantBlob, wantOK := g.showBlob(plain, sha, ".nself/ci.yaml", listed)
	gotBlob, gotOK := g.showBlob(batched, sha, ".nself/ci.yaml", listed)
	if !wantOK || !gotOK || wantBlob != gotBlob || gotBlob != "version: 1\n" {
		t.Fatalf("showBlob: git (%q,%v), batch (%q,%v)", wantBlob, wantOK, gotBlob, gotOK)
	}
	// An object id that is not a blob in the listing must fail, not fall back to something else.
	if _, ok := g.showBlob(batched, sha, ".nself/ci.yaml", "100644 blob "+bad+"\t.nself/ci.yaml"); ok {
		t.Fatal("a listing naming a missing blob must fail the read")
	}
}
