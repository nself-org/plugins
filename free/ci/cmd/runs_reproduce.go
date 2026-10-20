package main

// Purpose: `nself ci runs reproduce <job> [--run <run>] [--check] [--shell]
// [--json] [--repo dir]` (P7-CI-41, Epic D12/X18): re-run one job locally from
// its recorded inputs. The lookup, the drift report, the worktree and the
// link are internal/reproduce; this file parses flags, supplies the run
// engine (the same preparePlan and Coordinator `ci evidence verify` uses) and
// prints.
// Inputs:  argv after `reproduce`; env NSELF_CI_HOME, NSELF_CI_REPO.
// Outputs: the drift report on stderr before the job runs, then a summary or
// one v1 envelope; exit is the re-run's (0 pass, 1 a required check failed,
// 2 infra), 1 for a refusal (E634 platform, no such run or job, wrong
// repository) and 2 for infrastructure.
// Constraints: V15 only; --check reads and reports only; --shell makes the
// workspace (process level: a plain checkout of the commit, no container) and
// keeps it, because the exec funnel is the only place a process starts; no
// secret is fetched or injected.

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/nself-org/cli/sdk/go/v2/output"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/reproduce"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const reproduceUsage = "nself ci runs reproduce <job> [--run <run>] [--check] [--shell] [--json] [--repo dir]"

type reproduceFlags struct {
	json, check, shell bool
	repo, run          string
}

func newReproduceFlagSet(f *reproduceFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("reproduce", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.BoolVar(&f.check, "check", false, "report the recorded inputs and drift; run nothing")
	fs.BoolVar(&f.shell, "shell", false, "make the job workspace at the recorded commit and keep it (process level)")
	fs.StringVar(&f.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	fs.StringVar(&f.run, "run", "", "run id, unique prefix or latest (default: the newest run that has the job)")
	return fs
}

func init() {
	registerRunsSub("reproduce", runRunsReproduce)
}

func runRunsReproduce(args []string) int {
	var f reproduceFlags
	pos, err := parseMixed(newReproduceFlagSet(&f), args)
	if err != nil || len(pos) != 1 || (f.check && f.shell) {
		return usageErr(reproduceUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stop()
	ignoreBrokenPipe() // a vanished consumer must not skip the worktree cleanup
	root, err := evidenceRoot(f.repo)
	if err != nil {
		return reproduceFail(f, err)
	}
	st, err := openHistory(false)
	if err != nil {
		return reproduceFail(f, err)
	}
	if st != nil {
		defer func() { _ = st.Close() }()
	}
	home := os.Getenv("NSELF_CI_HOME")
	var runOut io.Writer = os.Stderr
	if f.json {
		runOut = io.Discard
	}
	env := reproduce.Env{Root: root, Home: home, Store: st, Run: reproduceRun(home, runOut),
		OnReport: func(r reproduce.Report) { printReproduceReport(os.Stderr, r) }}
	res, err := reproduce.Reproduce(ctx, env, reproduce.Request{Run: f.run, Job: pos[0], Check: f.check, Shell: f.shell})
	if err != nil {
		return reproduceFail(f, err)
	}
	return printReproduction(f, res)
}

// reproduceRun is the run engine of a reproduction: the recorded job (and the
// jobs it needs) planned and run in the temporary worktree, no status posted
// (machine-invoked, D29).
func reproduceRun(home string, out io.Writer) reproduce.RunFunc {
	return func(ctx context.Context, spec reproduce.RunSpec) (reproduce.RunOutcome, error) {
		o := cirun.Options{Root: spec.Tree, Home: home, NoStatus: true, Pipeline: spec.Pipeline, Job: spec.Job,
			Trigger: spec.Trigger, Container: spec.Container}
		p, digest, subst, err := preparePlan(ctx, o)
		if err != nil {
			return reproduce.RunOutcome{}, err
		}
		c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, Substantive: subst, Out: out}
		res, err := c.Run(ctx)
		if err != nil {
			return reproduce.RunOutcome{}, err
		}
		got := reproduce.RunOutcome{RunID: res.RunID, Exit: res.Exit, Evidence: res.Summary.Doc}
		for _, pj := range p.Jobs {
			if pj.ID == spec.Job {
				got.Services = pj.Job.NeedsServices
			}
		}
		if got.Evidence.Schema == "" {
			return got, &evidence.Error{Code: evidence.CodeWorktree, Message: "the run finished but left no evidence", Exit: evidence.ExitInternal}
		}
		return got, nil
	}
}

// reproduceFail prints an error and maps it to an exit code: a refusal the
// user can fix by choosing another run, job, repository or host is 1,
// anything else is what exitForError says (config and plan errors 1, infra 2).
func reproduceFail(f reproduceFlags, err error) int {
	if code, ok := reproduce.Refusal(err); ok {
		if !f.json {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return writeReproduceError(code, err, 1)
	}
	if !f.json {
		return exitForError(err)
	}
	return writeReproduceError(firstOf(cirun.CodeOf(err), evidence.CodeWorktree), err, 2)
}

func writeReproduceError(code string, err error, exit int) int {
	_ = output.WriteError(os.Stdout, "ci runs reproduce", output.ErrorDetail{Code: code, Message: err.Error(), ExitCode: exit}, nil)
	return exit
}

func printReproduceReport(w io.Writer, r reproduce.Report) {
	fmt.Fprintf(w, "reproduce %s at %.12s (%s, recorded on %s, now %s)\n", r.Job, r.Revision, r.Isolation, r.Platform, r.HostPlatform)
	for _, d := range r.Drift {
		switch d.Kind {
		case reproduce.DriftToolGone:
			fmt.Fprintf(w, "  drift: %s is not available here (recorded %q)\n", d.Name, d.Recorded)
		case reproduce.DriftIsolation:
			fmt.Fprintf(w, "  drift: recorded isolation %s, running as %s\n", d.Recorded, d.Current)
		default:
			fmt.Fprintf(w, "  drift: %s %s: recorded %q, now %q\n", d.Kind, d.Name, d.Recorded, d.Current)
		}
	}
	for _, m := range r.Warnings {
		fmt.Fprintln(w, "  warning: "+m)
	}
}

func printReproduction(f reproduceFlags, res reproduce.Result) int {
	if f.json {
		meta := &output.Meta{}
		meta.Warnings = res.Report.Warnings
		data := struct {
			Report       reproduce.Report `json:"report"`
			Workspace    string           `json:"workspace,omitempty"`
			Reproduction *reproduce.Link  `json:"link,omitempty"`
		}{res.Report, res.Workspace, res.Link}
		if err := output.WriteData(os.Stdout, "ci runs reproduce", data, meta); err != nil {
			return exitForError(err)
		}
		return res.Exit
	}
	switch {
	case res.Workspace != "":
		fmt.Printf("workspace %s (commit %.12s, process level; remove with: git -C <repo> worktree remove --force %s)\n", res.Workspace, res.Report.Revision, res.Workspace)
	case res.Link != nil:
		l := res.Link
		verdict := "differs from"
		if l.Reproduced {
			verdict = "matches"
		}
		fmt.Printf("%s: run %s %s the original %s (%s %s -> %s %s)\n", l.Job, l.Reproduction.RunID, verdict, l.Original.RunID,
			l.Original.Result, l.Original.FailureClass, l.Reproduction.Result, l.Reproduction.FailureClass)
	}
	return res.Exit
}
