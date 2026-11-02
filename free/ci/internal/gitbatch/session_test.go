//go:build !windows

package gitbatch

import (
	"context"
	"errors"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// gitIn runs real git in dir for fixture setup and for the expected answers.
func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := osexec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func fixtureRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "init", "-q", "-b", "main")
	for p, body := range map[string]string{"top.txt": "top\n", ".nself/ci.yaml": "version: 1\n", "a/b/c.txt": "deep\n"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "run.sh"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("top.txt", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "fixture")
	return dir
}

func newTestSession(t *testing.T, root string, opt Options) *Session {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	s := NewSession(ctx, root, opt)
	t.Cleanup(func() { s.Close(); cancel() })
	return s
}

func testSet(t *testing.T) *Set {
	t.Helper()
	set := NewSet(context.Background(), Options{})
	t.Cleanup(set.Close)
	return set
}

func TestStatAndReadMatchGit(t *testing.T) {
	dir := fixtureRepo(t)
	s := newTestSession(t, dir, Options{})
	ctx := context.Background()
	obj, err := s.Stat(ctx, "HEAD")
	if err != nil || obj.OID != gitIn(t, dir, "rev-parse", "HEAD") || obj.Type != "commit" {
		t.Fatalf("Stat(HEAD) = %+v, %v", obj, err)
	}
	blob, data, err := s.Read(ctx, "HEAD:top.txt")
	if err != nil || string(data) != "top\n" || blob.Type != "blob" || blob.Size != 4 {
		t.Fatalf("Read = %+v %q %v", blob, data, err)
	}
	if _, err := s.Stat(ctx, "refs/remotes/origin/nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing ref is ErrNotFound, got %v", err)
	}
	if _, _, err := s.Read(ctx, strings.Repeat("0", 40)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("a missing object is ErrNotFound, got %v", err)
	}
	if _, err := s.Stat(ctx, "HEAD\nHEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a name with a newline would break the framing: %v", err)
	}
	if _, err := s.Stat(ctx, "HEAD"); err != nil { // a miss does not end the session
		t.Fatalf("session after a miss: %v", err)
	}
}

func TestTreeEntryMatchesLsTree(t *testing.T) {
	dir := fixtureRepo(t)
	ctx := context.Background()
	s := newTestSession(t, dir, Options{})
	for _, rel := range []string{"top.txt", ".nself/ci.yaml", "a/b/c.txt", "run.sh", "link", "a", "a/b"} {
		want := strings.Fields(strings.SplitN(gitIn(t, dir, "ls-tree", "HEAD", "--", rel), "\t", 2)[0])
		e, found, err := s.TreeEntry(ctx, "HEAD", rel)
		if err != nil || !found || len(want) != 3 || e.Mode != want[0] || e.Type != want[1] || e.OID != want[2] {
			t.Fatalf("%s: got %+v found=%v err=%v, ls-tree says %v", rel, e, found, err, want)
		}
	}
	for _, rel := range []string{"nope", "top.txt/inside", "a/missing/x", ".nself/other.yaml"} {
		if _, found, err := s.TreeEntry(ctx, "HEAD", rel); found || err != nil {
			t.Fatalf("%s: found=%v err=%v, want absent without error", rel, found, err)
		}
	}
	for _, rel := range []string{"", "/abs", "a/../top.txt", "a//b", "./top.txt", "a/", `a\b`} {
		if _, _, err := s.TreeEntry(ctx, "HEAD", rel); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("%q must be left to git, got %v", rel, err)
		}
	}
	// The root may be a subdirectory: paths are relative to it, as `git -C sub ls-tree` reads them.
	sub := newTestSession(t, filepath.Join(dir, "a"), Options{})
	if e, found, err := sub.TreeEntry(ctx, "HEAD", "b/c.txt"); err != nil || !found || e.Type != "blob" {
		t.Fatalf("subdirectory root: %+v %v %v", e, found, err)
	}
}

func TestSessionSeesNewCommitsAndRefs(t *testing.T) {
	dir := fixtureRepo(t)
	s := newTestSession(t, dir, Options{})
	ctx := context.Background()
	first, err := s.Stat(ctx, "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "later.txt"), []byte("later\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "second")
	gitIn(t, dir, "update-ref", "refs/remotes/origin/main", "HEAD")
	second, err := s.Stat(ctx, "HEAD")
	if err != nil || second.OID == first.OID || second.OID != gitIn(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("a long-lived session must see the new commit: %+v %v (first %s)", second, err, first.OID)
	}
	ref, err := s.Stat(ctx, "refs/remotes/origin/main^{commit}")
	if err != nil || ref.OID != second.OID {
		t.Fatalf("a ref written after the session started: %+v %v", ref, err)
	}
}

func TestAdaptersFailClosedOnMissingObjects(t *testing.T) {
	dir := fixtureRepo(t)
	ctx := WithSet(context.Background(), testSet(t))
	if oid, ok, used := RevParse(ctx, dir, "HEAD"); !used || !ok || oid != gitIn(t, dir, "rev-parse", "HEAD") {
		t.Fatalf("HEAD: %s %v %v", oid, ok, used)
	}
	if _, ok, used := RevParse(ctx, dir, "no-such-ref"); !used || ok {
		t.Fatalf("a missing ref is a failed read, not a fallback: ok=%v used=%v", ok, used)
	}
	if _, ok, used := ObjectType(ctx, dir, strings.Repeat("1", 40)); !used || ok {
		t.Fatalf("missing object type: ok=%v used=%v", ok, used)
	}
	if _, ok, used := ReadBlob(ctx, dir, strings.Repeat("1", 40)); !used || ok {
		t.Fatalf("missing blob: ok=%v used=%v", ok, used)
	}
	if _, ok, used := ReadBlob(ctx, dir, "HEAD"); !used || ok {
		t.Fatalf("a commit is not a blob: ok=%v used=%v", ok, used)
	}
	if _, _, ok, used := LsTree(ctx, dir, strings.Repeat("1", 40), "top.txt"); !used || ok {
		t.Fatalf("ls-tree of a missing commit must fail: ok=%v used=%v", ok, used)
	}
	if _, found, ok, used := LsTree(ctx, dir, "HEAD", "nope"); !used || !ok || found {
		t.Fatalf("an absent path is not a failure: found=%v ok=%v used=%v", found, ok, used)
	}
	if _, _, used := RevParse(context.Background(), dir, "HEAD"); used {
		t.Fatal("no set in the context: the caller runs git")
	}
}

func TestNotARepositoryIsUnavailable(t *testing.T) {
	s := newTestSession(t, t.TempDir(), Options{ReadTimeout: 5 * time.Second})
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("a git that never answers sends the caller to the real git: %v", err)
	}
	if _, err := s.Stat(context.Background(), "HEAD"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("and the session is not relaunched on every read: %v", err)
	}
}

func TestFindEntryRejectsMalformedTrees(t *testing.T) {
	for name, tree := range map[string][]byte{
		"no mode":      []byte(" name\x00" + strings.Repeat("a", 20)),
		"no nul":       []byte("100644 name"),
		"short id":     []byte("100644 name\x00abc"),
		"unknown mode": []byte("100666 name\x00" + strings.Repeat("a", 20)),
	} {
		if _, _, err := findEntry(tree, "name", 20); !errors.Is(err, ErrMalformed) {
			t.Fatalf("%s: want ErrMalformed, got %v", name, err)
		}
	}
	if _, found, err := findEntry([]byte("100644 other\x00"+strings.Repeat("a", 20)), "name", 20); found || err != nil {
		t.Fatalf("absent name: %v %v", found, err)
	}
}
