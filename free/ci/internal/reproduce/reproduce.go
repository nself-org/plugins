package reproduce

// Purpose: `nself ci runs reproduce <job>` (P7-CI-41): rebuild a job's inputs
// from stored evidence, report drift before running, re-run the job through
// the run engine in a temporary worktree of the exact recorded commit, and
// link the new evidence to the original.
// Inputs:  an Env (repository root, store, state dir, git access, the run
// engine as a RunFunc) and a Request (run reference, job id, check/shell).
// Outputs: a Result with the report, the new outcome and the link.
// Constraints: nothing here creates an attempt or calls the store's
// admission: the re-run is a Coordinator run supplied by cmd, whose attempts
// come from run.CreateAttempt (SCHED F15), so run.Dispatch sees them; no
// secret is fetched or injected (the engine applies the env policy and
// resolves secrets through P7-TRUST); the worktree is removed on every exit
// path except --shell, which keeps it on purpose.

import (
	"context"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RunSpec is what the run engine is asked to re-run.
type RunSpec struct {
	Tree      string // the temporary checkout of the recorded commit
	Job       string // planned job id (--job: the job and its dependencies)
	Pipeline  string // named pipeline to plan, "" for the default
	Trigger   string // the trigger the original was planned under
	Container bool   // the original ran in the toolchain container
}

// RunOutcome is what the engine returns.
type RunOutcome struct {
	RunID    string
	Exit     int
	Evidence model.Evidence
	Services []string // services the job needs (from the plan)
}

// RunFunc re-runs spec and returns the run-level evidence it recorded.
type RunFunc func(ctx context.Context, spec RunSpec) (RunOutcome, error)

// Env is what a reproduction needs.
type Env struct {
	Root     string // repository root
	Home     string // state directory ("" = NSELF_CI_HOME, else ~/.nself/ci)
	Store    *store.Store
	Git      evidence.Capture // nil: the exec funnel
	Probe    cirun.Capture    // nil: the exec funnel; reads tool versions
	Platform string           // "" = this host (tests set it)
	Run      RunFunc
	// OnReport, when set, receives the drift report before any worktree is
	// made or job is run (the command prints it there).
	OnReport func(Report)
	Now      func() time.Time
}

// Request names what to reproduce.
type Request struct {
	Run   string // run id, unique prefix or "latest"; "" searches the newest runs
	Job   string // planned job id
	Check bool   // report only: no worktree, no run
	Shell bool   // prepare the workspace and keep it; run nothing
}

// Result is the outcome of one reproduction.
type Result struct {
	Report Report
	// Workspace is the kept checkout of --shell, "" otherwise.
	Workspace    string
	Reproduction *Outcome
	Link         *Link
	// Exit is the exit code of the re-run (the run engine's, D11).
	Exit int
}

// Prepare locates the evidence and reads the drift report; it runs nothing.
func Prepare(ctx context.Context, env Env, req Request) (Source, Report, error) {
	src, err := Locate(ctx, env.Store, req.Run, req.Job)
	if err != nil {
		return Source{}, Report{}, err
	}
	rep, err := Preflight(ctx, env, src)
	return src, rep, err
}

// Reproduce re-runs one job from its recorded inputs.
func Reproduce(ctx context.Context, env Env, req Request) (res Result, err error) {
	src, rep, err := Prepare(ctx, env, req)
	res.Report = rep
	if err == nil && env.OnReport != nil {
		env.OnReport(rep)
	}
	if err != nil || req.Check {
		return res, err
	}
	if !req.Shell && env.Run == nil {
		return res, fail(evidence.CodeWorktree, "no run engine is available")
	}
	wt, err := evidence.AddWorktree(ctx, env.Git, env.Root, src.Doc.Revision)
	if err != nil {
		return res, err
	}
	keep := false
	defer func() { // a cleanup failure never replaces a result already reached
		if keep {
			return
		}
		if rerr := wt.Remove(); rerr != nil {
			if err == nil {
				res.Report.Warnings = append(res.Report.Warnings, "temporary worktree not fully removed: "+firstLine(rerr.Error()))
			}
		}
	}()
	if req.Shell {
		keep, res.Workspace = true, wt.Path
		return res, nil
	}
	out, err := env.Run(ctx, specOf(src, wt.Path))
	if err != nil {
		return res, err
	}
	res.Report.Services = out.Services
	again := Outcome{RunID: out.RunID, Result: "skip"}
	if r, class, ok := OutcomeOf(out.Evidence, src.JobID); ok {
		again.Result, again.FailureClass = r, class
	}
	again.EvidenceID = env.evidenceID(ctx, out.RunID)
	link := Link{Schema: LinkSchema, At: env.now().UTC().Format(time.RFC3339), Job: src.JobID, Revision: src.Doc.Revision,
		Original: src.Original, Reproduction: again, Drift: res.Report.Drift, Warnings: res.Report.Warnings,
		Reproduced: again.Result == src.Original.Result && again.FailureClass == src.Original.FailureClass}
	res.Reproduction, res.Link, res.Exit = &again, &link, out.Exit
	if ctx.Err() == nil && out.Evidence.Result != "cancelled" {
		if err = SaveLink(env.Home, link); err != nil {
			return res, err
		}
	}
	return res, nil
}

func specOf(src Source, tree string) RunSpec {
	spec := RunSpec{Tree: tree, Job: src.JobID, Trigger: string(src.Doc.Trigger), Container: src.Doc.Runner.Isolation == "container"}
	if k := src.Doc.Pipeline.Kind; k == "named" || k == "drill" {
		spec.Pipeline = src.Doc.Pipeline.Name
	}
	return spec
}

func (e Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

// evidenceID is the id of the newest run-level document of runID, "" when unknown.
func (e Env) evidenceID(ctx context.Context, runID string) string {
	if e.Store == nil || runID == "" {
		return ""
	}
	row, ok, err := e.Store.PipelineByID(ctx, runID)
	if err != nil || !ok {
		return ""
	}
	docs, err := evidence.ForRun(ctx, e.Store, row)
	if err != nil || len(docs) == 0 {
		return ""
	}
	return docs[0].ID
}
