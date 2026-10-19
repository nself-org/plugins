package truth

// Purpose: the command line of the truth check, run as `go run run.go <cmd>`
// from free/ci (the plugin binary does not link it until the V15 engine does).
// Inputs:  arguments, stdin for `hash`.
// Outputs: exit code 0 pass, 1 findings, 2 usage or a rule/scan error.
// Constraints: `check` prints one line per rule kind and the findings;
// `sweep` prints human lines on stderr and exactly one compact ci.evidence/v1
// line on stdout; `hash` reads literals from stdin (never argv) and prints
// their dead-host rule values; nothing here reads the network.

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

// Main runs one command and returns the process exit code.
func Main(args []string, in io.Reader, out, errw io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errw, "usage: run.go check|sweep|hash [flags] [rule files...]")
		return 2
	}
	switch args[0] {
	case "check", "truth":
		return cmdCheck(args[1:], out, errw)
	case "sweep":
		return cmdSweep(args[1:], out, errw)
	case "hash":
		return cmdHash(in, out, errw)
	}
	fmt.Fprintf(errw, "truth: unknown command %q (check, sweep, hash)\n", args[0])
	return 2
}

func cmdCheck(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(errw)
	root := fs.String("root", ".", "directory to scan")
	rulesRoot := fs.String("rules-root", "", "directory the rule files are relative to (default: --root)")
	kind := fs.String("kind", "", "report one rule kind only (banned-literal, retired-term, dead-host), or `rules` to validate the rule files without scanning")
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 {
		fmt.Fprintln(errw, "usage: check --root DIR [--rules-root DIR] [--kind K] RULEFILE...")
		return 2
	}
	base := *rulesRoot
	if base == "" {
		base = *root
	}
	if *kind == "rules" {
		rs, err := Load(base, fs.Args())
		if err != nil {
			fmt.Fprintln(out, "ERROR truth.rules\n  "+err.Error())
			return 2
		}
		fmt.Fprintf(out, "PASS truth.rules (%d rule(s) in %d file(s))\n", len(rs.Rules), len(rs.Files))
		return 0
	}
	checks, found := RunFull(*root, fs.Args(), *rulesRoot)
	if *kind != "" {
		var only []model.Check
		for _, c := range checks {
			if c.ID == CheckIDOf(*kind) {
				only = append(only, c)
			}
		}
		if len(only) == 0 {
			fmt.Fprintf(errw, "truth: unknown --kind %q (rules, %s)\n", *kind, strings.Join(Kinds, ", "))
			return 2
		}
		checks = only
	}
	return report(checks, found, out)
}

// report prints the checks and returns the exit code.
func report(checks []model.Check, found []Finding, out io.Writer) int {
	code := 0
	for _, c := range checks {
		fmt.Fprintf(out, "%s %s (%d finding(s))\n", strings.ToUpper(string(c.Result)), c.ID, c.Findings)
		if c.Result == "error" {
			fmt.Fprintln(out, indent(c.Excerpt))
		}
		for _, f := range found {
			if CheckIDOf(f.Kind) == c.ID {
				fmt.Fprintln(out, "  "+Excerpt([]Finding{f}, 1))
			}
		}
		switch {
		case c.Result == "error":
			code = 2
		case c.Result != "pass" && code == 0:
			code = 1
		}
	}
	return code
}

func indent(s string) string { return "  " + strings.ReplaceAll(s, "\n", "\n  ") }

// cmdSweep: sweep --rules-root DIR --revision SHA RULEFILE... -- NAME=DIR|NAME=!reason...
func cmdSweep(args []string, out, errw io.Writer) int {
	fs := flag.NewFlagSet("sweep", flag.ContinueOnError)
	fs.SetOutput(errw)
	rulesRoot := fs.String("rules-root", ".", "directory the rule files are relative to")
	revision := fs.String("revision", "", "40 hex digit revision the document is bound to")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	files, repos := splitDash(fs.Args())
	if len(files) == 0 || len(repos) == 0 {
		fmt.Fprintln(errw, "usage: sweep --rules-root DIR --revision SHA RULEFILE... -- NAME=DIR|NAME=!reason...")
		return 2
	}
	digest, err := digestFiles(*rulesRoot, files)
	if err != nil {
		fmt.Fprintln(errw, failf(files[0], "", "%v", err))
		return 2
	}
	var entries []SweepEntry
	for _, r := range repos {
		name, dir, ok := strings.Cut(r, "=")
		if !ok || name == "" {
			fmt.Fprintf(errw, "truth: bad repository argument %q (want NAME=DIR or NAME=!reason)\n", r)
			return 2
		}
		if reason, bad := strings.CutPrefix(dir, "!"); bad {
			entries = append(entries, SweepEntry{Repo: name, Err: reason})
			continue
		}
		entries = append(entries, SweepEntry{Repo: name, Checks: Run(dir, files, *rulesRoot)})
	}
	ev := SweepEvidence(*revision, digest, entries, time.Now())
	for _, c := range ev.Checks {
		fmt.Fprintf(errw, "%s %s (%d finding(s))\n", strings.ToUpper(string(c.Result)), c.ID, c.Findings)
		if c.Result != "pass" && c.Excerpt != "" {
			fmt.Fprintln(errw, indent(c.Excerpt))
		}
	}
	doc, err := json.Marshal(ev)
	if err != nil {
		fmt.Fprintln(errw, "truth: cannot encode the evidence:", err)
		return 2
	}
	fmt.Fprintln(out, string(doc))
	switch ev.Result {
	case "pass":
		return 0
	case "fail":
		return 1
	}
	return 2
}

func splitDash(args []string) (before, after []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// digestFiles is the sha256 of the rule files' bytes, in order.
func digestFiles(root string, files []string) (string, error) {
	h := sha256.New()
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func cmdHash(in io.Reader, out, errw io.Writer) int {
	sc := bufio.NewScanner(in)
	n := 0
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" {
			fmt.Fprintln(out, HashLiteral(l))
			n++
		}
	}
	if sc.Err() != nil || n == 0 {
		fmt.Fprintln(errw, "usage: hash < literals (one per line)")
		return 2
	}
	return 0
}
