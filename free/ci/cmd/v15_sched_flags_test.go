package main

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
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
		vals, err := schedRevisionValues("")
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
// most unless told otherwise; a queued (scheduled or api) run is not capped.
func TestSchedFlagsForegroundDefault(t *testing.T) {
	resetSchedFlags(t)
	for trigger, want := range map[string]string{"": `"10m0s"`, "local": `"10m0s"`, "manual": `"10m0s"`, "schedule": "", "api": "", "push": ""} {
		vals, err := schedRevisionValues(trigger)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(vals[schedpolicy.KeyMaxQueue]); got != want {
			t.Errorf("trigger %q: max_queue %q want %q", trigger, got, want)
		}
	}
	if err := parseOn(bareFlags, "--max-queue", "30s"); err != nil {
		t.Fatal(err)
	}
	if vals, _ := schedRevisionValues(""); string(vals[schedpolicy.KeyMaxQueue]) != `"30s"` {
		t.Errorf("--max-queue lost to the default: %v", vals)
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

// D-0984: the flags of a bare run reach RunOptions.PolicyValues, and a later
// parse does not inherit them.
func TestParseBareFillsPolicyValues(t *testing.T) {
	resetSchedFlags(t)
	t.Setenv("NSELF_CI_REPO", t.TempDir())
	o, err := parseBare([]string{"--prefer", "local", "--runner", "ms5", "--max-queue", "2m"})
	if err != nil {
		t.Fatal(err)
	}
	v := o.PolicyValues
	if string(v[schedpolicy.KeyPreset]) != `"prefer-local"` || string(v[schedpolicy.KeyRunner]) != `"ms5"` || string(v[schedpolicy.KeyMaxQueue]) != `"2m"` || len(v) != 3 {
		t.Fatalf("policy values: %v", v)
	}
	o, err = parseBare(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(o.PolicyValues) != 1 || string(o.PolicyValues[schedpolicy.KeyMaxQueue]) != `"10m0s"` {
		t.Fatalf("a bare foreground run gets only the 10 minute queue cap, got %v", o.PolicyValues)
	}
	o, err = parseBare([]string{"--trigger", "push"})
	if err != nil || len(o.PolicyValues) != 0 {
		t.Fatalf("a queued run has no default cap: %v %v", o.PolicyValues, err)
	}
}
