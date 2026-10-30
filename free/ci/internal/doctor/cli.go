package doctor

// Purpose: the logic of `nself ci doctor [--json] [--nodes] [--providers]
// [--selftest [--alert]] [--repo DIR]`, kept here with injectable Deps so cmd/doctor.go stays a shim.
// Inputs:  argv after `doctor`, the output writers, Deps.
// Outputs: a table, or with --json one v1 envelope (contract:cli.json-
// envelope) whose data is {checks, summary}; exit 0 clean, 10 when a check
// failed, 12 with warnings only (P7-REG D10), 1 for a usage error and 2 when
// the report itself cannot be produced.
// Constraints: warnings are also listed in the envelope meta; a failing check
// still prints the full report.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/config"
)

const command = "ci doctor"

type options struct {
	json, nodes, providers bool
	selftest, alert        bool
	repo                   string
}

func newFlags() (*flag.FlagSet, *options) {
	o := &options{}
	fs := flag.NewFlagSet(command, flag.ContinueOnError)
	fs.BoolVar(&o.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&o.nodes, "nodes", false, "include the nodes section")
	fs.BoolVar(&o.providers, "providers", false, "include the providers section")
	fs.BoolVar(&o.selftest, "selftest", false, "include the selftest checks of an unattended host (CI-DS…)")
	fs.BoolVar(&o.alert, "alert", false, "with --selftest, send one notifier message a day per failing set")
	fs.StringVar(&o.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	return fs, o
}

// Flags returns the flag set of `ci doctor` for help generation.
func Flags() *flag.FlagSet { fs, _ := newFlags(); return fs }

type reportData struct {
	Checks  []Check `json:"checks"`
	Summary Summary `json:"summary"`
}

// Run implements `nself ci doctor`.
func Run(args []string, stdout, stderr io.Writer, d Deps) int {
	fs, o := newFlags()
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 1
	}
	if fs.NArg() > 0 || (o.alert && !o.selftest) {
		_, _ = fmt.Fprintln(stderr, "usage: nself ci doctor [--json] [--nodes] [--providers] [--selftest [--alert]] [--repo DIR]")
		return 1
	}
	root, err := RepoRoot(o.repo, d)
	if err != nil {
		return fail(o.json, stdout, stderr, err)
	}
	if fi, statErr := os.Stat(root); statErr != nil || !fi.IsDir() {
		return fail(o.json, stdout, stderr, fmt.Errorf("doctor: --repo %s is not a readable directory", root))
	}
	ctx := context.Background()
	checks := RunLocal(ctx, root, d)
	env := Env{Root: root, Capture: d.Capture, Getenv: d.Getenv}
	want := map[string]bool{SelectNodes: o.nodes, SelectProviders: o.providers, SelectSelftest: o.selftest}
	checks = append(checks, RunSections(ctx, env, want, SectionBudget)...)
	sortChecks(checks)
	if o.alert {
		if sent, err := alert(ctx, env, checks); err != nil {
			_, _ = fmt.Fprintln(stderr, "doctor: the alert was not sent:", err)
		} else if sent != "" && !o.json {
			_, _ = fmt.Fprintln(stderr, "doctor:", sent)
		}
	}

	data := reportData{Checks: checks, Summary: Summarise(checks)}
	code := ExitCode(checks)
	if o.json {
		var warnings []string
		for _, c := range checks {
			if c.Status == Warn {
				warnings = append(warnings, c.ID+": "+c.Message)
			}
		}
		if err := output.WriteData(stdout, command, data, &output.Meta{Warnings: warnings}); err != nil {
			return fail(false, stdout, stderr, err)
		}
		return code
	}
	printReport(stdout, data)
	return code
}

func fail(asJSON bool, stdout, stderr io.Writer, err error) int {
	if asJSON {
		_ = output.WriteError(stdout, command, output.ErrorDetail{Code: config.CodeInvalid, Message: err.Error(), ExitCode: 2}, nil)
	} else {
		_, _ = fmt.Fprintln(stderr, err)
	}
	return 2
}

func printReport(w io.Writer, data reportData) {
	for _, c := range data.Checks {
		_, _ = fmt.Fprintf(w, "%-4s %-8s %s\n", strings.ToUpper(string(c.Status)), c.ID, c.Message)
		if c.Detail != "" {
			for _, line := range strings.Split(c.Detail, "\n") {
				_, _ = fmt.Fprintf(w, "       %s\n", line)
			}
		}
		if c.Remediation != "" && c.Status != Pass {
			_, _ = fmt.Fprintf(w, "       fix: %s\n", c.Remediation)
		}
	}
	s := data.Summary
	_, _ = fmt.Fprintf(w, "\n%d passed, %d warning(s), %d failed\n", s.Pass, s.Warn, s.Fail)
}
