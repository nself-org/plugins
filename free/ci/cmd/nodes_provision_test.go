package main

// Purpose: the two nodes keys are registered in the subcommand table, are
// reached through run() without touching main.go, and refuse bad argv before
// any host is contacted.

import (
	"io"
	"os"
	"strings"
	"testing"
)

// capture runs fn with stdout and stderr redirected and returns both.
func capture(t *testing.T, fn func() int) (code int, out, errs string) {
	t.Helper()
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	so, se := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wo, we
	defer func() { os.Stdout, os.Stderr = so, se }()
	code = fn()
	wo.Close()
	we.Close()
	bo, _ := io.ReadAll(ro)
	be, _ := io.ReadAll(re)
	return code, string(bo), string(be)
}

func TestNodesKeysRegistered(t *testing.T) {
	for _, key := range []string{"nodes provision", "nodes verify"} {
		if _, ok := subcommands[key]; !ok {
			t.Errorf("subcommand %q is not registered", key)
		}
	}
	h, key, rest, ok := resolve([]string{"nodes", "verify", "--host", "x"})
	if !ok || h == nil || key != "nodes verify" || strings.Join(rest, " ") != "--host x" {
		t.Fatalf("resolve: key=%q rest=%v ok=%v", key, rest, ok)
	}
}

func TestNodesHelpThroughRun(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"nodes", "provision", "--help"}, "nself ci nodes provision [flags]"},
		{[]string{"nodes", "verify", "-h"}, "nself ci nodes verify [flags]"},
	} {
		code, out, errs := capture(t, func() int { return run(tc.args) })
		if code != 0 || errs != "" || !strings.Contains(out, tc.want) {
			t.Fatalf("%v: code=%d out=%q err=%q", tc.args, code, out, errs)
		}
	}
}

func TestNodesRefusalsThroughRun(t *testing.T) {
	t.Setenv("GITHUB_RUNNER_TOKEN", "")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"nodes", "provision"}, "Error: --github-url is required"},
		{[]string{"nodes", "provision", "--github-url", "u"}, "Error: a runner registration token is required"},
		{[]string{"nodes", "provision", "--github-url", "u", "--token", "t", "--host", "a", "--host", "b"}, "Error: provision takes at most one --host"},
		{[]string{"nodes", "provision", "--bogus"}, "Error: unknown flag: --bogus"},
		{[]string{"nodes", "verify", "--bogus"}, "Error: unknown flag: --bogus"},
	} {
		code, out, errs := capture(t, func() int { return run(tc.args) })
		if code != 1 || out != "" || !strings.Contains(errs, tc.want) {
			t.Fatalf("%v: code=%d out=%q err=%q", tc.args, code, out, errs)
		}
	}
}
