package main

// Purpose: `nself ci logs <job|attempt> [-f] [--json] [--run <ref>] [--attempt N]`
// (P7-CI-39, Epic D12/D23): print the stored, redacted output of one attempt,
// or follow it while it runs. The files and the reader are internal/logs; this
// file resolves what to read, follows until the attempt ends, and prints.
// Inputs:  a job name (of --run, default the latest run), a job id, or an
// attempt id / unique prefix; env NSELF_CI_HOME.
// Outputs: text lines, or with --json ci logs NDJSON (contract:cli.json-stream:
// one `log` record per line, the last record the result); exit 0, 1 (nothing
// to read, usage) or 2 (storage failure).
// Constraints: V15 only; registered RemoteResolvers are asked first (a run on
// another machine is read there); the command never creates a store; with -f
// a line appears within about a second of being stored; ctrl-C ends a follow.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/logs"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type logsFlags struct {
	follow, json bool
	run          string
	attempt      int
}

func newLogsFlagSet(f *logsFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	fs.BoolVar(&f.follow, "f", false, "follow the log while the attempt runs")
	fs.BoolVar(&f.json, "json", false, "print ci logs NDJSON records")
	fs.StringVar(&f.run, "run", "", "the run a job name belongs to (default: the latest run)")
	fs.IntVar(&f.attempt, "attempt", 0, "which attempt of the job (default: the newest)")
	return fs
}

func init() {
	registerV15("logs", subcommandEntry{V15Only: true, Handler: runLogs, Flags: func() *flag.FlagSet { return newLogsFlagSet(&logsFlags{}) }})
}

// logTarget is the attempt whose log is read.
type logTarget struct{ run, attempt string }

func runLogs(args []string) int {
	var f logsFlags
	pos, err := parseMixed(newLogsFlagSet(&f), args)
	if err != nil || len(pos) != 1 || f.attempt < 0 {
		return usageErr("nself ci logs <job|attempt> [-f] [--json] [--run <ref>] [--attempt N]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for _, r := range cirun.Registered.Remote {
		if v, ok, rerr := r.Resolve(ctx, f.run); rerr == nil && ok {
			return logsFail(f, "ci logs", fmt.Errorf("%s: run %s lives on another machine; read its logs there", logs.CodeNone, v.ID))
		}
	}
	st, err := openHistory(false)
	if err != nil {
		return logsFail(f, "ci logs", err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	home := os.Getenv("NSELF_CI_HOME")
	tgt, err := resolveLogTarget(ctx, st, home, pos[0], f)
	if err != nil {
		return logsFail(f, "ci logs", err)
	}
	dir := logs.AttemptDir(home, tgt.run, tgt.attempt)
	if !logs.Exists(dir) && !f.follow {
		return logsFail(f, "ci logs", fmt.Errorf("%s: no stored logs for attempt %s (the job produced no output, or retention removed them)", logs.CodeNone, tgt.attempt))
	}
	pr := newLogPrinter(f.json, tgt)
	done := func() bool { return attemptEnded(ctx, st, tgt.attempt) }
	if f.follow {
		err = logs.Follow(ctx, dir, 0, done, pr.line)
	} else {
		err = logs.Read(dir, 0, pr.line)
	}
	if err != nil {
		return logsFail(f, "ci logs", err)
	}
	pr.end()
	return 0
}

// attemptEnded reports whether the attempt can no longer write output. Without
// a store (or without a state for it) nothing is running.
func attemptEnded(ctx context.Context, st *store.Store, attempt string) bool {
	if st == nil {
		return true
	}
	state, _, err := st.AttemptStateEpoch(ctx, attempt)
	if err != nil {
		return true
	}
	switch state {
	case model.JobState("passed"), "failed", "cancelled", "lost", "needs-operator":
		return true
	}
	return false
}

// resolveLogTarget turns the argument into a run and an attempt: a job of the
// run first, then an attempt id (or unique prefix) found among the stored logs.
func resolveLogTarget(ctx context.Context, st *store.Store, home, arg string, f logsFlags) (logTarget, error) {
	if st != nil {
		row, err := cirun.ResolveRun(ctx, st, f.run)
		if err == nil {
			if t, ok, jerr := jobTarget(ctx, st, row.ID, arg, f.attempt); jerr != nil || ok {
				return t, jerr
			}
		} else if f.run != "" {
			return logTarget{}, err
		}
	}
	if t, ok := attemptTarget(home, arg); ok {
		return t, nil
	}
	return logTarget{}, fmt.Errorf("%s: %q is neither a job of %s nor a stored attempt; list runs with `nself ci runs list`", logs.CodeNone, arg, runWord(f.run))
}

func runWord(ref string) string {
	if ref == "" {
		return "the latest run"
	}
	return "run " + ref
}

func jobTarget(ctx context.Context, st *store.Store, runID, arg string, n int) (logTarget, bool, error) {
	jobs, err := st.PipelineJobs(ctx, runID)
	if err != nil {
		return logTarget{}, false, err
	}
	for _, j := range jobs {
		if j.Name != arg && j.ID != arg {
			continue
		}
		atts, err := st.AttemptsByJob(ctx, j.ID)
		if err != nil {
			return logTarget{}, false, err
		}
		var pick *store.AttemptView
		for i := range atts {
			if (n == 0 && (pick == nil || atts[i].N > pick.N)) || atts[i].N == n {
				pick = &atts[i]
			}
		}
		if pick == nil {
			return logTarget{}, false, fmt.Errorf("%s: job %s has no attempt %d in run %s", logs.CodeNone, arg, n, runID)
		}
		return logTarget{run: runID, attempt: pick.ID}, true, nil
	}
	return logTarget{}, false, nil
}

// attemptTarget finds a stored attempt by id or unique prefix, newest run first.
func attemptTarget(home, arg string) (logTarget, bool) {
	if !logs.ValidID(arg) {
		return logTarget{}, false
	}
	runs, err := logs.ListRuns(home)
	if err != nil {
		return logTarget{}, false
	}
	var found []logTarget
	for i := len(runs) - 1; i >= 0; i-- {
		ids, _ := logs.Attempts(logs.RunDir(home, runs[i].Run))
		for _, a := range ids {
			if a == arg {
				return logTarget{run: runs[i].Run, attempt: a}, true
			}
			if strings.HasPrefix(a, arg) {
				found = append(found, logTarget{run: runs[i].Run, attempt: a})
			}
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return logTarget{}, false
}

// logPrinter writes lines as text or as stream records.
type logPrinter struct {
	json   bool
	tgt    logTarget
	n      int
	lastOK int64
}

func newLogPrinter(asJSON bool, t logTarget) *logPrinter { return &logPrinter{json: asJSON, tgt: t} }

func (p *logPrinter) line(l logs.Line) error {
	p.n, p.lastOK = p.n+1, l.Seq
	if !p.json {
		_, err := fmt.Fprintln(os.Stdout, l.Text)
		return err
	}
	b, err := output.StreamRecord("ci logs", "log",
		output.Field{Key: "run_id", Value: p.tgt.run}, output.Field{Key: "attempt_id", Value: p.tgt.attempt},
		output.Field{Key: "seq", Value: l.Seq}, output.Field{Key: "ts", Value: l.TS},
		output.Field{Key: "stream", Value: l.Stream}, output.Field{Key: "text", Value: l.Text})
	if err == nil {
		_, err = os.Stdout.Write(b)
	}
	return err
}

func (p *logPrinter) end() {
	if !p.json {
		return
	}
	if b, err := output.StreamEnd("ci logs", nil, output.Field{Key: "run_id", Value: p.tgt.run}, output.Field{Key: "attempt_id", Value: p.tgt.attempt},
		output.Field{Key: "lines", Value: p.n}, output.Field{Key: "last_seq", Value: p.lastOK}); err == nil {
		_, _ = os.Stdout.Write(b)
	}
}

// logsFail prints err (as a stream error line with --json) and returns the exit code.
func logsFail(f logsFlags, command string, err error) int {
	code, exit := "E632", 2
	msg := err.Error()
	if le, ok := logs.AsError(err); ok {
		code, msg = le.Code, le.Message
		if le.Code == logs.CodeNone {
			exit = 1
		}
	} else if strings.HasPrefix(msg, logs.CodeNone+":") {
		code, exit, msg = logs.CodeNone, 1, strings.TrimSpace(strings.TrimPrefix(msg, logs.CodeNone+":"))
	} else if cirun.CodeOf(err) != "" {
		code = cirun.CodeOf(err)
		if code == cirun.CodeRun {
			exit = 1
		}
	}
	if f.json {
		if b, e := output.StreamError(command, output.ErrorDetail{Code: code, Message: msg, ExitCode: exit}, nil); e == nil {
			_, _ = os.Stdout.Write(b)
			return exit
		}
	}
	fmt.Fprintf(os.Stderr, "%s: %s\n", code, msg)
	return exit
}
