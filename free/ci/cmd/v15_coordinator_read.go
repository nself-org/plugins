package main

// Purpose: the read side of `--coordinator` (P7-NODE-19, Epic D14): what `ci runs`, `ci runs list`,
// `ci runs cancel`, `ci logs` and `ci evidence` do when a coordinator or a live local serve is the target.
// Inputs:  the command's parsed arguments, the registered RemoteResolvers.
// Outputs: the command's usual output with data from the coordinator; handled=false when no coordinator
//          is active, so the command carries on against the local store as before.
// Constraints: with an active target a failure is the command's failure; none of these falls back to the
//          local store; the evidence document is the one the coordinator signed (fields untouched).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/coordclient"
	"github.com/nself-org/plugins/free/ci/internal/logs"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func coordReadFail(asJSON bool, err error) int { return coordFail(os.Stdout, os.Stderr, asJSON, err) }

// remoteRun shows one run (ref) or lists the runs (ref ""), when a coordinator is active.
func coordRuns(ctx context.Context, ref string, asJSON bool) (int, bool) {
	if ref == "" {
		list, handled, err := cirun.ListRemote(ctx)
		switch {
		case err != nil:
			return coordReadFail(asJSON, err), true
		case !handled:
			return 0, false
		}
		return printCoordList(asJSON, list), true
	}
	v, ok, err := cirun.ResolveRemote(ctx, ref)
	switch {
	case err != nil:
		return coordReadFail(asJSON, err), true
	case ok:
		return printRun(asJSON, v), true
	}
	return 0, false
}

func printCoordList(asJSON bool, list []cirun.RunView) int {
	if asJSON {
		if err := output.WriteData(os.Stdout, "ci runs", list, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	for _, r := range list {
		rev := r.Revision
		if len(rev) > 7 {
			rev = rev[:7]
		}
		fmt.Printf("%-34s %-10s %s\n", r.ID, r.Status, firstOf(rev, "-"))
	}
	return 0
}

// remoteCancel cancels ref on the coordinator.
func coordCancel(ctx context.Context, ref string) (int, bool) {
	n, handled, err := cirun.CancelRemote(ctx, ref)
	switch {
	case err != nil:
		return coordReadFail(false, err), true
	case !handled:
		return 0, false
	case n == 0:
		fmt.Fprintf(os.Stderr, "E626: run %s has no live attempt to cancel (finished, or between jobs)\n", ref)
		return 1, true
	}
	fmt.Printf("cancel requested for %d attempt(s) of %s\n", n, ref)
	return 0, true
}

// remoteLogs prints a job's stored log from the coordinator.
func coordLogs(ctx context.Context, f logsFlags, job string) (int, bool) {
	pr := newLogPrinter(f.json, logTarget{run: firstOf(f.run, "latest"), attempt: strconv.Itoa(f.attempt)})
	handled, err := cirun.LogsRemote(ctx, cirun.RemoteLogQuery{Run: f.run, Job: job, Attempt: f.attempt, Follow: f.follow},
		func(l cirun.RemoteLogLine) error {
			return pr.line(logs.Line{TS: l.TS, Stream: l.Stream, Seq: l.Seq, Text: l.Text})
		})
	switch {
	case err != nil:
		return coordReadFail(f.json, err), true
	case !handled:
		return 0, false
	}
	pr.end()
	return 0, true
}

// remoteEvidence prints the signed evidence of ref from the coordinator.
func coordEvidence(ctx context.Context, f evidenceFlags, ref string) (int, bool) {
	doc, handled, err := cirun.EvidenceRemote(ctx, ref)
	switch {
	case err != nil:
		return coordReadFail(f.json, err), true
	case !handled:
		return 0, false
	}
	ev, err := attest.Verify(doc) // schema, consistency and, for a runner-signed document, the signature
	if err != nil {
		return coordReadFail(f.json, fmt.Errorf("the coordinator's evidence for %s does not verify: %v", ref, err)), true
	}
	if f.json {
		_ = output.WriteData(os.Stdout, "ci evidence", json.RawMessage(doc), nil)
		return 0, true
	}
	printEvidence(os.Stdout, ev.RunID, ev)
	return 0, true
}

// refuseWhileServe stops a command that writes the local store while a live `ci serve` owns it: the serve
// is the one writer of that store, and a second writer (a prune that deletes rows and logs, a resolve) is
// exactly what handing a home to a serve is meant to prevent. handled=true means the command must return.
func refuseWhileServe(command string) (int, bool) {
	home := coordclient.Home(os.Getenv)
	if !coordclient.ServeHoldsLock(home) {
		return 0, false
	}
	fmt.Fprintf(os.Stderr, "E664: %s writes the run store of %s, which a running `nself ci serve` owns (serve_unreachable); stop the serve first\n", command, home)
	return 1, true
}
