package main

import (
	"context"
	"flag"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

func resetSchedFlags(t *testing.T) {
	t.Helper()
	schedFlags = schedpolicy.Flags{}
	t.Cleanup(func() { schedFlags = schedpolicy.Flags{} })
}

// parseOn parses args on a copy-free flag set quietly. The sets are package
// level, so the caller resets schedFlags first.
func parseOn(fs *flag.FlagSet, args ...string) error {
	fs.SetOutput(io.Discard)
	return fs.Parse(args)
}

func TestSchedFlagsRegistered(t *testing.T) {
	for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags} {
		for _, f := range []string{"prefer", "runner", "max-queue"} {
			if fs.Lookup(f) == nil {
				t.Errorf("%s flag set has no --%s", name, f)
			}
		}
	}
}

func TestSchedFlagsParse(t *testing.T) {
	for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags} {
		resetSchedFlags(t)
		if err := parseOn(fs, "--prefer", "local", "--runner", "ms5", "--max-queue", "2m"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		vals, err := schedRevisionValues(false)
		if err != nil {
			t.Fatal(err)
		}
		if string(vals[schedpolicy.KeyPreset]) != `"prefer-local"` || string(vals[schedpolicy.KeyRunner]) != `"ms5"` || string(vals[schedpolicy.KeyMaxQueue]) != `"2m"` || len(vals) != 3 {
			t.Errorf("%s: %v", name, vals)
		}
	}
}

func TestSchedFlagsRefusals(t *testing.T) {
	for _, args := range [][]string{
		{"--prefer", "warp"}, {"--prefer", ""}, {"--max-queue", "soon"}, {"--max-queue", "0s"}, {"--max-queue", "-1m"}, {"--runner", ""},
	} {
		for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags} {
			resetSchedFlags(t)
			err := parseOn(fs, args...)
			if err == nil || !strings.Contains(err.Error(), "E691") {
				t.Errorf("%s %v: %v", name, args, err)
			}
			if schedFlags != (schedpolicy.Flags{}) {
				t.Errorf("%s %v: a refused value was kept: %+v", name, args, schedFlags)
			}
		}
	}
}

// TestSchedFlagsForegroundDefault: a person at a terminal waits 10 minutes at
// most unless told otherwise; a scripted or queued run is not capped.
func TestSchedFlagsForegroundDefault(t *testing.T) {
	resetSchedFlags(t)
	for foreground, want := range map[bool]string{true: `"10m0s"`, false: ""} {
		vals, err := schedRevisionValues(foreground)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(vals[schedpolicy.KeyMaxQueue]); got != want {
			t.Errorf("foreground %v: max_queue %q want %q", foreground, got, want)
		}
	}
	if err := parseOn(bareFlags, "--max-queue", "30s"); err != nil {
		t.Fatal(err)
	}
	if vals, _ := schedRevisionValues(true); string(vals[schedpolicy.KeyMaxQueue]) != `"30s"` {
		t.Errorf("--max-queue lost to the default: %v", vals)
	}
}

// TestInteractiveRun: only a terminal on stdin, no --trigger and no
// NSELF_INVOKED_BY make a run a person waiting; a bare cron run is not.
func TestInteractiveRun(t *testing.T) {
	defer func(f func() bool) { stdinTerminal = f }(stdinTerminal)
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, tc := range []struct {
		name    string
		tty     bool
		trigger string
		env     map[string]string
		want    bool
	}{
		{"terminal", true, "", nil, true},
		{"cron with stdin from /dev/null or a pipe", false, "", nil, false},
		{"terminal with a trigger", true, "push", nil, false},
		{"terminal under a wrapper", true, "", map[string]string{"NSELF_INVOKED_BY": "cam"}, false},
	} {
		stdinTerminal = func() bool { return tc.tty }
		if got := interactiveRun(tc.trigger, env(tc.env)); got != tc.want {
			t.Errorf("%s: interactive %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestSchedFlagsReachPolicy: the flags of a bare run end in the effective
// policy: the options hook fills PolicyRun, the policy hook resolves it, and a
// project-pinned runner beats a different --runner (reported, not applied).
func TestSchedFlagsReachPolicy(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", "-b", "main", root).CombinedOutput(); err != nil {
		t.Fatalf("git init: %s %v", out, err)
	}
	db := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(db, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	project := policy.ProjectIDGitCached(ctx, policy.Runner(report.Capture), root)
	if err = st.ApplyWrite(ctx, store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: schedpolicy.KeyRunner, Value: []byte(`"ms6"`), Actor: "test"}, nil); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	hook := cirun.PolicyHook{OpenStore: func() (*store.Store, error) { return store.Open(db, store.Options{}) }, Capture: report.Capture}
	resolve := func(interactive bool, args ...string) cirun.PolicyResult {
		t.Helper()
		resetSchedFlags(t)
		if err := parseOn(bareFlags, args...); err != nil {
			t.Fatal(err)
		}
		o := RunOptions{Root: root, Interactive: interactive}
		if err := schedOptions(&o); err != nil {
			t.Fatal(err)
		}
		res, err := hook.Resolve(ctx, cirun.PolicyRequest{Root: root, Run: o.PolicyRun})
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	bare := resolve(false)
	if got := resolve(false, "--runner", "ms5"); got.Digest != bare.Digest || len(got.IgnoredRun) != 1 {
		t.Errorf("--runner against a pinned runner: digest same %v, ignored %v", got.Digest == bare.Digest, got.IgnoredRun)
	}
	if got := resolve(false, "--prefer", "local"); got.Digest == bare.Digest || len(got.IgnoredRun) != 0 {
		t.Errorf("--prefer local did not reach the policy: %v", got.IgnoredRun)
	}
	if got := resolve(false, "--max-queue", "5m"); got.Digest == bare.Digest {
		t.Error("--max-queue 5m did not reach the policy")
	}
	if got := resolve(true); got.Digest == bare.Digest {
		t.Error("an interactive run did not get the foreground queue cap")
	}
	if got := resolve(false); got.Digest != bare.Digest {
		t.Error("a scripted run's digest changed")
	}
}

func TestSchedFlagsInHelp(t *testing.T) {
	help := v15Help()
	for _, f := range []string{"-prefer", "-runner", "-max-queue"} {
		if !strings.Contains(help, f) {
			t.Errorf("v15 help does not list %s", f)
		}
	}
}
