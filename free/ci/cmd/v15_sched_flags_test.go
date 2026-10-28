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
	for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags, "explain": explainFlags} {
		for _, f := range []string{"prefer", "runner", "max-queue"} {
			if fs.Lookup(f) == nil {
				t.Errorf("%s flag set has no --%s", name, f)
			}
		}
	}
}

func TestSchedFlagsParse(t *testing.T) {
	for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags, "explain": explainFlags} {
		resetSchedFlags(t)
		if err := parseOn(fs, "--prefer", "local", "--runner", "ms5", "--max-queue", "2m"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		vals, err := schedRevisionValues()
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
		for name, fs := range map[string]*flag.FlagSet{"bare": bareFlags, "plan": planFlags, "explain": explainFlags} {
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

// TestSchedFlagsNoImplicitCap: with no flag a run has no per-run values: the
// foreground queue cap is the dispatcher's default (interactive runs only), not a
// run-scope value that would shorten a longer project limit.
func TestSchedFlagsNoImplicitCap(t *testing.T) {
	resetSchedFlags(t)
	if vals, err := schedRevisionValues(); err != nil || vals != nil {
		t.Fatalf("flagless run values = %v (%v)", vals, err)
	}
	if err := parseOn(bareFlags, "--max-queue", "30s"); err != nil {
		t.Fatal(err)
	}
	if vals, _ := schedRevisionValues(); string(vals[schedpolicy.KeyMaxQueue]) != `"30s"` {
		t.Errorf("--max-queue lost: %v", vals)
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
	// the values themselves, through the same resolver the policy hook builds
	values := func(interactive bool, args ...string) map[string]any {
		t.Helper()
		resetSchedFlags(t)
		if err := parseOn(bareFlags, args...); err != nil {
			t.Fatal(err)
		}
		o := RunOptions{Root: root, Interactive: interactive}
		if err := schedOptions(&o); err != nil {
			t.Fatal(err)
		}
		st, err := store.Open(db, store.Options{})
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = st.Close() }()
		res := policy.Resolver{Store: st, Run: policy.Runner(report.Capture)}
		got, err := res.Resolve(ctx, policy.Repo{Root: root, Project: project, Visibility: "unknown", Class: "owner"}, policy.Revision{Dir: root, Run: o.PolicyRun})
		if err != nil {
			t.Fatal(err)
		}
		return got.Values
	}
	if v := values(false, "--prefer", "local", "--runner", "ms5", "--max-queue", "5m"); v[schedpolicy.KeyPreset] != "prefer-local" || v[schedpolicy.KeyRunner] != "ms6" || v[schedpolicy.KeyMaxQueue] != int64(300000) {
		t.Errorf("per-run values: preset %v runner %v (the project's ms6 must stand) max queue %v", v[schedpolicy.KeyPreset], v[schedpolicy.KeyRunner], v[schedpolicy.KeyMaxQueue])
	}
	// the project's longer limit stands for an interactive run with no flag: no cap is injected
	if err := st2(t, db, project, schedpolicy.KeyMaxQueue, `"1h"`); err != nil {
		t.Fatal(err)
	}
	if v := values(true); v[schedpolicy.KeyMaxQueue] != int64(3600000) {
		t.Errorf("an interactive run shortened the project's 1h limit to %v", v[schedpolicy.KeyMaxQueue])
	}
	bare := resolve(false)
	if got := resolve(false, "--runner", "ms5"); got.Digest != bare.Digest || len(got.IgnoredRun) != 1 {
		t.Errorf("--runner against a pinned runner: digest same %v, ignored %v", got.Digest == bare.Digest, got.IgnoredRun)
	}
	if got := resolve(false, "--prefer", "local"); len(got.IgnoredRun) != 0 {
		t.Errorf("--prefer local was refused: %v", got.IgnoredRun)
	}
	// placement values never enter the digest: evidence verification re-resolves without them
	for _, args := range [][]string{{"--prefer", "local"}, {"--max-queue", "5m"}} {
		if got := resolve(false, args...); got.Digest != bare.Digest {
			t.Errorf("%v changed the digest evidence is verified against", args)
		}
	}
	if got := resolve(true); got.Digest != bare.Digest || len(got.IgnoredRun) != 0 {
		t.Errorf("an interactive flagless run differs from the policy in force: ignored %v", got.IgnoredRun)
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

// st2 writes one project policy row into the store at db.
func st2(t *testing.T, db, project, key, value string) error {
	t.Helper()
	st, err := store.Open(db, store.Options{})
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	return st.ApplyWrite(context.Background(), store.PolicyWrite{Op: store.PolicySet, Scope: "project", Subject: project, Key: key, Value: []byte(value), Actor: "test"}, nil)
}
