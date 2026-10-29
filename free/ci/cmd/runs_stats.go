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
	"fmt"
	"os"
	"strings"
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

func parseStats(args []string) (statsArgs, error) {
	a := statsArgs{since: stats.DefaultSince}
	for i := 0; i < len(args); i++ {
		name, val, hasVal := strings.Cut(args[i], "=")
		take := func() (string, error) {
			if hasVal {
				return val, nil
			}
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", name)
			}
			i++
			return args[i], nil
		}
		var err error
		var v string
		switch name {
		case "--json", "-json":
			a.json = true
		case "--since":
			if v, err = take(); err == nil {
				a.since, err = clientapi.ParseSince(v)
			}
		case "--project":
			a.project, err = take()
		case "--coordinator":
			a.coordinator, err = take()
		default:
			err = fmt.Errorf("unexpected argument %q", args[i])
		}
		if err != nil {
			return a, err
		}
	}
	return a, nil
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
