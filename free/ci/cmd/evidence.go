package main

// Purpose: `nself ci evidence [run|rev] [--json]`, `nself ci evidence
// --pipeline <name>` and `nself ci evidence verify <rev> | --file <f>`
// (P7-CI-38, Epic D3, D11, D23). The questions are answered by
// internal/evidence; this file parses flags, builds the environment and prints.
// Inputs:  argv after the key; env NSELF_CI_REPO, NSELF_CI_HOME.
// Outputs: text, one v1 envelope (--json) or ci.events NDJSON (--stream);
// exit 0 ok; verify: 1 fails, 10 stale, 11 invalid, 12 unbound; 2 infra.
// Constraints: V15 only; RemoteResolvers are asked first; a read-only command
// never creates a store; a revision is a full sha; verify never runs in the working tree.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

type evidenceFlags struct {
	json, stream           bool
	repo, pipeline, policy string
	file, atRev            string
}

func newEvidenceFlagSet(f *evidenceFlags, verify bool) *flag.FlagSet {
	fs := flag.NewFlagSet("evidence", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	if verify {
		fs.BoolVar(&f.stream, "stream", false, "with --json: ci.events NDJSON, the last line the result")
		fs.StringVar(&f.policy, "policy", "required", "policy to verify under (required)")
		fs.StringVar(&f.file, "file", "", "check this evidence document instead of a revision")
		fs.StringVar(&f.atRev, "at-revision", "", "with --file: the revision the document must describe (default HEAD)")
	} else {
		fs.StringVar(&f.pipeline, "pipeline", "", "report on a named pipeline (overdue evidence: exit 10)")
	}
	return fs
}

func init() {
	registerV15("evidence", subcommandEntry{V15Only: true, Handler: runEvidence,
		Flags: func() *flag.FlagSet { return newEvidenceFlagSet(&evidenceFlags{}, false) }})
	registerV15("evidence verify", subcommandEntry{V15Only: true, SideEffect: SideEffectRead, Handler: runVerify,
		Flags: func() *flag.FlagSet { return newEvidenceFlagSet(&evidenceFlags{}, true) }})
}

// parseMixed accepts flags before and after the positional arguments.
func parseMixed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if args = fs.Args(); len(args) > 0 {
			pos, args = append(pos, args[0]), args[1:]
		}
	}
	return pos, nil
}

func evidenceRoot(repo string) (string, error) {
	if r := firstOf(repo, os.Getenv("NSELF_CI_REPO")); r != "" {
		return filepath.Abs(r)
	}
	return os.Getwd()
}

// openHistory opens the run store; a missing one is nil unless create is set.
func openHistory(create bool) (*store.Store, error) {
	if !create && !evidence.StoreExists(evidence.StorePath()) {
		return nil, nil
	}
	st, err := cirun.OpenStore(runsStorePath())
	if err != nil {
		return nil, err
	}
	_ = cirun.Recover(context.Background(), st, "", os.Getenv("NSELF_CI_HOME"))
	return st, nil
}

func evidenceEnv(root string, st *store.Store) evidence.Env {
	return evidence.Env{Root: root, Store: st, Policy: func(ctx context.Context) (string, error) {
		r, err := cirun.ResolvePolicy(ctx, root, "default")
		return r.Digest, err
	}}
}

// evidenceFail prints err in the mode the caller asked for and returns the exit code.
func evidenceFail(command string, f evidenceFlags, err error) int {
	var ee *evidence.Error
	if !errors.As(err, &ee) {
		if !f.json {
			return exitForError(err)
		}
		ee = &evidence.Error{Code: firstOf(cirun.CodeOf(err), evidence.CodeWorktree), Message: err.Error(), Exit: exitForError(err)}
	}
	ee.Exit = max(ee.Exit, 1)
	detail := output.ErrorDetail{Code: ee.Code, Message: ee.Message, ExitCode: ee.Exit}
	if line, e := output.StreamError(command, detail, nil); f.json && f.stream && e == nil {
		flushActiveStream() // queued records first, so the error line is the last one
		_, _ = os.Stdout.Write(line)
	} else if f.json {
		_ = output.WriteError(os.Stdout, command, detail, nil)
	} else {
		fmt.Fprintln(os.Stderr, ee)
	}
	return ee.Exit
}

func runEvidence(args []string) int {
	var f evidenceFlags
	pos, err := parseMixed(newEvidenceFlagSet(&f, false), args)
	if err != nil || len(pos) > 1 {
		return usageErr("nself ci evidence [run|rev] [--json] | nself ci evidence --pipeline <name> [--json]")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if f.pipeline != "" {
		return reportPipeline(ctx, f)
	}
	ref := strings.Join(pos, "")
	for _, r := range cirun.Registered.Remote {
		if v, ok, rerr := r.Resolve(ctx, ref); rerr == nil && ok {
			return printRun(f.json, v) // the run lives on another node: its evidence is read there
		}
	}
	root, err := evidenceRoot(f.repo)
	var st *store.Store
	if err == nil {
		st, err = openHistory(false)
	}
	if err != nil {
		return evidenceFail("ci evidence", f, err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	shown, err := evidence.Show(ctx, evidenceEnv(root, st), ref)
	if err != nil {
		return evidenceFail("ci evidence", f, err)
	}
	if f.json {
		_ = output.WriteData(os.Stdout, "ci evidence", shown.Evidence, nil)
		return 0
	}
	printEvidence(os.Stdout, shown.Run.ID, shown.Evidence)
	return 0
}

func printEvidence(w io.Writer, run string, ev model.Evidence) {
	fresh := string(ev.Freshness.State)
	if ev.Freshness.Reason != nil {
		fresh += " (" + *ev.Freshness.Reason + ")"
	}
	fmt.Fprintf(w, "run %s: %s, revision %.12s, %s, freshness %s\n", run, ev.Result, ev.Revision, ev.Binding.State, fresh)
	for _, c := range ev.Checks {
		fmt.Fprintf(w, "  %-6s %s\n", c.Result, c.ID)
	}
}

func usageErr(usage string) int {
	fmt.Fprintln(os.Stderr, "usage: "+usage)
	return 1
}

func reportPipeline(ctx context.Context, f evidenceFlags) int {
	rep, err := evidence.ReportPipeline(ctx, f.pipeline)
	if err != nil {
		return evidenceFail("ci evidence", f, err)
	}
	if f.json {
		_ = output.WriteData(os.Stdout, "ci evidence", rep, nil)
	} else {
		fmt.Printf("pipeline %s: overdue=%t\n", rep.Pipeline, rep.Overdue)
	}
	if rep.Overdue {
		return evidence.ExitStale
	}
	return 0
}

func runVerify(args []string) int {
	var f evidenceFlags
	pos, err := parseMixed(newEvidenceFlagSet(&f, true), args)
	if err != nil || (f.file == "") == (len(pos) == 0) || len(pos) > 1 || (f.stream && !f.json) || (f.atRev != "" && f.file == "") || f.policy != "required" {
		return usageErr("nself ci evidence verify <rev> | --file <f> [--at-revision <sha>] [--policy required] [--json [--stream]]")
	}
	const cmd = "ci evidence verify"
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stop()
	ignoreBrokenPipe() // a vanished consumer must not skip the worktree cleanup
	root, err := evidenceRoot(f.repo)
	var st *store.Store
	if err == nil {
		st, err = openHistory(f.file == "") // a file check never creates a store
	}
	if err != nil {
		return evidenceFail(cmd, f, err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	env := evidenceEnv(root, st)
	sess := &streamSession{w: os.Stdout, command: cmd}
	if f.stream {
		activeMu.Lock()
		activeSession = sess
		activeMu.Unlock()
		defer func() { activeMu.Lock(); activeSession = nil; activeMu.Unlock() }()
	}
	env.Run = verifyRun(f, os.Getenv("NSELF_CI_HOME"))
	var v evidence.Verdict
	if f.file != "" {
		v, err = verifyFile(ctx, env, f)
	} else {
		v, _, err = evidence.VerifyRevision(ctx, env, pos[0])
	}
	if err != nil {
		return evidenceFail(cmd, f, err)
	}
	return printVerdict(cmd, f, sess, v)
}

func printVerdict(command string, f evidenceFlags, sess *streamSession, v evidence.Verdict) int {
	var meta *output.Meta
	if len(v.Warnings) > 0 {
		meta = &output.Meta{Warnings: v.Warnings}
	}
	switch {
	case f.json && f.stream:
		sess.emitter(v.Report.Evidence.RunID).End(v.Report, meta)
	case f.json:
		_ = output.WriteData(os.Stdout, command, v.Report, meta)
	default:
		r := v.Report
		fmt.Printf("nself-ci: %.12s %s (class %s, %s, freshness %s)\n", r.Revision, r.Result, r.Class, r.Binding.State, r.Freshness.State)
		for _, why := range v.Reasons {
			fmt.Println("  " + string(why))
		}
		for _, w := range v.Warnings {
			fmt.Println("  note: " + w)
		}
	}
	return v.Exit
}

// verifyRun runs the required pipeline in a temporary worktree: no status is
// posted (machine-invoked, D29) and the run's own output goes to stderr, or
// to stdout as ci.events when --stream asked for it.
func verifyRun(f evidenceFlags, home string) evidence.RunFunc {
	return func(ctx context.Context, tree string) (model.Evidence, error) {
		o := cirun.Options{Root: tree, Home: home, NoStatus: true, Full: true}
		var out io.Writer = os.Stderr
		switch {
		case f.stream:
			o.JSON, o.Stream, o.Renderer, out = true, true, "ci-events", os.Stdout
		case f.json:
			out = io.Discard
		}
		p, digest, subst, err := preparePlan(ctx, o)
		if err != nil {
			return model.Evidence{}, err
		}
		c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, Substantive: subst, Out: out}
		res, err := c.Run(ctx)
		if err != nil {
			return model.Evidence{}, err
		}
		if res.Summary.Doc.Schema == "" {
			return model.Evidence{}, &evidence.Error{Code: evidence.CodeWorktree, Message: "the run finished but left no evidence", Exit: evidence.ExitInternal}
		}
		return res.Summary.Doc, nil
	}
}
