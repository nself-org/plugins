package main

// Purpose: `nself ci runs --flaky [--limit N] [--json]` lists checks that
// passed and failed on identical inputs, and the bare-run flag `--confirm-flaky
// N` that reruns a failed job N more times on request (P7-CI-40,
// cap:ci.diagnostics; the detector is internal/flaky, the reruns are
// internal/run/retry.go).
// Inputs:  argv after `runs`; env NSELF_CI_HOME.
// Outputs: a table or one v1 envelope; exit 0 (a listing never fails a run),
// 1 usage, 2 infrastructure.
// Constraints: V15 only; read-only, never creates a store; a flaky check is
// debt shown beside its result and is never reported as a pass.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/flaky"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// confirmFlakyFlag is the parsed `--confirm-flaky`; the options hook moves it into RunOptions.
var confirmFlakyFlag int

func init() {
	registerRunsFlag("flaky", runRunsFlaky)
	cirun.FlakyHistory = flaky.Annotate
	bareFlags.IntVar(&confirmFlakyFlag, "confirm-flaky", 0, "rerun a job that failed with a code failure up to N more times (recorded; the failure stays a failure)")
	RegisterOptionsHook(func(o *RunOptions) error {
		n := confirmFlakyFlag
		confirmFlakyFlag = 0
		if n < 0 || n > cirun.MaxConfirmFlaky {
			return &cirun.Error{Code: cirun.CodeOptions, Message: fmt.Sprintf("--confirm-flaky must be between 0 and %d", cirun.MaxConfirmFlaky)}
		}
		o.ConfirmFlaky = n
		return nil
	})
}

func runRunsFlaky(args []string) int {
	fs := flag.NewFlagSet("runs --flaky", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print one v1 JSON envelope")
	limit := fs.Int("limit", flaky.DefaultRuns, "read at most N recent runs")
	pos, err := parseMixed(fs, args)
	if err != nil || len(pos) > 0 || *limit < 1 {
		return usageErr("nself ci runs --flaky [--limit N] [--json]")
	}
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	flakes, err := flaky.Scan(context.Background(), st, *limit)
	if err != nil {
		return exitForError(err)
	}
	if *asJSON {
		if err = output.WriteData(os.Stdout, "ci runs flaky", flakes, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	if len(flakes) == 0 {
		fmt.Println("no flaky checks found in the recorded runs")
		return 0
	}
	fmt.Printf("%-24s %-8s %-12s %-5s %s\n", "CHECK", "REVISION", "SOURCE", "P/F", "RUNS")
	for _, f := range flakes {
		var runs []string
		for _, r := range f.Runs {
			runs = append(runs, r.Run+":"+string(r.Result))
		}
		fmt.Printf("%-24s %-8.7s %-12s %d/%d   %s\n", f.Check, firstOf(f.Revision, "-"), f.Source, f.Passes, f.Fails, strings.Join(runs, " "))
	}
	fmt.Println("flaky checks are debt: listing them changes no result and none of them counts as a pass")
	return 0
}
