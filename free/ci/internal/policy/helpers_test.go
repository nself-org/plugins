package policy

// Purpose: git fixtures and a real-process Runner for the policy tests.
// Inputs: temp dirs. Outputs: a bare origin, a clone and helpers to edit both.
// Constraints: tests only; a missing git is a t.Fatal, never a skip.

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func testRunner(ctx context.Context, argv ...string) (string, int, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), ee.ExitCode(), nil
	}
	if err != nil {
		return out.String(), -1, err
	}
	return out.String(), 0, nil
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, exit, err := testRunner(context.Background(), append([]string{"git", "-C", dir}, args...)...)
	if err != nil || exit != 0 {
		t.Fatalf("git %v in %s: exit %d err %v\n%s", args, dir, exit, err, out)
	}
	return strings.TrimSpace(out)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fixture is a bare origin with a clone whose main starts with the given policy
// block ("" = a config with no policy).
type fixture struct {
	t            *testing.T
	origin, work string
	other        string // a second clone that pushes policy changes
	runner       Runner
}

const baseConfig = "version: 1\n"

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, origin: filepath.Join(root, "origin.git"), work: filepath.Join(root, "work"), other: filepath.Join(root, "other"), runner: testRunner}
	git(t, root, "init", "-q", "--bare", "-b", "main", f.origin)
	git(t, root, "init", "-q", "-b", "main", f.work)
	git(t, f.work, "remote", "add", "origin", f.origin)
	write(t, filepath.Join(f.work, ".nself/ci.yaml"), baseConfig)
	git(t, f.work, "add", ".")
	git(t, f.work, "commit", "-q", "-m", "base")
	git(t, f.work, "push", "-q", "origin", "main")
	return f
}

// ensureOther clones origin the first time a test needs to push as someone else.
func (f *fixture) ensureOther() {
	if _, err := os.Stat(f.other); err != nil {
		git(f.t, filepath.Dir(f.other), "clone", "-q", f.origin, f.other)
	}
}

// push commits a new .nself/ci.yaml from the second clone to origin main and
// returns the new commit.
func (f *fixture) push(config string) string {
	f.t.Helper()
	f.ensureOther()
	write(f.t, filepath.Join(f.other, ".nself/ci.yaml"), config)
	git(f.t, f.other, "commit", "-q", "-am", "policy")
	git(f.t, f.other, "push", "-q", "origin", "main")
	return git(f.t, f.other, "rev-parse", "HEAD")
}

func (f *fixture) setWorktree(config string) {
	f.t.Helper()
	write(f.t, filepath.Join(f.work, ".nself/ci.yaml"), config)
}

func testStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func put(t *testing.T, s *store.Store, scope, subject, key, value string) {
	t.Helper()
	err := s.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: scope, Subject: subject, Key: key, Value: []byte(value), Actor: "test"}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// testKeys is the trust keys plus one Spend key and one SCHED-style family.
func testKeys(t *testing.T) *Keyset {
	t.Helper()
	k := NewKeyset()
	specs := append(trustKeys(),
		KeySpec{Key: "economics.paid_enabled", Owner: "sched", Kind: TightenLower, Order: []string{"off", "capped", "on"}, Default: "on", Spend: true, Decode: orderDecoder(TightenLower, []string{"off", "capped", "on"})},
		KeySpec{Key: "sched.*", Owner: "sched", Kind: TightenAny, Decode: decodeOne},
	)
	for _, s := range specs {
		if err := k.Register(s); err != nil {
			t.Fatal(err)
		}
	}
	return k
}

func ignoredKeys(r Resolved) map[string]string {
	out := map[string]string{}
	for _, ig := range r.Ignored {
		out[ig.Key] = ig.Reason
	}
	return out
}
