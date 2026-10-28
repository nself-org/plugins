package main

// Purpose: TestJSONEnvelope, the --json contract of every plugin-dev
// subcommand (init, new, dev, debug, link, unlink, test): one v1 envelope on
// stdout, an exit status equal to its class, nothing else on stdout, and a
// golden per case under testdata/json.
// Inputs:  the in-process harness (json_harness_test.go) with stub tools on PATH.
// Outputs: test results.
// Constraints: plugin-dev printed no JSON before, so nothing is gated: the same
// golden must match in v1.4 and v1.5 mode (compattest.Both), proving the
// envelope does not depend on compat.V15.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/compat/compattest"
)

// stubs puts exit-status stub scripts for the named tools first on PATH.
func stubs(t *testing.T, status map[string]string) {
	t.Helper()
	for name, body := range status {
		stubBin(t, name, body)
	}
}

// onlyStubs makes PATH hold nothing but an empty dir (no dlv, no go).
func onlyStubs(t *testing.T) { t.Setenv("PATH", t.TempDir()) }

var jsonCases = []jsonCase{
	{name: "init-ok", args: []string{"init", "demo", "--no-interactive", "--json"}},
	{name: "new-ok", args: []string{"new", "demo", "--no-interactive", "--json"}},
	{name: "init-bad-template", args: []string{"init", "demo", "--template", "cobol", "--json"}, code: 1},
	{name: "init-bad-tenancy", args: []string{"init", "demo", "--tenancy", "x", "--json"}, code: 1},
	{name: "init-bad-name", args: []string{"init", "Bad_Name", "--no-interactive", "--json"}, code: 1},
	{name: "init-dest-not-empty", args: []string{"init", "demo", "--no-interactive", "--json"}, code: 1,
		setup: func(t *testing.T, root string) { pluginDir(t, root, "demo") }},
	{name: "init-missing-arg", args: []string{"init", "--json"}, code: 1},
	{name: "init-unknown-flag", args: []string{"init", "demo", "--nope=s3cret", "--json"}, code: 1},
	{name: "unknown-command", args: []string{"bogus", "--json"}, code: 1},

	{name: "link-ok", args: []string{"link", "rt", "--host", "--json"},
		setup: func(t *testing.T, root string) { pluginDir(t, root, "rt") }},
	{name: "link-not-plugin", args: []string{"link", "nowhere", "--json"}, code: 1},
	{name: "link-list-empty", args: []string{"link", "--list", "x", "--json"}},
	{name: "link-list", args: []string{"link", "--list", "x", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		if run([]string{"link", filepath.Join(root, "work", "rt")}) != 0 {
			t.Fatal("setup link failed")
		}
	}},
	{name: "link-bad-registry", args: []string{"link", "--list", "x", "--json"}, code: 2, setup: func(t *testing.T, root string) {
		dir := filepath.Join(root, "home", ".nself")
		_ = os.MkdirAll(dir, 0o750)
		_ = os.WriteFile(filepath.Join(dir, "plugin-links.json"), []byte("{broken"), 0o600)
	}},
	{name: "unlink-noop", args: []string{"unlink", "rt", "--json"}},
	{name: "unlink-ok", args: []string{"unlink", "rt", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		if run([]string{"link", filepath.Join(root, "work", "rt")}) != 0 {
			t.Fatal("setup link failed")
		}
	}},

	{name: "debug-port-only", args: []string{"debug", "rt", "--port", "3000", "--port-only", "--json"}},
	{name: "debug-bad-port", args: []string{"debug", "rt", "--port", "80", "--json"}, code: 1},
	{name: "debug-no-dlv", args: []string{"debug", "rt", "--port", "3000", "--json"}, code: 2, setup: func(t *testing.T, _ string) { onlyStubs(t) }},
	{name: "debug-ok", args: []string{"debug", "rt", "--port", "3000", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"dlv": "exit 0", "go": "exit 0"})
	}},
	{name: "debug-not-found", args: []string{"debug", "rt", "--port", "3000", "--json"}, code: 1,
		setup: func(t *testing.T, _ string) { stubs(t, map[string]string{"dlv": "exit 0"}) }},

	{name: "dev-ok", args: []string{"dev", "rt", "--no-link", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"bash": "exit 0"})
	}},
	{name: "dev-linked", args: []string{"dev", "rt", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"bash": "exit 0"})
	}},
	{name: "dev-watcher-fails", args: []string{"dev", "rt", "--no-link", "--json"}, code: 2, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"bash": "exit 3"})
	}},
	{name: "dev-bad-entrypoint", args: []string{"dev", "rt", "--no-link", "--entrypoint", "../x", "--json"}, code: 1,
		setup: func(t *testing.T, root string) { pluginDir(t, root, "rt") }},
	{name: "dev-not-found", args: []string{"dev", "rt", "--json"}, code: 1},

	{name: "test-unit-ok", args: []string{"test", "rt", "--phase", "unit", "--host", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"go": "echo child-stdout; echo child-stderr >&2; exit 0"})
	}},
	{name: "test-smoke-ok", args: []string{"test", "rt", "--phase", "smoke", "--json"}, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"nself": "exit 0", "curl": "printf 200"})
	}},
	{name: "test-unit-fails", args: []string{"test", "rt", "--phase", "unit", "--host", "--json"}, code: 1, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		stubs(t, map[string]string{"go": "echo child-stdout; echo child-stderr >&2; exit 1"})
	}},
	{name: "test-unit-no-go", args: []string{"test", "rt", "--phase", "unit", "--host", "--json"}, code: 2, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		onlyStubs(t)
	}},
	{name: "test-unit-docker-launch", args: []string{"test", "rt", "--phase", "unit", "--json"}, code: 2, setup: func(t *testing.T, root string) {
		pluginDir(t, root, "rt")
		onlyStubs(t)
		// The availability probe succeeds, but the executable disappears before launch.
		stubs(t, map[string]string{"docker": "echo child-stdout; echo child-stderr >&2; /bin/rm \"$0\"; exit 0"})
	}},
	{name: "test-bad-phase", args: []string{"test", "rt", "--phase", "bogus", "--json"}, code: 1},
	{name: "test-not-found", args: []string{"test", "rt", "--phase", "unit", "--json"}, code: 1},
}

func TestJSONEnvelope(t *testing.T) {
	compattest.Both(t, func(t *testing.T) {
		for _, c := range jsonCases {
			t.Run(c.name, func(t *testing.T) { runCase(t, c) })
		}
	})
}
