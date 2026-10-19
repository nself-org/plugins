package main

// Purpose: `nself ci runs follow <run> [--from-seq n] [--coordinator <url>]
// [--json]` (P7-CI-63, Epic D30): replay a run's contract:ci.events records and
// tail them until the evidence line. The command is a stream: output is always
// NDJSON (json-stream v1), one envelope per line, seq strictly increasing,
// ending with the data.type "result" line that carries the evidence; --json is
// accepted for symmetry.
// Inputs:  a run reference; NSELF_CI_HOME.
// Outputs: NDJSON on stdout; exit 0 after the result line, 1 (unknown run,
// usage) or 2 (the stream failed or the run died before it finished).
// Constraints: read class (HTTP serves it as NDJSON, MCP lists no tool for it);
// a run started with `--detach` replays its own event file byte for byte; any
// other run (foreground, or on a coordinator) gets the records rebuilt from its
// observable state, the store keeps no event log (event_index is not written);
// a run that died without a result ends with an E648 child_exit error line.

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/detach"
	"github.com/nself-org/plugins/free/ci/internal/evidence"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// codeStoreFailed is the store's E607 (store operation failed), the default infra code.
const codeStoreFailed = "E607"

func init() {
	registerRunsSub("follow", runFollow)
	registerV15("runs follow", subcommandEntry{V15Only: true, Flags: func() *flag.FlagSet { return newFollowFlagSet(&followFlags{}) }, Handler: runFollow})
}

type followFlags struct {
	json        bool
	from        int64
	coordinator string
}

// followIdle is how long a quiet event file waits before the run state is asked (a var for tests).
var followIdle = detach.IdleGrace

func newFollowFlagSet(f *followFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("runs follow", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "accepted; the output is always json-stream v1")
	fs.Int64Var(&f.from, "from-seq", 0, "first event seq to print (default: replay from the start)")
	fs.StringVar(&f.coordinator, "coordinator", "", "coordinator URL the run lives on (env NSELF_CI_COORDINATOR)")
	return fs
}

func runFollow(args []string) int {
	var f followFlags
	pos, err := parseMixed(newFollowFlagSet(&f), args)
	if err != nil || len(pos) != 1 || f.from < 0 {
		return usageErr("nself ci runs follow <run> [--from-seq n] [--coordinator <url>] [--json]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	src, err := openRunSource(ctx, pos[0], f.coordinator)
	if err != nil {
		return followFail(err, "", exitForError(err))
	}
	defer src.Close()
	v, err := src.View(ctx)
	if err != nil {
		return followFail(err, "", exitForError(err))
	}
	if path := detach.StreamPath(detach.Home(""), v.RunID); path != "" {
		return followFile(ctx, src, path, f.from)
	}
	return followSynth(ctx, src, v, f.from)
}

// followFile copies a detached run's event file; if the run ended without a
// final line it waits once more (the engine writes the final line just after
// the store marks the run finished), then closes the stream from the run's
// last state.
func followFile(ctx context.Context, src *runSource, path string, from int64) int {
	over := func() (bool, error) { // finished in the store AND its writer gone: the store marks a run finished before the last events are written
		cur, err := src.View(ctx)
		return err == nil && detach.Terminal(cur.State) && !detach.WriterAlive(path), err
	}
	var out detach.Outcome
	var err error
	for attempt, idle := 0, followIdle; attempt < 2; attempt, idle = attempt+1, idle*5 {
		out, err = detach.Follow(ctx, os.Stdout, detach.FollowParams{Path: path, From: from, Idle: idle, Over: over})
		if err != nil || out.End != detach.EndGone {
			break
		}
		from = max(from, out.LastSeq+1)
	}
	switch {
	case err != nil:
		return followFail(err, "", 2)
	case out.End == detach.EndResult:
		return 0
	case out.End == detach.EndError:
		return max(out.Exit, 1)
	}
	cur, err := src.View(ctx)
	if err != nil {
		return followFail(err, "", 2)
	}
	if cur.State == "lost" {
		return followFail(nil, "child_exit: the detached run ended without a result (state lost)", 2, detach.CodeID)
	}
	line, err := detach.ResultLine(cur.RunID, max(out.LastSeq+1, from), evidenceOf(ctx, src, cur))
	if err != nil {
		return followFail(err, "", 2)
	}
	_, _ = os.Stdout.Write(line)
	return 0
}

// followSynth rebuilds the stream of a run with no event file.
func followSynth(ctx context.Context, src *runSource, v detach.View, from int64) int {
	syn := detach.NewSynth(os.Stdout, v.RunID, from)
	for {
		syn.Observe(v)
		if detach.Terminal(v.State) {
			syn.Finish(v, evidenceOf(ctx, src, v))
			return 0
		}
		select {
		case <-ctx.Done():
			syn.Flush()
			return followFail(ctx.Err(), "", 1)
		case <-time.After(waitPoll):
		}
		var err error
		if v, err = src.View(ctx); err != nil {
			syn.Flush()
			return followFail(err, "", exitForError(err))
		}
	}
}

// evidenceOf is the run's evidence document when this machine can build it,
// else a minimal document naming the run and its state.
func evidenceOf(ctx context.Context, src *runSource, v detach.View) any {
	if src.st != nil {
		if root, err := evidenceRoot(""); err == nil {
			if shown, err := evidence.Show(ctx, evidenceEnv(root, src.st), v.RunID); err == nil {
				return shown.Evidence
			}
		}
	}
	return map[string]any{"run_id": v.RunID, "state": v.State}
}

// followFail writes the closing error line of a stream and returns the exit.
// The code is explicit, else the error's own, else E626 (usage) or E607 (infra).
func followFail(err error, msg string, exit int, code ...string) int {
	if msg == "" && err != nil {
		msg = err.Error()
	}
	c := cirun.CodeOf(err)
	if len(code) > 0 {
		c = code[0]
	}
	if c == "" {
		c = cirun.CodeRun
		if exit >= 2 {
			c = codeStoreFailed
		}
	}
	if b, e := output.StreamError("ci runs follow", output.ErrorDetail{Code: c, Message: msg, ExitCode: exit}, nil); e == nil {
		_, _ = os.Stdout.Write(b)
	}
	return exit
}
