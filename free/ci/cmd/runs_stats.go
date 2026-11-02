package main

// Purpose: `nself ci runs stats [--since <dur>] [--project <owner/repo>] [--coordinator <url>] [--json]`
// (P7-SCHED-20, Epic SCHED D23, contract:ci.stats v1): the health of CI itself over a window (default 24h):
// queue wait and depth per band, runner utilisation, pass, fail, flaky and infra-retry rates, job durations,
// cache hit ratio, CAS and artifact bytes, ephemeral provisioning latency, spend by confidence, leases,
// nodes, status-outbox backlog and store size.
// Inputs:  argv after `runs stats`; env NSELF_CI_HOME, NSELF_CI_COORDINATOR.
// Outputs: a short table or one v1 JSON envelope whose data is the stats document; exit 0, 1 (usage) or 2.
// Constraints: V15 only; read only; a figure whose source is absent prints "-" (JSON null) with the reason,
//          never 0; with a coordinator the document is the one it serves for the caller's projects.

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/coordinator/clientapi"
	"github.com/nself-org/plugins/free/ci/internal/stats"
)

const statsUsage = "nself ci runs stats [--since <duration>] [--project <owner/repo>] [--coordinator <url>] [--json]"

func init() { registerRunsSub("stats", runStats) }

type statsArgs struct {
	since       time.Duration
	project     string
	coordinator string
	json        bool
}

// statsFlagSet declares the flags of `runs stats` into a (nil: a throwaway); since stays text until parseStats reads it.
func statsFlagSet(a *statsArgs, since *string) *flag.FlagSet {
	if a == nil {
		a, since = &statsArgs{}, new(string)
	}
	fs := flag.NewFlagSet("runs stats", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(since, "since", "", "window to report, e.g. 7d or 24h (default 24h)")
	fs.StringVar(&a.project, "project", "", "limit the figures to one project (owner/repo)")
	fs.StringVar(&a.coordinator, "coordinator", "", "coordinator URL to ask instead of the local store")
	fs.BoolVar(&a.json, "json", false, "print one v1 JSON envelope")
	return fs
}

func parseStats(args []string) (statsArgs, error) {
	a := statsArgs{since: stats.DefaultSince}
	var since string
	pos, err := parseMixed(statsFlagSet(&a, &since), args)
	if err == nil && len(pos) > 0 {
		err = fmt.Errorf("unexpected argument %q", pos[0])
	}
	if err == nil && since != "" {
		a.since, err = clientapi.ParseSince(since)
	}
	return a, err
}

func runStats(args []string) int {
	a, err := parseStats(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return usageErr(statsUsage)
	}
	ctx := context.Background()
	if a.coordinator != "" {
		coordinatorFlag = a.coordinator
		defer func() { coordinatorFlag = "" }()
	}
	cl, _, active, err := coordResolver.Client()
	if err != nil {
		return coordReadFail(a.json, err)
	}
	if active {
		raw, err := cl.Stats(ctx, a.since, a.project)
		if err != nil {
			return coordReadFail(a.json, err)
		}
		var doc stats.Doc
		if json.Unmarshal(raw, &doc) != nil {
			return coordReadFail(a.json, fmt.Errorf("the coordinator's stats document is not valid"))
		}
		return printStats(a.json, doc, raw)
	}
	st, err := openHistory(false)
	if err != nil {
		return exitForError(err)
	}
	opts := stats.Options{Now: time.Now(), Since: a.since, Project: a.project, Global: true}
	var doc stats.Doc
	if st == nil { // no store on this machine reads as an empty one
		doc = stats.Build(stats.NoStoreRows(), opts)
	} else {
		defer func() { _ = st.Close() }()
		if doc, err = stats.Compute(ctx, st, opts); err != nil {
			return exitForError(err)
		}
	}
	return printStats(a.json, doc, nil)
}

func printStats(asJSON bool, doc stats.Doc, raw json.RawMessage) int {
	if asJSON {
		var data any = doc
		if raw != nil {
			data = raw
		}
		if err := output.WriteData(os.Stdout, "ci runs stats", data, nil); err != nil {
			return exitForError(err)
		}
		return 0
	}
	fmt.Print(stats.Text(doc))
	return 0
}
