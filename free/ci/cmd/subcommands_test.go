package main

// Purpose: test the subcommand table: registration, longest match, the
// default gate fallthrough and the core flags of build, forgejo and serve.

import (
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/surface"
)

// TestRegisteredSubcommands proves the four filed subcommands are in the table.
func TestRegisteredSubcommands(t *testing.T) {
	for _, k := range []string{"run", "build", "forgejo", "serve"} {
		if _, ok := subcommands[k]; !ok {
			t.Errorf("subcommand %q not registered", k)
		}
	}
}

// TestLongestMatchWins registers a two-token key next to a one-token key.
func TestLongestMatchWins(t *testing.T) {
	saved, savedMax := subcommands, maxKeyTokens
	defer func() { subcommands, maxKeyTokens = saved, savedMax }()
	subcommands = map[string]handler{}
	maxKeyTokens = 1
	register("nodes", func([]string) int { return 1 })
	register("nodes provision", func([]string) int { return 2 })
	h, key, rest, ok := resolve([]string{"nodes", "provision", "--x"})
	if !ok || key != "nodes provision" || h(nil) != 2 || !reflect.DeepEqual(rest, []string{"--x"}) {
		t.Fatalf("resolve = %q ok=%v rest=%v", key, ok, rest)
	}
	_, key, rest, ok = resolve([]string{"nodes", "list"})
	if !ok || key != "nodes" || !reflect.DeepEqual(rest, []string{"list"}) {
		t.Fatalf("shorter key: %q %v", key, rest)
	}
	if _, _, _, ok = resolve([]string{"--check", "nodes"}); ok {
		t.Fatal("a flag token must not select a subcommand")
	}
	if _, _, _, ok = resolve([]string{"/repo"}); ok {
		t.Fatal("a path must fall through to the gate")
	}
}

// TestDuplicateRegisterPanics guards the one-file-per-subcommand rule.
func TestDuplicateRegisterPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate register did not panic")
		}
	}()
	register("run", func([]string) int { return 0 })
}

// TestHelpDispatch checks every captured help is reachable and run has none.
func TestHelpDispatch(t *testing.T) {
	for _, k := range surface.Keys() {
		if txt, ok := surface.Help(k); !ok || !strings.Contains(txt, "Usage:") {
			t.Errorf("help %q missing", k)
		}
	}
	if printHelp("run") {
		t.Error("run must keep its own flag usage")
	}
}

// TestBuildArgs covers the build flags, including the argv the 1.4.x proxy sends.
func TestBuildArgs(t *testing.T) {
	o, err := parseBuildArgs([]string{"--artifact", "android", "--timeout", "30", "-v", "--upload", "--tag", "v1", "--owner", "o", "--repo", "r", "/x"}, flag.ContinueOnError)
	if err != nil || o.artifact != "android" || o.timeout != 30 || !o.verbose || !o.upload || o.tag != "v1" || o.owner != "o" || o.repo != "r" || o.dir != "/x" {
		t.Fatalf("proxy argv: %+v %v", o, err)
	}
	o, err = parseBuildArgs([]string{"somedir"}, flag.ContinueOnError)
	if err != nil || o.artifact != "android" || o.timeout != 900 || o.dir != "somedir" {
		t.Fatalf("core defaults: %+v %v", o, err)
	}
}

// TestServeArgs covers flags, the allowlist merge and the unsandboxed env.
func TestServeArgs(t *testing.T) {
	t.Setenv("NSELF_CI_ALLOWED_REPOS", " b/two , ,c/three")
	t.Setenv("NSELF_CI_ALLOW_UNSANDBOXED", "TRUE")
	cfg, err := parseServeArgs([]string{"--addr", ":9", "--concurrency", "4", "--allowed-repos", "a/one,a/uno", "--allowed-repos", "z/z", "--insecure", "--verbose"}, flag.ContinueOnError)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9" || cfg.Concurrency != 4 || !cfg.Insecure || !cfg.Verbose || !cfg.AllowUnsandboxed {
		t.Fatalf("cfg = %+v", cfg)
	}
	if want := []string{"a/one", "a/uno", "z/z", "b/two", "c/three"}; !reflect.DeepEqual(cfg.AllowedRepos, want) {
		t.Fatalf("allowed = %v", cfg.AllowedRepos)
	}
	cfg, _ = parseServeArgs(nil, flag.ContinueOnError)
	if cfg.Addr != ":3845" || cfg.Concurrency != 2 || cfg.JobTimeout != 600 || cfg.WorkDir != "/tmp/nself-ci-workdirs" {
		t.Fatalf("defaults = %+v", cfg)
	}
}
