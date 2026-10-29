package main

// Purpose: `nself ci runs wait <run> [--timeout <dur>] [--coordinator <url>]
// [--json]` (P7-CI-63, Epic D30): a bounded long-poll for agents and scripts.
// It returns as soon as the run is terminal (exit 0, whatever the result: read
// state) or at the timeout (exit 10, state running).
// Inputs:  a run reference (id, unique prefix, "latest"); NSELF_CI_HOME;
// NSELF_INVOKE_DEADLINE_MS (set by the machine invoker, P7-SURF-24).
// Outputs: one v1 envelope (--json) or a table; exit 0, 10, 1 (unknown run,
// usage) or 2 (infrastructure).
// Constraints: read class; the wait never ends later than the invoke deadline
// minus 10 s, so a document call is not killed first; the first read of a
// local run is allowed to finish (firstReadGrace) even when the budget is
// shorter, so a real run reports its real state; each poll recovers after
// dead coordinators (idempotent), so a killed run turns `lost` (terminal, exit
// 0) instead of staying running; remote runs are asked through the registered
// RemoteResolvers, which read --coordinator as NSELF_CI_COORDINATOR.

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const (
	// defaultWaitTimeout is the wait when --timeout is not given.
	defaultWaitTimeout = 240 * time.Second
	// deadlineMargin is kept free before the invoke deadline.
	deadlineMargin = 10 * time.Second
	// exitStillRunning is the exit status of a wait that timed out.
	exitStillRunning = 10
	// firstReadGrace is the longest the first read of a local run may take when the budget is shorter
	// (store open, dead-coordinator recovery, the read itself). It is half the deadline margin, so the
	// wait still ends before the invoker's deadline, and it bounds a held SQLite lock.
	firstReadGrace = deadlineMargin / 2
)

// waitPoll is the store poll interval (a var for tests).
var waitPoll = 250 * time.Millisecond

func init() {
	registerRunsSub("wait", runWait)
	registerV15("runs wait", subcommandEntry{V15Only: true, SideEffect: SideEffectRead, Flags: func() *flag.FlagSet { return newWaitFlagSet(&waitFlags{}) }, Handler: runWait})
}

type waitFlags struct {
	json        bool
	timeout     time.Duration
	coordinator string
}

func newWaitFlagSet(f *waitFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("runs wait", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.DurationVar(&f.timeout, "timeout", defaultWaitTimeout, "longest wait (capped below NSELF_INVOKE_DEADLINE_MS minus 10s)")
	fs.StringVar(&f.coordinator, "coordinator", "", "coordinator URL the run lives on (env NSELF_CI_COORDINATOR)")
	return fs
}

// waitBudget is how long a wait may last: the timeout, never past the invoke
// deadline minus the margin. deadlineMS is the raw env value.
func waitBudget(timeout time.Duration, deadlineMS string) time.Duration {
	if timeout < 0 {
		timeout = 0
	}
	if ms, err := strconv.ParseInt(deadlineMS, 10, 64); err == nil && ms > 0 {
		if capped := time.Duration(ms)*time.Millisecond - deadlineMargin; capped < timeout {
			timeout = max(capped, 0)
		}
	}
	return timeout
}

type waitDoc struct {
	RunID    string          `json:"run_id"`
	State    string          `json:"state"`
	Terminal bool            `json:"terminal"`
	Revision string          `json:"revision,omitempty"`
	Jobs     []cirun.JobView `json:"jobs"`
	WaitedMS int64           `json:"waited_ms"`
}

func runWait(args []string) int {
	started := time.Now()
	var f waitFlags
	pos, err := parseMixed(newWaitFlagSet(&f), args)
	if err != nil || len(pos) != 1 || f.timeout < 0 {
		return usageErr("nself ci runs wait <run> [--timeout <dur>] [--coordinator <url>] [--json]")
	}
	budget := waitBudget(f.timeout, os.Getenv("NSELF_INVOKE_DEADLINE_MS"))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Every store, recovery and resolver call shares the budget (a zero budget
	// still gets a short first look), so a stalled call cannot outlive the invoke deadline.
	wctx, cancel := context.WithDeadline(ctx, started.Add(cmp.Or(budget, 500*time.Millisecond)))
	defer cancel()
	// The first local read gets at least firstReadGrace: a budget that ends before it would report the
	// run as "unknown" although it exists. Later polls keep the budget.
	gctx, gcancel := context.WithDeadline(ctx, started.Add(max(budget, firstReadGrace)))
	defer gcancel()
	src, err := openRunSourceGrace(wctx, gctx, pos[0], f.coordinator)
	if err != nil {
		return waitFail(ctx, wctx, err, detach.View{RunID: pos[0]}, f.json, started)
	}
	defer src.Close()
	src.firstRead = gctx
	last := detach.View{RunID: pos[0], State: "unknown"}
	for {
		v, err := src.View(wctx)
		if err != nil {
			return waitFail(ctx, wctx, err, last, f.json, started)
		}
		last = v
		if detach.Terminal(v.State) {
			return printWait(f.json, v, src.Detail, true, time.Since(started))
		}
		left := budget - time.Since(started)
		if left <= 0 {
			return printWait(f.json, v, src.Detail, false, time.Since(started))
		}
		select {
		case <-wctx.Done():
			return waitFail(ctx, wctx, wctx.Err(), last, f.json, started)
		case <-time.After(min(waitPoll, left)):
		}
	}
}

// waitFail ends a wait whose lookup failed: the budget running out is "still
// running" (exit 10, the last state seen); anything else is the error.
func waitFail(ctx, wctx context.Context, err error, last detach.View, asJSON bool, started time.Time) int {
	if ctx.Err() == nil && wctx.Err() != nil {
		return printWait(asJSON, last, cirun.RunView{ID: last.RunID, Status: last.State}, false, time.Since(started))
	}
	return exitForError(err)
}

func printWait(asJSON bool, v detach.View, d cirun.RunView, terminal bool, waited time.Duration) int {
	code := 0
	if !terminal {
		code = exitStillRunning
	}
	doc := waitDoc{RunID: v.RunID, State: v.State, Terminal: terminal, Revision: d.Revision, Jobs: d.Jobs, WaitedMS: waited.Milliseconds()}
	if doc.Jobs == nil {
		doc.Jobs = []cirun.JobView{}
	}
	if asJSON {
		if err := output.WriteData(os.Stdout, "ci runs wait", doc, nil); err != nil {
			return exitForError(err)
		}
		return code
	}
	fmt.Printf("run %s: %s\n", v.RunID, v.State)
	for _, j := range d.Jobs {
		fmt.Printf("  %-14s %s %s\n", j.State, j.Name, j.FailureClass)
	}
	return code
}
