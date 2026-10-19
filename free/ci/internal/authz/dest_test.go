package authz_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/authz"
)

func scratch(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func destReason(t *testing.T, dest, cwd string) authz.PathReason {
	t.Helper()
	_, err := authz.ArtifactGetDest(dest, cwd)
	var pe *authz.PathError
	if !errors.As(err, &pe) {
		t.Fatalf("dest %q: want refusal, got %v", dest, err)
	}
	return pe.Reason
}

func TestArtifactGetDest_Refusals(t *testing.T) {
	root := scratch(t)
	cwd := filepath.Join(root, "work")
	mustMkdir(t, cwd)
	nonEmpty := filepath.Join(root, "full")
	mustMkdir(t, nonEmpty)
	mustWrite(t, filepath.Join(nonEmpty, "f"))
	gitDir := filepath.Join(cwd, ".git")
	mustMkdir(t, filepath.Join(gitDir, "hooks"))
	file := filepath.Join(root, "afile")
	mustWrite(t, file)
	link := filepath.Join(root, "link")
	empty := filepath.Join(root, "empty")
	mustMkdir(t, empty)
	if err := os.Symlink(empty, link); err != nil {
		t.Fatal(err)
	}
	cwdLink := filepath.Join(root, "cwdlink")
	if err := os.Symlink(cwd, cwdLink); err != nil {
		t.Fatal(err)
	}
	rows := []struct {
		name string
		dest string
		want authz.PathReason
	}{
		{"cwd absolute", cwd, authz.DestCwd},
		{"cwd dot", ".", authz.DestCwd},
		{"cwd via symlink", filepath.Join(cwdLink, "."), authz.DestSymlink},
		{"cwd via symlink leaf", cwdLink, authz.DestSymlink},
		{"non-empty directory", nonEmpty, authz.DestNotEmpty},
		{"directory under .git", filepath.Join(gitDir, "hooks"), authz.DestUnderGit},
		{"new directory under .git", filepath.Join(gitDir, "new"), authz.DestUnderGit},
		{"the .git directory itself", gitDir, authz.DestUnderGit},
		{"relative into .git", ".git/hooks", authz.DestUnderGit},
		{"upper-case .GIT", filepath.Join(root, ".GIT", "x"), authz.DestUnderGit},
		{"ntfs short name GIT~1", filepath.Join(root, "GIT~1", "x"), authz.DestUnderGit},
		{"trailing-space .git", filepath.Join(root, ".git ", "x"), authz.DestUnderGit},
		{"a regular file", file, authz.DestNotDir},
		{"symlink leaf to an empty directory", link, authz.DestSymlink},
		{"missing parent", filepath.Join(root, "nope", "x"), authz.DestNoParent},
		{"empty string", "", authz.DestEmpty},
	}
	for _, r := range rows {
		t.Run(r.name, func(t *testing.T) {
			if got := destReason(t, r.dest, cwd); got != r.want {
				t.Fatalf("got %s want %s", got, r.want)
			}
		})
	}
	if got := destReason(t, "x", "relative/cwd"); got != authz.DestEmpty {
		t.Fatalf("relative cwd: %s", got)
	}
}

func TestArtifactGetDest_ParentSymlinkIntoGitIsResolved(t *testing.T) {
	root := scratch(t)
	cwd := filepath.Join(root, "work")
	mustMkdir(t, filepath.Join(cwd, ".git"))
	if err := os.Symlink(filepath.Join(cwd, ".git"), filepath.Join(root, "innocent")); err != nil {
		t.Fatal(err)
	}
	if got := destReason(t, filepath.Join(root, "innocent", "out"), cwd); got != authz.DestUnderGit {
		t.Fatalf("got %s", got)
	}
}

func TestArtifactGetDest_Accepts(t *testing.T) {
	root := scratch(t)
	cwd := filepath.Join(root, "work")
	mustMkdir(t, cwd)
	empty := filepath.Join(root, "empty")
	mustMkdir(t, empty)
	for name, dest := range map[string]string{
		"existing empty directory": empty,
		"fresh path":               filepath.Join(root, "fresh"),
		"fresh relative path":      "out",
		"sibling of cwd":           "../elsewhere",
	} {
		got, err := authz.ArtifactGetDest(dest, cwd)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !filepath.IsAbs(got) {
			t.Fatalf("%s: %q is not absolute", name, got)
		}
		if _, serr := os.Lstat(got); name != "existing empty directory" && serr == nil {
			t.Fatalf("%s: ArtifactGetDest must not create %s", name, got)
		}
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
}
