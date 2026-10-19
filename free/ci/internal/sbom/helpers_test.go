package sbom_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sbom"
)

const pinVersion = "1.54.1"

func testPin() sbom.Pin { return sbom.Pin{Version: pinVersion} }

// fakeRunner answers `<syft> version` from versions[syft path] and any other
// command from onScan; it records every Cmd.
type fakeRunner struct {
	versions map[string]string
	onScan   func(c sbom.Cmd) (sbom.Result, error)
	cmds     []sbom.Cmd
	iso      string // "" = process
}

func (f *fakeRunner) IsolationName() string {
	if f.iso == "" {
		return "process"
	}
	return f.iso
}

func (f *fakeRunner) Run(_ context.Context, c sbom.Cmd) (sbom.Result, error) {
	f.cmds = append(f.cmds, c)
	if len(c.Argv) == 2 && c.Argv[1] == "version" {
		v, ok := f.versions[c.Argv[0]]
		if !ok {
			return sbom.Result{}, fmt.Errorf("fork/exec %s: no such file or directory", c.Argv[0])
		}
		return sbom.Result{Output: "Application:   syft\nVersion:       " + v + "\nBuildDate:     2026-10-01T19:00:50Z\n"}, nil
	}
	if f.onScan == nil {
		return sbom.Result{}, fmt.Errorf("unexpected command %v", c.Argv)
	}
	return f.onScan(c)
}

func goldenDoc(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "golden", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// scanReturning answers a scan with the golden documents, rewritten for the target name.
func scanReturning(t *testing.T, mutate func(cdx string) string) func(sbom.Cmd) (sbom.Result, error) {
	cdx, spdx := string(goldenDoc(t, "dir.cdx.json")), string(goldenDoc(t, "dir.spdx.json"))
	return func(c sbom.Cmd) (sbom.Result, error) {
		name := c.Argv[indexOf(c.Argv, "--source-name")+1]
		doc := strings.Replace(cdx, `"name": "fx"`, `"name": "`+name+`"`, 1)
		if mutate != nil {
			doc = mutate(doc)
		}
		return sbom.Result{Files: map[string][]byte{c.Collect[0]: []byte(doc), c.Collect[1]: []byte(strings.ReplaceAll(spdx, `"name": "fx"`, `"name": "`+name+`"`))}}, nil
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

// installedSyft makes an executable placeholder where the install command puts syft.
func installedSyft(t *testing.T, home string) string {
	t.Helper()
	p := sbom.ToolPath(home, pinVersion)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}
