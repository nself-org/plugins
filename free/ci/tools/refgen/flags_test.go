package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReferenceListsListenerAndPackageFlags fails when a flag that is not a
// FlagSet call in the cmd package is left out of Reference.md: the serve
// listener flags (coordinator.RegisterListener, one of them from a literal
// table) and the `nodes add` flags defined in internal/nodes/lifecycle.
func TestReferenceListsListenerAndPackageFlags(t *testing.T) {
	out, err := generate(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	ref := string(out[referencePage])
	for _, row := range []string{
		"| `--agent-addr` | `serve *` |",
		"| `--api-addr` | `serve *` |",
		"| `--api-socket` | `serve *` |",
		"| `--agent-binary` | `nodes *` |",
		"| `--drain-deadline` | `serve *` |",
	} {
		if !strings.Contains(ref, row) {
			t.Errorf("Reference.md has no row %s", row)
		}
	}
}

// unmappedFlagPackages are packages whose flags Reference.md does not list yet,
// with the reason. The agent binary and the bench and helper tools are not
// `nself ci` commands; the rest need an owner mapping decided by the CI wiki
// owner (found by P7-AUTO-13/generator-gaps work, reported, not guessed).
var unmappedFlagPackages = map[string]string{
	"bench/cache/cmd/cachebench":        "benchmark tool, not a nself ci command",
	"bench/cmd/cibench":                 "benchmark tool, not a nself ci command",
	"bench/cmd/schedbench":              "benchmark tool, not a nself ci command",
	"cmd/agent":                         "the nself-ci-agent binary has its own CLI",
	"internal/agent/update":             "agent upgrade CLI (nself-ci-agent)",
	"internal/gates/truth":              "gate helper CLI; owner command not mapped",
	"internal/providers/github/jit":     "runner mint flags; owner command not mapped",
	"internal/providers/gitlab/runners": "runner mint flags; owner command not mapped",
}

// TestNoFlagDefinedOutsideScannedPackages fails when a package that flagRows
// does not read defines a flag, so a new flag cannot be left out silently:
// add the package to flagPackages (or read its registration in cmd).
func TestNoFlagDefinedOutsideScannedPackages(t *testing.T) {
	m, err := loadModule(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range m.sortedPkgs() {
		if p.path == modulePath+"/cmd" || strings.HasPrefix(p.path, modulePath+"/tools/") {
			continue
		}
		if _, ok := flagPackages[p.path]; ok {
			continue
		}
		if _, ok := unmappedFlagPackages[strings.TrimPrefix(p.path, modulePath+"/")]; ok {
			continue
		}
		for _, f := range p.files {
			names, err := fileFlagNames(m, p, f)
			if err != nil {
				t.Fatal(err)
			}
			listeners, err := listenerFlagNames(m, p, f)
			if err != nil {
				t.Fatal(err)
			}
			if got := append(names, listeners...); len(got) > 0 {
				t.Errorf("%s/%s defines flags %v that refgen does not read: add the package to flagPackages", p.path, f.name, got)
			}
		}
	}
}

// noPackages empties flagPackages for a test over a module that has none of them.
func noPackages(t *testing.T) {
	t.Helper()
	was := flagPackages
	flagPackages = map[string]string{}
	t.Cleanup(func() { flagPackages = was })
}

// fakeModule loads a one-file cmd package from src.
func fakeModule(t *testing.T, src string) *module {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cmd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cmd", "serve_x.go"), []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := loadModule(root)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestListenerFlagsFromLiteralAndTable(t *testing.T) {
	noPackages(t)
	m := fakeModule(t, `package cmd

import "x/coordinator"

func init() {
	coordinator.RegisterListener("a", "a-addr", ":1", nil)
	for _, l := range []struct{ name, flag, def string }{{"b", "b-addr", ":2"}, {"c", "c-sock", "auto"}} {
		coordinator.RegisterListener(l.name, l.flag, l.def, nil)
	}
}
`)
	cmds := []Command{{Key: "serve", Files: []string{"serve_x.go"}}, {Key: "serve install"}}
	rows, err := flagRows(m, cmds)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Name] = strings.Join(r.Commands, ",")
	}
	for _, n := range []string{"a-addr", "b-addr", "c-sock"} {
		if got[n] != "serve *" {
			t.Errorf("flag %s = %q, want serve *", n, got[n])
		}
	}
}

func TestUnresolvableListenerFlagIsAnError(t *testing.T) {
	noPackages(t)
	m := fakeModule(t, `package cmd

import "x/coordinator"

func init() { coordinator.RegisterListener("a", flagName(), ":1", nil) }
`)
	if _, err := flagRows(m, []Command{{Key: "serve", Files: []string{"serve_x.go"}}}); err == nil {
		t.Fatal("a listener flag that cannot be resolved was skipped")
	}
}
