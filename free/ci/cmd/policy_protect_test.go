package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeRun answers by substring of the joined argv, first match wins.
type fakeRun struct {
	replies [][3]string // match, out, exit ("0" or "1")
	calls   []string
}

func (f *fakeRun) run(_ context.Context, argv ...string) (string, int, error) {
	line := strings.Join(argv, " ")
	f.calls = append(f.calls, line)
	for _, r := range f.replies {
		if strings.Contains(line, r[0]) {
			if r[2] == "1" {
				return r[1], 1, nil
			}
			return r[1], 0, nil
		}
	}
	return "no recorded reply: " + line, 1, nil
}

func protectFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "internal", "protect", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func newProtectEnv(t *testing.T, f *fakeRun) (protectEnv, *bytes.Buffer) {
	var out bytes.Buffer
	return protectEnv{run: f.run, out: &out, root: t.TempDir(), home: t.TempDir(),
		now: func() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }}, &out
}

const protPath = "repos/example-org/example-repo/branches/main/protection"

func TestPolicyProtectDryRunChangesNothing(t *testing.T) {
	f := &fakeRun{replies: [][3]string{{protPath, protectFixture(t, "live_full.json"), "0"}}}
	env, out := newProtectEnv(t, f)
	code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", dryRun: true, appID: 15368})
	if code != 0 {
		t.Fatalf("exit = %d, out = %s", code, out)
	}
	if !strings.Contains(out.String(), "would add required check nself-ci (app 15368)") || !strings.Contains(out.String(), `"context": "nself-ci"`) {
		t.Fatalf("out = %s", out)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "PUT") {
			t.Fatalf("a dry run wrote: %s", c)
		}
	}
	if entries, _ := os.ReadDir(env.home); len(entries) != 0 {
		t.Fatalf("a dry run created files: %v", entries)
	}
}

func TestPolicyProtectApplyWritesSnapshotFirst(t *testing.T) {
	f := &fakeRun{replies: [][3]string{{"--method PUT", "{}", "0"}, {protPath, protectFixture(t, "live_already.json"), "0"}}}
	// First GET must be the pre-state: serve it once, then the post-state.
	pre := protectFixture(t, "live_full.json")
	n := 0
	env, out := newProtectEnv(t, f)
	env.run = func(ctx context.Context, argv ...string) (string, int, error) {
		if strings.HasSuffix(strings.Join(argv, " "), protPath) {
			if n++; n == 1 {
				return pre, 0, nil
			}
		}
		return f.run(ctx, argv...)
	}
	code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", json: true, appID: 15368})
	if code != 0 {
		t.Fatalf("exit = %d, out = %s", code, out)
	}
	var env1 struct {
		Data protectResult `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &env1); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if env1.Data.Snapshot == "" || len(env1.Data.Added) != 1 {
		t.Fatalf("data = %+v", env1.Data)
	}
	if _, err := os.Stat(env1.Data.Snapshot); err != nil || !strings.HasPrefix(env1.Data.Snapshot, filepath.Join(env.home, "protect")) {
		t.Fatalf("snapshot %q: %v", env1.Data.Snapshot, err)
	}
}

func TestPolicyProtect403ExitsThreeWithEquivalent(t *testing.T) {
	f := &fakeRun{replies: [][3]string{{protPath, "gh: Upgrade to GitHub Pro or make this repository public to enable this feature. (HTTP 403)", "1"}}}
	env, out := newProtectEnv(t, f)
	stderr := captureStderr(t, func() {
		if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", appID: 15368}); code != 3 {
			t.Fatalf("exit = %d, want 3", code)
		}
	})
	if !strings.Contains(stderr, "E643") || !strings.Contains(stderr, "--hook") || !strings.Contains(stderr, "--audit") {
		t.Fatalf("stderr = %s", stderr)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "PUT") {
			t.Fatalf("no write may follow a 403: %s", c)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %s", out)
	}
}

func TestPolicyProtectAlreadyRequiredIsNoop(t *testing.T) {
	f := &fakeRun{replies: [][3]string{{protPath, protectFixture(t, "live_already.json"), "0"}}}
	env, out := newProtectEnv(t, f)
	if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", appID: 15368}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "already required") {
		t.Fatalf("out = %s", out)
	}
	if entries, _ := os.ReadDir(env.home); len(entries) != 0 {
		t.Fatalf("no-op wrote files: %v", entries)
	}
}

func TestPolicyProtectHookAndExisting(t *testing.T) {
	root := t.TempDir()
	f := &fakeRun{replies: [][3]string{
		{"symbolic-ref", "origin/trunk\n", "0"},
		{"rev-parse --git-path hooks/pre-push", ".git/hooks/pre-push\n", "0"},
	}}
	env, out := newProtectEnv(t, f)
	env.root = root
	if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", hook: true, appID: 15368}); code != 0 {
		t.Fatalf("exit = %d, out = %s", code, out)
	}
	body, err := os.ReadFile(filepath.Join(root, ".git", "hooks", "pre-push"))
	if err != nil || !strings.Contains(string(body), `branch="trunk"`) {
		t.Fatalf("hook = %q %v", body, err)
	}
	// An existing foreign hook: E642, exit 1, untouched.
	foreign := filepath.Join(root, ".git", "hooks", "pre-push")
	_ = os.WriteFile(foreign, []byte("#!/bin/sh\necho mine\n"), 0o755)
	stderr := captureStderr(t, func() {
		if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", hook: true, appID: 15368}); code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
	})
	if !strings.Contains(stderr, "E642") {
		t.Fatalf("stderr = %s", stderr)
	}
	if got, _ := os.ReadFile(foreign); string(got) != "#!/bin/sh\necho mine\n" {
		t.Fatalf("hook overwritten: %q", got)
	}
}

func TestPolicyProtectAuditExit(t *testing.T) {
	f := &fakeRun{replies: [][3]string{
		{"--paginate", "aaaa\tone\nbbbb\ttwo", "0"},
		{"commits/aaaa/status", `{"statuses":[{"context":"nself-ci","state":"success"}]}`, "0"},
		{"commits/bbbb/status", `{"statuses":[]}`, "0"},
		{"commits/bbbb/check-runs", `{"check_runs":[]}`, "0"},
	}}
	env, out := newProtectEnv(t, f)
	if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", audit: 7, auditSet: true, appID: 15368}); code != 1 {
		t.Fatalf("exit = %d, want 1 (a commit has no green nself-ci)", code)
	}
	if !strings.Contains(out.String(), "bbbb") || strings.Contains(out.String(), "  aaaa") || !strings.Contains(out.String(), "1 without a green nself-ci") {
		t.Fatalf("out = %s", out)
	}
}

func TestPolicyProtectUsage(t *testing.T) {
	for _, args := range [][]string{{"--hook", "--audit", "3"}, {"extra"}, {"--audit", "-1"}, {"--audit", "0"}, {"--restore", ""}, {"--hook", "--dry-run"}, {"--app-id", "0"}} {
		var code int
		captureStderr(t, func() { code = runPolicyProtect(args) })
		if code != 1 {
			t.Fatalf("%v: exit = %d, want 1", args, code)
		}
	}
}

func TestPolicyProtectRegisteredV15Only(t *testing.T) {
	e, ok := v15Subcommands["policy protect"]
	if !ok || !e.V15Only || e.Flags == nil {
		t.Fatalf("entry = %+v", e)
	}
	if _, ok := subcommands["policy protect"]; ok {
		t.Fatal("a v1.4 argv must not reach the new command")
	}
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	var b bytes.Buffer
	_, _ = b.ReadFrom(r)
	return b.String()
}

// TestPolicyProtectHookNeverGuessesTheBranch: no --branch, no origin/HEAD and no
// answer from GitHub is an error, not a hook for "main".
func TestPolicyProtectHookNeverGuessesTheBranch(t *testing.T) {
	root := t.TempDir()
	f := &fakeRun{replies: [][3]string{{"rev-parse --git-path hooks/pre-push", ".git/hooks/pre-push\n", "0"}}}
	env, _ := newProtectEnv(t, f)
	env.root = root
	stderr := captureStderr(t, func() {
		if code := env.dispatch(context.Background(), protectFlags{hook: true, appID: 15368}); code == 0 {
			t.Fatal("exit 0 for a hook whose branch is unknown")
		}
	})
	if !strings.Contains(stderr, "--branch") {
		t.Fatalf("stderr = %s", stderr)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "hooks", "pre-push")); err == nil {
		t.Fatal("a hook was written for a guessed branch")
	}
	// With --branch the hook needs neither a remote nor GitHub.
	if code := env.dispatch(context.Background(), protectFlags{hook: true, branch: "master", appID: 15368}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	body, _ := os.ReadFile(filepath.Join(root, ".git", "hooks", "pre-push"))
	if !strings.Contains(string(body), `branch="master"`) {
		t.Fatalf("hook = %s", body)
	}
}

// TestPolicyProtectRestoreNeedsNoRemote: the snapshot names its repository.
func TestPolicyProtectRestoreNeedsNoRemote(t *testing.T) {
	live := protectFixture(t, "live_full.json")
	f := &fakeRun{replies: [][3]string{{"--method PUT", "{}", "0"}, {protPath, live, "0"}}}
	env, out := newProtectEnv(t, f)
	snapPath := filepath.Join(t.TempDir(), "s.json")
	data, _ := json.Marshal(map[string]any{"repo": "example-org/example-repo", "branch": "main", "taken_at": "x", "protection": json.RawMessage(live)})
	_ = os.WriteFile(snapPath, data, 0o600)
	if code := env.dispatch(context.Background(), protectFlags{restore: snapPath, restoreSet: true, appID: 15368}); code != 0 {
		t.Fatalf("exit = %d, out = %s", code, out)
	}
	for _, c := range f.calls {
		if strings.Contains(c, "remote get-url") {
			t.Fatalf("restore must not read the remote: %s", c)
		}
	}
	if !strings.Contains(out.String(), "what it replaced is saved to") {
		t.Fatalf("out = %s", out)
	}
}

// TestPolicyProtectFailedApplyPointsAtTheSnapshot: when the PUT does not verify,
// the error says where the previous protection is and how to put it back.
func TestPolicyProtectFailedApplyPointsAtTheSnapshot(t *testing.T) {
	live := protectFixture(t, "live_full.json")
	f := &fakeRun{replies: [][3]string{{"--method PUT", "{}", "0"}, {protPath, live, "0"}}} // the read-back still shows the old protection
	env, _ := newProtectEnv(t, f)
	stderr := captureStderr(t, func() {
		if code := env.dispatch(context.Background(), protectFlags{repo: "example-org/example-repo", branch: "main", appID: 15368}); code == 0 {
			t.Fatal("a PUT that did not land must not exit 0")
		}
	})
	if !strings.Contains(stderr, "--restore") || !strings.Contains(stderr, env.home) {
		t.Fatalf("stderr = %s", stderr)
	}
}
