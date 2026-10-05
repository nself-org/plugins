package main

// Purpose: pin the core-flag translation table (gate_args.go) and the frozen
// 1.4.x argv. legacyBuildCIArgs is a verbatim copy of core's buildCIArgs at the
// branch point (cli cmd/commands/ci.go), so a table change that breaks a
// deployed 1.4.x CLI fails here.
// Inputs:  argv slices. Outputs: parsed gateOpts. No network, no subprocess.

import (
	"flag"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// legacyArgsInput mirrors core's ciArgsInput.
type legacyArgsInput struct {
	checkMode, noStatus, noGitleaks, filesystem, verbose bool
	sha, owner, repo, repoRoot                           string
}

// legacyBuildCIArgs is core's buildCIArgs, copied unchanged.
func legacyBuildCIArgs(in legacyArgsInput) []string {
	ciArgs := []string{}
	if in.checkMode || in.noStatus {
		ciArgs = append(ciArgs, "--no-status")
	}
	if in.noGitleaks {
		ciArgs = append(ciArgs, "--no-gitleaks")
	}
	if in.filesystem {
		ciArgs = append(ciArgs, "--filesystem")
	}
	if in.verbose {
		ciArgs = append(ciArgs, "-v")
	}
	if in.sha != "" {
		ciArgs = append(ciArgs, "--sha", in.sha)
	}
	if in.owner != "" {
		ciArgs = append(ciArgs, "--owner", in.owner)
	}
	if in.repo != "" {
		ciArgs = append(ciArgs, "--repo", in.repo)
	}
	ciArgs = append(ciArgs, in.repoRoot)
	return ciArgs
}

func parse(t *testing.T, args ...string) gateOpts {
	t.Helper()
	t.Setenv("NSELF_CI_REPO", "")
	t.Setenv("NSELF_CI_SKIP_STATUS", "")
	o, err := parseGateArgs(args, flag.ContinueOnError)
	if err != nil {
		t.Fatalf("parse %v: %v", args, err)
	}
	return o
}

// TestFlagTranslationTable covers each row of the table in gate_args.go.
func TestFlagTranslationTable(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		name string
		args []string
		want func(o gateOpts) bool
	}{
		{"check", []string{"--check", root}, func(o gateOpts) bool { return o.check && !o.postStatus() && o.frame() }},
		{"no-status", []string{"--no-status", root}, func(o gateOpts) bool { return o.skipStatus && !o.postStatus() && !o.frame() }},
		{"no-gitleaks", []string{"--no-gitleaks", root}, func(o gateOpts) bool { return o.skipGitleaks }},
		{"filesystem", []string{"--filesystem", root}, func(o gateOpts) bool { return o.forceFilesystem }},
		{"short verbose", []string{"-v", root}, func(o gateOpts) bool { return o.verbose }},
		{"long verbose", []string{"--verbose", root}, func(o gateOpts) bool { return o.verbose }},
		{"sha", []string{"--sha", "abc1234", root}, func(o gateOpts) bool { return o.sha == "abc1234" }},
		{"owner", []string{"--owner", "nself-org", root}, func(o gateOpts) bool { return o.owner == "nself-org" }},
		{"repo", []string{"--repo", "plugins", root}, func(o gateOpts) bool { return o.repo == "plugins" }},
		{"env and gateway", []string{"--env", "local", "--gateway", "http://h:1", root}, func(o gateOpts) bool { return o.env == "local" && o.gateway == "http://h:1" }},
		{"repo-root first then flag (cobra order)", []string{root, "--check"}, func(o gateOpts) bool { return o.check && o.repoRoot == root }},
		{"default posts status", []string{root}, func(o gateOpts) bool { return o.postStatus() && !o.frame() }},
	}
	for _, c := range cases {
		o := parse(t, c.args...)
		if !c.want(o) {
			t.Errorf("%s: %v parsed to %+v", c.name, c.args, o)
		}
	}
}

// TestRepoRootIsAbsolute mirrors core's filepath.Abs and the "." default.
func TestRepoRootIsAbsolute(t *testing.T) {
	o := parse(t)
	if !filepath.IsAbs(o.repoRoot) {
		t.Fatalf("default root %q not absolute", o.repoRoot)
	}
	o = parse(t, "rel/dir")
	if !filepath.IsAbs(o.repoRoot) {
		t.Fatalf("relative root %q not absolute", o.repoRoot)
	}
}

// TestEnvFallbacksUnchanged keeps NSELF_CI_REPO and NSELF_CI_SKIP_STATUS working.
func TestEnvFallbacksUnchanged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NSELF_CI_REPO", dir)
	t.Setenv("NSELF_CI_SKIP_STATUS", "1")
	o, err := parseGateArgs(nil, flag.ContinueOnError)
	if err != nil || o.repoRoot != dir || o.postStatus() {
		t.Fatalf("env fallbacks: %+v err=%v", o, err)
	}
}

// TestFrozenLegacyArgv feeds every flag combination the 1.4.x proxy can send
// and requires the option each flag stands for, never --check framing.
func TestFrozenLegacyArgv(t *testing.T) {
	root := t.TempDir()
	bools := []bool{false, true}
	for _, noStatus := range bools {
		for _, noGit := range bools {
			for _, fsys := range bools {
				for _, verbose := range bools {
					for _, sha := range []string{"", "abc1234"} {
						for _, owner := range []string{"", "nself-org"} {
							for _, repo := range []string{"", "cli"} {
								in := legacyArgsInput{checkMode: noStatus, noGitleaks: noGit, filesystem: fsys, verbose: verbose, sha: sha, owner: owner, repo: repo, repoRoot: root}
								argv := legacyBuildCIArgs(in)
								o := parse(t, argv...)
								ok := o.skipStatus == noStatus && o.skipGitleaks == noGit && o.forceFilesystem == fsys &&
									o.verbose == verbose && o.sha == sha && o.owner == owner && o.repo == repo &&
									o.repoRoot == root && !o.check && !o.frame()
								if !ok {
									t.Fatalf("legacy argv %v parsed to %+v", argv, o)
								}
							}
						}
					}
				}
			}
		}
	}
}

// TestUnknownFlagIsAnError keeps flag errors visible to the caller.
func TestUnknownFlagIsAnError(t *testing.T) {
	if _, err := parseGateArgs([]string{"--nope"}, flag.ContinueOnError); err == nil {
		t.Fatal("unknown flag accepted")
	}
}

// TestWantsHelp covers the help detection used before dispatch.
func TestWantsHelp(t *testing.T) {
	for args, want := range map[string]bool{"--help": true, "-h": true, "--check": false, "": false} {
		var a []string
		if args != "" {
			a = []string{args}
		}
		if got := wantsHelp(a); got != want {
			t.Errorf("wantsHelp(%v) = %v", a, got)
		}
	}
	if wantsHelp([]string{"--", "--help"}) {
		t.Error("help after -- must be positional")
	}
}

// TestFrameLines pins the bytes core printed around the gate.
func TestFrameLines(t *testing.T) {
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	o := gateOpts{check: true, repoRoot: "/r"}
	frameHeader(o)
	frameFailure()
	os.Stdout, os.Stderr = so, se
	_ = wo.Close()
	_ = we.Close()
	out, _ := io.ReadAll(ro)
	errOut, _ := io.ReadAll(re)
	if want := "\n\u2192 nself-ci gate\n\u2139 repo: /r (check mode \u2014 no status posted)\n"; string(out) != want {
		t.Errorf("header = %q, want %q", out, want)
	}
	if string(errOut) != "Error: gate failed\n" {
		t.Errorf("failure = %q", errOut)
	}
}
