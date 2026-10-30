// Command refgen generates the CI wiki reference from the code of free/ci:
// guides/ci/Reference.md (commands, exit codes, error codes, reason codes,
// doctor ids, pinned versions), the evidence field tables of
// guides/ci/Evidence-and-Machine-Output.md, the trust reasons table of
// guides/ci/Trust-Security-and-Privacy.md and the guides/ci block of
// _Sidebar.md.
//
// Usage: go generate ./tools/refgen/... (from free/ci), or
// go run ./tools/refgen [-root DIR] [-check].
// Exit codes: 0 written or unchanged, 1 failed or (with -check) drift, 2 usage.
package main

//go:generate go run -mod=vendor . -root ../../../..

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nself-org/plugins/free/ci/internal/toolchain"
)

// Paths of the generated files, relative to the plugins repository root.
const (
	referencePage = ".github/wiki/guides/ci/Reference.md"
	evidencePage  = ".github/wiki/guides/ci/Evidence-and-Machine-Output.md"
	trustPage     = ".github/wiki/guides/ci/Trust-Security-and-Privacy.md"
	sidebarPage   = ".github/wiki/_Sidebar.md"
	guidesDir     = ".github/wiki/guides/ci"
	ciDir         = "free/ci"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run parses flags, generates and writes (or checks) the files.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("refgen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "../../../..", "plugins repository root")
	check := fs.Bool("check", false, "report drift instead of writing")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	out, err := generate(*root)
	if err != nil {
		fmt.Fprintln(stderr, "refgen:", err)
		return 1
	}
	status := 0
	for _, rel := range []string{referencePage, evidencePage, schedulingPage, trustPage, sidebarPage} {
		path := filepath.Join(*root, rel)
		have, _ := os.ReadFile(path)
		if bytes.Equal(have, out[rel]) {
			continue
		}
		if *check {
			fmt.Fprintf(stdout, "%s: out of date; run go generate ./tools/refgen/...\n", rel)
			status = 1
			continue
		}
		if err := os.WriteFile(path, out[rel], 0o644); err != nil {
			fmt.Fprintln(stderr, "refgen:", err)
			return 1
		}
		fmt.Fprintf(stdout, "wrote %s\n", rel)
	}
	return status
}

// generate returns the content every generated file should have, by path
// relative to root.
func generate(root string) (map[string][]byte, error) {
	m, err := loadModule(filepath.Join(root, ciDir))
	if err != nil {
		return nil, err
	}
	mf, err := readManifest(filepath.Join(root, ciDir, "plugin.json"))
	if err != nil {
		return nil, err
	}
	d := refData{manifest: mf, reasons: reasonCodes(m), doctor: doctorChecks(m)}
	if d.commands, err = commands(m, mf); err != nil {
		return nil, err
	}
	if d.flags, err = flagRows(m, d.commands); err != nil {
		return nil, err
	}
	if d.codes, err = errorCodes(m); err != nil {
		return nil, err
	}
	v, err := toolchain.Embedded()
	if err != nil {
		return nil, err
	}
	d.pins = v.Rows
	out := map[string][]byte{referencePage: []byte(renderReference(d))}

	for _, f := range []struct{ rel, name, body string }{
		{evidencePage, "evidence-fields", evidenceTables()},
		{trustPage, "trust-reasons", trustReasonsTable()},
	} {
		doc, err := os.ReadFile(filepath.Join(root, f.rel))
		if err != nil {
			return nil, err
		}
		s, err := splice(string(doc), f.name, f.body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.rel, err)
		}
		out[f.rel] = []byte(s)
	}
	sdoc, err := os.ReadFile(filepath.Join(root, schedulingPage))
	if err != nil {
		return nil, err
	}
	ss := string(sdoc)
	for _, r := range []struct{ name, body string }{
		{"sched-presets", presetTable()},
		{"sched-reasons", placementReasonTable(d.reasons)},
	} {
		if ss, err = splice(ss, r.name, r.body); err != nil {
			return nil, fmt.Errorf("%s: %w", schedulingPage, err)
		}
	}
	out[schedulingPage] = []byte(ss)
	block, err := sidebarBlock(filepath.Join(root, guidesDir))
	if err != nil {
		return nil, err
	}
	doc, err := os.ReadFile(filepath.Join(root, sidebarPage))
	if err != nil {
		return nil, err
	}
	s, err := splice(string(doc), "guides-ci", block)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", sidebarPage, err)
	}
	out[sidebarPage] = []byte(s)
	return out, nil
}
