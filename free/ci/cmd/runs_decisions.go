package main

// Purpose: `nself ci runs decisions [<run>] [--json]` (P7-SCHED-05, Epic D16):
// lists the stored placement decisions, one per attempt, newest runs last. The
// ref column is the placement_ref evidence carries (sched:decision/<id>), and
// <run> may also be such a ref. Also registers the stored-record source of
// `nself ci explain <run>/<job>`.
// Inputs:  argv after `decisions`; env NSELF_CI_HOME.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (unknown run or ref,
// usage) or 2 (infrastructure).
// Constraints: V15 only (the runs command is); read only, and no store on this
// machine is an empty list; with no <run> the newest 20 decisions are shown.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/planview"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/schedtel"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

const decisionsUsage = "nself ci runs decisions [<run>] [--json]"

func init() {
	registerRunsSub("decisions", runDecisions)
	planview.Register(schedtel.Source{Open: openDecisionStore})
}

// openDecisionStore opens the run store for the explain source; none is an E692.
func openDecisionStore(context.Context) (*store.Store, func(), error) {
	st, err := openHistory(false)
	if err != nil {
		return nil, nil, err
	}
	if st == nil {
		return nil, nil, &planview.Error{Code: planview.CodeExplainTarget, Message: "no run history on this machine, so no decisions are recorded"}
	}
	return st, func() { _ = st.Close() }, nil
}

// decisionView is one row of the output.
type decisionView struct {
	Ref         string `json:"ref"`
	Run         string `json:"run"`
	Job         string `json:"job"`
	JobName     string `json:"job_name"`
	Attempt     int    `json:"attempt"`
	Outcome     string `json:"outcome"`
	Runner      string `json:"runner"`
	RunnerClass string `json:"runner_class"`
	CostMicro   int64  `json:"cost_micro"`
	WaitMs      int64  `json:"wait_ms"`
	WorldDigest string `json:"world_digest"`
	At          int64  `json:"at_ms"`
}

func runDecisions(args []string) int {
	var asJSON bool
	var ref string
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json" || a == "--json=true":
			asJSON = true
		case strings.HasPrefix(a, "-") || ref != "":
			return usageErr(decisionsUsage)
		default:
			ref = a
		}
	}
	ctx := context.Background()
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	var recs []store.TelDecision
	if st != nil {
		defer func() { _ = st.Close() }()
		if recs, err = decisionsFor(ctx, st, ref); err != nil {
			var pe *planview.Error
			if errors.As(err, &pe) { // an unknown ref is the caller's mistake, not an infrastructure failure
				fmt.Fprintln(os.Stderr, err)
				return 1
			}
			return exitForError(err)
		}
	}
	list := make([]decisionView, len(recs))
	for i, d := range recs {
		list[i] = decisionView{schedtel.RefPrefix + d.ID, d.Run, d.Job, d.JobName, d.Attempt, d.Outcome, d.Chosen, d.RunnerClass, d.CostMicro, d.WaitMs, d.WorldDigest, d.TS}
	}
	if asJSON {
		if err = output.WriteData(os.Stdout, "ci runs decisions", list, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	for _, d := range list {
		fmt.Printf("%-40s %-10s %-12s %-10s wait %dms\n", d.Ref, d.Outcome, firstOf(d.Runner, "-"), firstOf(d.RunnerClass, "-"), d.WaitMs)
	}
	return 0
}

// decisionsFor resolves ref: empty (newest), a placement ref or attempt id, or a run.
func decisionsFor(ctx context.Context, st *store.Store, ref string) ([]store.TelDecision, error) {
	switch {
	case ref == "":
		return st.TelDecisions(ctx, "", 20)
	case strings.HasPrefix(ref, schedtel.RefPrefix):
		d, ok, err := st.TelDecision(ctx, strings.TrimPrefix(ref, schedtel.RefPrefix))
		if err != nil || !ok {
			return nil, firstErr(err, &planview.Error{Code: planview.CodeExplainTarget, Message: fmt.Sprintf("no decision record %q", ref)})
		}
		return []store.TelDecision{d}, nil
	}
	row, err := cirun.ResolveRun(ctx, st, ref)
	if err != nil {
		return nil, err
	}
	return st.TelDecisions(ctx, row.ID, 0)
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}
