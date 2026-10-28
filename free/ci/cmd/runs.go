package main

// Purpose: `nself ci runs [run] [--json]` and `nself ci runs cancel <run>`
// (P7-CI-37, Epic D23): read the store from any process. Later Tickets
// (P7-CI-39 history/prune, 40 flaky, 41 reproduce, P7-TRUST-13, P7-CACHE-05)
// add `runs <sub>` through registerRunsSub from their own files.
// Inputs:  argv after `runs`; env NSELF_CI_HOME.
// Outputs: a table or one v1 JSON envelope; exit 0, 1 (unknown run, usage)
// or 2 (infrastructure).
// Constraints: V15 only; registered remote resolvers are asked first; reading
// first recovers after coordinators that died (idempotent).

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/output"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// runsSubs maps a `runs <sub>` name to its handler.
var runsSubs = map[string]handler{}

// registerRunsSub adds a `nself ci runs <name>` subcommand.
func registerRunsSub(name string, h handler) {
	if _, dup := runsSubs[name]; dup || strings.ContainsAny(name, " -") {
		panic("nself-ci: invalid runs subcommand " + name)
	}
	runsSubs[name] = h
}

// runsFlagSubs maps a `runs --<name>` view to its handler (P7-CI-40: --flaky).
var runsFlagSubs = map[string]handler{}

// registerRunsFlag adds a `nself ci runs --<name> [args]` view; the handler gets the other args.
func registerRunsFlag(name string, h handler) {
	if _, dup := runsFlagSubs[name]; dup || name == "" || strings.HasPrefix(name, "-") {
		panic("nself-ci: invalid runs flag view " + name)
	}
	runsFlagSubs[name] = h
}

func runsFlags() *flag.FlagSet {
	fs := flag.NewFlagSet("runs", flag.ContinueOnError)
	fs.Bool("json", false, "print one v1 JSON envelope")
	fs.Bool("flaky", false, "list checks that passed and failed on identical inputs")
	return fs
}

func init() {
	registerV15("runs", subcommandEntry{V15Only: true, Flags: runsFlags, Handler: runRuns})
	registerRunsSub("cancel", runCancel)
}

func runRuns(args []string) int {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		if h, ok := runsSubs[args[0]]; ok {
			return h(args[1:])
		}
	}
	for i, a := range args {
		if h, ok := runsFlagSubs[strings.TrimLeft(a, "-")]; ok && strings.HasPrefix(a, "-") {
			return h(append(append([]string(nil), args[:i]...), args[i+1:]...))
		}
	}
	var ref string
	var asJSON bool
	for _, a := range args {
		switch {
		case a == "--json" || a == "-json" || a == "--json=true":
			asJSON = true
		case strings.HasPrefix(a, "-") || ref != "":
			fmt.Fprintln(os.Stderr, "usage: nself ci runs [run] [--json] | nself ci runs --flaky [--json] | nself ci runs cancel <run>")
			return 1
		default:
			ref = a
		}
	}
	ctx := context.Background()
	if code, ok := coordRuns(ctx, ref, asJSON); ok { // a coordinator or live local serve owns the runs (P7-NODE-19)
		return code
	}
	for _, r := range cirun.Registered.Remote {
		if v, ok, err := r.Resolve(ctx, ref); err == nil && ok {
			return printRun(asJSON, v)
		}
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return exitForError(err)
	}
	defer func() { _ = st.Close() }()
	_ = cirun.Recover(ctx, st, "", os.Getenv("NSELF_CI_HOME"))
	if ref == "" {
		return listRuns(ctx, st, asJSON)
	}
	row, err := cirun.ResolveRun(ctx, st, ref)
	if err != nil {
		return exitForError(err)
	}
	v, err := cirun.ViewRun(ctx, st, row)
	if err != nil {
		return exitForError(err)
	}
	return printRun(asJSON, v)
}

func runsStorePath() string {
	if h := os.Getenv("NSELF_CI_HOME"); h != "" {
		return filepath.Join(h, "state.db")
	}
	return ""
}

func listRuns(ctx context.Context, st *store.Store, asJSON bool) int {
	rows, err := cirun.ListRuns(ctx, st, 20)
	if err != nil {
		return exitForError(err)
	}
	if asJSON {
		type row struct {
			ID       string `json:"id"`
			Project  string `json:"project"`
			Revision string `json:"revision"`
			Trigger  string `json:"trigger"`
			Status   string `json:"status"`
		}
		list := make([]row, len(rows))
		for i, r := range rows {
			list[i] = row{r.ID, r.Project, r.Revision, r.Trigger, r.Status}
		}
		if err = output.WriteData(os.Stdout, "ci runs", list, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	for _, r := range rows {
		rev := r.Revision
		if len(rev) > 7 {
			rev = rev[:7]
		}
		fmt.Printf("%-34s %-10s %-8s %s\n", r.ID, r.Status, firstOf(rev, "-"), r.Trigger)
	}
	return 0
}

func printRun(asJSON bool, v cirun.RunView) int {
	if asJSON {
		if err := output.WriteData(os.Stdout, "ci runs", v, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	fmt.Printf("run %s: %s\n", v.ID, v.Status)
	for _, j := range v.Jobs {
		fmt.Printf("  %-14s %s %s\n", j.State, j.Name, j.FailureClass)
		if j.IdempotencyKey != "" {
			fmt.Printf("    attempt %s idempotency-key %s\n", j.AttemptID, j.IdempotencyKey)
		}
	}
	return 0
}

// runCancel is `nself ci runs cancel <run>`: it writes the cancel intents;
// the owning coordinator stops the jobs and the process trees.
func runCancel(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintln(os.Stderr, "usage: nself ci runs cancel <run>")
		return 1
	}
	ctx := context.Background()
	if code, ok := coordCancel(ctx, args[0]); ok { // never open the store of a local serve (P7-NODE-19)
		return code
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return exitForError(err)
	}
	defer func() { _ = st.Close() }()
	n, err := cirun.CancelRun(ctx, st, args[0], "cancelled by `nself ci runs cancel`")
	if err != nil {
		return exitForError(err)
	}
	if n == 0 {
		fmt.Fprintf(os.Stderr, "E626: run %s has no live attempt to cancel (finished, or between jobs)\n", args[0])
		return 1
	}
	fmt.Printf("cancel requested for %d attempt(s) of %s\n", n, args[0])
	return 0
}
