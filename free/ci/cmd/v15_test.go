package main

import (
	"errors"
	"flag"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestV15OnlyFixture(t *testing.T) {
	const key = "zz-fixture"
	subcommands[key] = func([]string) int { return 14 }
	registerV15(key, subcommandEntry{Handler: func([]string) int { return 15 }, V15Only: true, Flags: runsFlags})
	defer func() { delete(v15Subcommands, key); delete(subcommands, key) }()
	t.Setenv("NSELF_V15", "1")
	if !compat.V15() {
		t.Fatal("v1.5 fixture not enabled")
	}
	if got := runV15([]string{key}); got != 15 {
		t.Fatalf("v1.5 result %d", got)
	}
	t.Setenv("NSELF_V15", "0")
	if compat.V15() {
		t.Fatal("v1.4 fixture still enabled")
	}
	if got := run([]string{key}); got != 14 {
		t.Fatalf("v1.4 result %d", got)
	}
}

// TestSharedKey: a V15Only entry may share a key with a v1.4 entry and wins
// only under NSELF_V15=1.
func TestSharedKey(t *testing.T) {
	old := subcommands["serve"]
	if old == nil {
		t.Fatal("v1.4 serve key missing")
	}
	subcommands["serve"] = func([]string) int { return 14 }
	registerV15("serve", subcommandEntry{Handler: func([]string) int { return 15 }, V15Only: true, Flags: runsFlags})
	defer func() { delete(v15Subcommands, "serve"); subcommands["serve"] = old }()
	t.Setenv("NSELF_V15", "1")
	if got := runV15([]string{"serve"}); got != 15 {
		t.Fatalf("v1.5 serve = %d", got)
	}
	t.Setenv("NSELF_V15", "0")
	if got := run([]string{"serve"}); got != 14 {
		t.Fatalf("v1.4 serve = %d", got)
	}
}

func TestHiddenIsV15Only(t *testing.T) {
	if p := tableProblems(v15Subcommands); len(p) != 0 {
		t.Fatalf("real table has problems: %v", p)
	}
	bad := map[string]subcommandEntry{"netproxy": {Handler: func([]string) int { return 0 }, Hidden: true, Flags: runsFlags}}
	if p := tableProblems(bad); len(p) != 1 || !strings.Contains(p[0], "Hidden without V15Only") {
		t.Fatalf("Hidden without V15Only not caught: %v", p)
	}
	registerV15("zz-hidden", subcommandEntry{Handler: func([]string) int { return 16 }, V15Only: true, Hidden: true, Flags: runsFlags})
	defer delete(v15Subcommands, "zz-hidden")
	t.Setenv("NSELF_V15", "1")
	if got := runV15([]string{"zz-hidden"}); got != 16 {
		t.Fatalf("hidden entry did not run: %d", got)
	}
	for _, k := range visibleV15() {
		if k == "zz-hidden" {
			t.Fatal("hidden entry counted")
		}
	}
	if strings.Contains(v15Help(), "zz-hidden") {
		t.Fatal("hidden entry in --help")
	}
}

func TestEntriesHaveFlags(t *testing.T) {
	for k, e := range v15Subcommands {
		if e.Flags == nil || e.Flags() == nil {
			t.Errorf("%s has no Flags constructor", k)
		}
	}
	for _, k := range []string{"runs", "plan", "config show"} {
		if _, ok := v15Subcommands[k]; !ok {
			t.Errorf("%s not registered", k)
		}
	}
}

// The ad-hoc job is the bare form `nself ci -- <cmd>`: it adds flags, never a first-level subcommand.
func TestHelpHasNoAdhocSubcommand(t *testing.T) {
	h := v15Help()
	for _, l := range strings.Split(h, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "adhoc") {
			t.Fatalf("first-level adhoc listed: %q", l)
		}
	}
	for _, want := range []string{"-isolation", "-image"} {
		if !strings.Contains(h, want) {
			t.Errorf("help lacks %s", want)
		}
	}
	if _, ok := subcommands["adhoc"]; ok {
		t.Fatal("adhoc registered as a subcommand")
	}
}

func TestHelpHasNoApprove(t *testing.T) {
	h := v15Help()
	for _, l := range strings.Split(h, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "approve") {
			t.Fatalf("first-level approve listed: %q", l)
		}
	}
	for _, want := range []string{"-pipeline", "-no-status", "runs"} {
		if !strings.Contains(h, want) {
			t.Errorf("help lacks %s", want)
		}
	}
}

func TestBareRefusals(t *testing.T) {
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	for name, args := range map[string][]string{
		"sha":       {"--sha", "abc123"},
		"trust":     {"--trust", "owner"},
		"network":   {"--network", "bogus"},
		"bad flag":  {"--nope"},
		"neg time":  {"--timeout=-1s"},
		"job+adhoc": {"--job", "x", "--", "echo"},
	} {
		if got := runV15(args); got != 1 {
			t.Errorf("%s: exit %d, want 1", name, got)
		}
	}
}

// D-1021: `nself ci --` with nothing after the dashes is a usage error, not the full gate.
func TestBareTrailingDashesIsUsage(t *testing.T) {
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	for _, args := range [][]string{{"--"}, {"--pipeline", "deep", "--"}} {
		if _, err := parseBare(args); err == nil || cirun.CodeOf(err) != cirun.CodeOptions {
			t.Errorf("parseBare(%v) = %v, want an options error", args, err)
		}
	}
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_CI_HOME", t.TempDir())
	if got := runV15([]string{"--"}); got != 1 {
		t.Errorf("exit %d, want 1", got)
	}
}

func TestOptionsHook(t *testing.T) {
	saved := optionsHooks
	defer func() { optionsHooks = saved }()
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	RegisterOptionsHook(func(o *RunOptions) error { o.NoStatus = true; o.Pipeline = "fast"; return nil })
	o, err := parseBare([]string{"--pipeline", "deep"})
	if err != nil || !o.NoStatus || o.Pipeline != "fast" {
		t.Fatalf("hook not applied after parsing: %+v %v", o, err)
	}
	boom := errors.New("E627: hook says no")
	RegisterOptionsHook(func(*RunOptions) error { return boom })
	if _, err = parseBare(nil); !errors.Is(err, boom) {
		t.Fatalf("hook error lost: %v", err)
	}
	optionsHooks = nil
	o, err = parseBare([]string{"--service", "pg", "--service", "redis", "--", "go", "version"})
	if err != nil || len(o.Services) != 2 || strings.Join(o.Adhoc, " ") != "go version" {
		t.Fatalf("services/adhoc: %+v %v", o, err)
	}
	if _, err = parseBare([]string{"--help"}); !errors.Is(err, flag.ErrHelp) && !bare.help {
		t.Fatalf("help: %v", err)
	}
}
