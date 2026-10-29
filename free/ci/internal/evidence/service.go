package evidence

// Purpose: the three questions `ci evidence` answers, with no I/O of their own
// beyond git (through the funnel) and the store: show the evidence of a run or
// revision with freshness computed now; verify a revision (return the current
// bound evidence this machine holds, or run the required pipeline in a
// temporary worktree of exactly that commit); verify an evidence document.
// Inputs:  an Env (repository root, store, hooks).
// Outputs: documents, Verdicts and *Error values with their exit codes.
// Constraints: a revision is a full sha, never a ref (prohibited: HEAD, a
// prefix); verify never reads the working tree; evidence is returned only
// when it is bound, clean, fresh against the inputs read from git objects of
// that revision, from this repository and under the policy in force now;
// adhoc and drill evidence is never returned by revision.

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// RunFunc runs the required pipeline in the checkout at tree and returns the
// run-level evidence it recorded (cmd supplies it: planning lives there).
type RunFunc func(ctx context.Context, tree string) (model.Evidence, error)

// Env is what the service needs.
type Env struct {
	Root  string
	Store *store.Store // nil: this machine has no run history
	Git   Capture      // nil: the exec funnel
	// Policy returns the policy digest in force now; nil skips the policy check.
	Policy func(ctx context.Context) (string, error)
	Run    RunFunc
}

// Shown is `ci evidence <run|rev>`.
type Shown struct {
	Run      store.PipelineRow
	Evidence model.Evidence
}

// Show finds a run by id, unique prefix, "latest" or full revision and returns
// its run-level evidence with freshness computed against the checkout now.
func Show(ctx context.Context, env Env, ref string) (Shown, error) {
	if env.Store == nil {
		return Shown{}, fail(CodeNone, ExitFails, "this machine has no run history")
	}
	var row store.PipelineRow
	if fullRevision.MatchString(ref) {
		rows, err := env.Store.PipelinesByRevision(ctx, ref)
		if err != nil {
			return Shown{}, err
		}
		if len(rows) == 0 {
			return Shown{}, fail(CodeNone, ExitFails, "no run recorded for revision %s", short(ref))
		}
		row = rows[0]
	} else {
		r, err := cirun.ResolveRun(ctx, env.Store, ref)
		if err != nil {
			return Shown{}, err
		}
		row = r
	}
	docs, err := ForRun(ctx, env.Store, row)
	if err != nil {
		return Shown{}, err
	}
	if len(docs) == 0 {
		return Shown{}, fail(CodeNone, ExitFails, "run %s (%s) has no evidence yet", row.ID, row.Status)
	}
	ev := docs[0].Evidence
	ev.Freshness = freshNow(ctx, env, ev)
	return Shown{Run: row, Evidence: ev}, nil
}

// freshNow compares the document with the inputs of HEAD; an uncommitted
// change makes it stale, because the checkout is no longer that commit.
func freshNow(ctx context.Context, env Env, ev model.Evidence) model.Freshness {
	unknown := func(why string) model.Freshness { return model.Freshness{State: "unknown", Reason: &why} }
	if env.Root == "" {
		return unknown("no repository to compare with")
	}
	cur, err := InputsAt(ctx, env.Git, env.Root, "HEAD")
	if err != nil {
		return unknown("current inputs unreadable: " + firstLine(err.Error()))
	}
	f := attest.Fresh(ev, cur)
	if f.State == "current" && ReadWorktree(ctx, env.Git, env.Root).Dirty {
		why := "the worktree has uncommitted changes"
		return model.Freshness{State: "stale", Reason: &why}
	}
	return f
}

// VerifyRevision returns the verdict for rev. ran is true when it had to run
// the pipeline.
func VerifyRevision(ctx context.Context, env Env, rev string) (v Verdict, ran bool, err error) {
	if _, err = ParseRevision(rev); err != nil {
		return v, false, err
	}
	if !CommitExists(ctx, env.Git, env.Root, rev) {
		return v, false, fail(CodeUsage, ExitFails, "%s is not a commit of this repository", short(rev))
	}
	tree, err := TreeFS(ctx, env.Git, env.Root, rev)
	if err != nil {
		return v, false, err
	}
	current, err := InputsOf(tree)
	if err != nil {
		return v, false, err
	}
	if v, ok := env.held(ctx, rev, current); ok {
		return v, false, nil
	}
	if env.Run == nil {
		return v, false, fail(CodeWorktree, ExitInternal, "no run engine is available")
	}
	wt, err := AddWorktree(ctx, env.Git, env.Root, rev)
	if err != nil {
		return v, false, err
	}
	defer func() { // a cleanup failure never replaces a verdict already reached
		if rerr := wt.Remove(); rerr != nil {
			if err == nil && v.Report.Revision != "" {
				v.Warnings = append(v.Warnings, "temporary worktree not fully removed: "+firstLine(rerr.Error()))
			} else if err == nil {
				err = rerr
			}
		}
	}()
	ev, err := env.Run(ctx, wt.Path)
	if err != nil {
		return v, true, err
	}
	if ctx.Err() != nil || ev.Result == "cancelled" || ev.Result == "error" {
		// an interrupt or an infrastructure error is not a verdict on the revision: exit 2, never the "fails" exit of a red build
		return v, true, fail(CodeWorktree, ExitInternal, "the run for %s did not finish (%s)", short(rev), ev.Result)
	}
	v = Judge(ev, rev, &current)
	env.checkScope(ctx, &v) // a rerun is held to the same repository and policy as a stored document
	return v, true, nil
}

// held is the evidence on this machine that already answers for rev.
func (env Env) held(ctx context.Context, rev string, current model.InputDigests) (Verdict, bool) {
	if env.Store == nil {
		return Verdict{}, false
	}
	docs, err := ForRevision(ctx, env.Store, rev)
	if err != nil {
		return Verdict{}, false
	}
	repo, _ := RepoID(ctx, env.Git, env.Root)
	policy := ""
	if env.Policy != nil {
		p, perr := env.Policy(ctx)
		if perr != nil {
			return Verdict{}, false
		}
		policy = p
	}
	for _, d := range docs {
		// only a finished, full-selection verdict answers: a cancelled, errored or affected-only run says nothing about rev
		if (d.Evidence.Result != "pass" && d.Evidence.Result != "fail") || d.Evidence.Selection.Mode != "full" || d.Evidence.Pipeline.Kind != "default" || repo == "" || d.Evidence.RepoID != repo || (env.Policy != nil && d.Evidence.PolicyDigest != policy) {
			continue
		}
		if v := Judge(d.Evidence, rev, &current); v.Exit == ExitOK || v.Exit == ExitFails {
			return v, true
		}
	}
	return Verdict{}, false
}

// ReadDocument reads an evidence file, bounded by attest.MaxDocument.
func ReadDocument(r io.Reader) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, attest.MaxDocument+1))
	if err != nil {
		return nil, fail(CodeRejected, ExitInvalid, "cannot read the evidence file: %v", err)
	}
	if len(b) > attest.MaxDocument {
		return nil, fail(CodeRejected, ExitInvalid, "the evidence file is larger than %d bytes", attest.MaxDocument)
	}
	return b, nil
}

// VerifyFile checks an external document. atRev names the revision the
// document must describe (default: HEAD of the repository); the inputs are
// read from the git objects of that revision, never the working tree.
func VerifyFile(ctx context.Context, env Env, doc []byte, atRev string) (Verdict, error) {
	want := atRev
	if want == "" {
		out, ok := git(ctx, env.Git, env.Root, "rev-parse", "HEAD")
		want = strings.TrimSpace(out)
		if !ok || env.Root == "" {
			return Verdict{}, fail(CodeUsage, ExitFails, "not inside a repository: pass --at-revision <sha>")
		}
	}
	if _, err := ParseRevision(want); err != nil {
		return Verdict{}, err
	}
	var current *model.InputDigests
	var missing []string
	var warn []string
	t, terr := TreeFS(ctx, env.Git, env.Root, want)
	if terr == nil {
		if d, derr := InputsOf(t); derr == nil {
			current = &d
		} else {
			warn = append(warn, firstLine(derr.Error()))
		}
	} else {
		warn = append(warn, "revision "+short(want)+" is not available in this repository")
	}
	v, err := VerifyDocument(doc, want, current, env.recorded(ctx))
	if err != nil {
		return v, err
	}
	if current == nil { // the revision cannot be read: nothing can be proven fresh or stale
		v.reject(attest.ReasonInvalid, "the inputs of "+short(want)+" cannot be read in this repository")
	}
	env.checkScope(ctx, &v)
	if terr == nil {
		missing = MissingInputs(v.Report.Evidence, t)
	}
	v.MarkMissing(missing)
	v.Warnings = append(v.Warnings, warn...)
	return v, nil
}

// checkScope refuses a document made for another repository, and marks one
// made under another policy as stale (--policy required: the policy in force now).
func (env Env) checkScope(ctx context.Context, v *Verdict) {
	ev := v.Report.Evidence
	if id, ok := RepoID(ctx, env.Git, env.Root); !ok || ev.RepoID != id {
		v.reject(attest.ReasonRepoMismatch, "the document was made for another repository")
	}
	if env.Policy == nil {
		return
	}
	if p, err := env.Policy(ctx); err != nil || ev.PolicyDigest != p {
		v.Reasons = append(v.Reasons, attest.ReasonPolicyMismatch)
		v.Warnings = append(v.Warnings, "the document was made under another policy than the one in force")
		if v.Exit == ExitOK {
			v.Exit = ExitStale
		}
	}
}

// recorded looks a run up in the store, without creating one.
func (env Env) recorded(ctx context.Context) func(model.Evidence) *model.Evidence {
	if env.Store == nil {
		return nil
	}
	return func(ev model.Evidence) *model.Evidence {
		row, ok, err := env.Store.PipelineByID(ctx, ev.RunID)
		if err != nil || !ok {
			return nil
		}
		docs, err := ForRun(ctx, env.Store, row)
		if err != nil || len(docs) == 0 {
			return nil
		}
		return &docs[matchingDoc(ev, docs)].Evidence
	}
}

// StorePath is the state.db path a command uses when NSELF_CI_HOME names none.
func StorePath() string {
	home := os.Getenv("NSELF_CI_HOME")
	if home == "" {
		user, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(user, ".nself", "ci")
	}
	return filepath.Join(home, "state.db")
}

// StoreExists reports whether a state.db is present at path, so a read-only
// command never creates one.
func StoreExists(path string) bool {
	if path == "" {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}
