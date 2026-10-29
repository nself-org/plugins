package main

// Purpose: `nself ci runs rerun <run|sha> [--where …] [--job id] [--json]
// [--repo dir]` (P7-AUTO-06): a new run of record for a recorded revision, with
// the same --where flag as bare `nself ci`.
// Inputs:  argv after `rerun`; env NSELF_CI_REPO, NSELF_CI_HOME. A run id (or
// unique prefix, or `latest`) resolves through the run history; a full sha
// without a record resolves through git.
// Outputs: the run's output as a bare run prints it, the new run id on stderr;
// the exit code is the gate's (0 pass, 1 failed, 2 infrastructure). A refusal
// (E626) exits 1.
// Constraints: V15 only; the CI-37 clean-tree rule holds: the checkout must be
// exactly the revision, clean, because a run binds to the tree it reads, so a
// dirty tree or another HEAD refuses with the reason. It sets Options.Record.
// The placement goes through the same options hook and Coordinator as a bare
// run; there is no second placement path.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"

	"github.com/nself-org/plugins/free/ci/internal/report"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

const rerunUsage = "nself ci runs rerun <run|sha> [--where local|<node>|<pool>|hosted|auto|all] [--job id] [--json] [--repo dir]"

var fullSHARe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

type rerunFlags struct {
	json            bool
	repo, job, here string
}

func newRerunFlagSet(f *rerunFlags) *flag.FlagSet {
	fs := flag.NewFlagSet("rerun", flag.ContinueOnError)
	fs.BoolVar(&f.json, "json", false, "print one v1 JSON envelope")
	fs.StringVar(&f.repo, "repo", "", "repository root (env NSELF_CI_REPO; default: current directory)")
	fs.StringVar(&f.job, "job", "", "run only this job and its dependencies")
	whereFlagSet(fs, &f.here)
	return fs
}

func init() { registerRunsSub("rerun", runRunsRerun) }

// rerunRevision resolves ref to the sha to run: a run id through lookup (the
// recorded revision), a full sha as given when exists says git knows it. The
// checkout (head) must be that sha and clean.
func rerunRevision(ref string, lookup func(string) (string, error), exists func(string) bool, head cirun.Revision) (string, error) {
	sha := ref
	if !fullSHARe.MatchString(ref) {
		got, err := lookup(ref)
		if err != nil {
			return "", err
		}
		sha = got
	} else if !exists(ref) {
		return "", &cirun.Error{Code: cirun.CodeRun, Message: fmt.Sprintf("%s is not a commit of this repository", ref[:12])}
	}
	switch {
	case head.SHA == "":
		return "", &cirun.Error{Code: cirun.CodeRun, Message: "the repository has no commit to run"}
	case head.Dirty:
		return "", &cirun.Error{Code: cirun.CodeRun, Message: "the worktree has uncommitted changes; a run binds to the tree it reads, so commit or stash first"}
	case head.SHA != sha:
		return "", &cirun.Error{Code: cirun.CodeRun, Message: fmt.Sprintf("the checkout is at %s, not %s; check out the revision first", short12(head.SHA), short12(sha))}
	}
	return sha, nil
}

func short12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func runRunsRerun(args []string) int {
	var f rerunFlags
	pos, err := parseMixed(newRerunFlagSet(&f), args)
	if err != nil || len(pos) != 1 {
		return usageErr(rerunUsage)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer stop()
	root, err := evidenceRoot(f.repo)
	if err != nil {
		return exitForError(err)
	}
	head := cirun.GitRevision(ctx, root, report.Capture)
	exists := func(sha string) bool {
		_, exit, e := report.Capture(ctx, "git", "-C", root, "cat-file", "-e", sha+"^{commit}")
		return e == nil && exit == 0
	}
	if _, err = rerunRevision(pos[0], rerunLookup(ctx), exists, head); err != nil {
		return exitForError(err)
	}
	o := RunOptions{Root: root, Home: os.Getenv("NSELF_CI_HOME"), Job: f.job, JSON: f.json, Record: true}
	o.Interactive = interactiveRun(o.Trigger, os.Getenv)
	if o.PolicyRun, err = schedRevisionValues(); err != nil {
		return exitForError(err)
	}
	v := f.here
	if v == "" {
		if k := whereKnown(ctx, false); k.HasRemote() {
			v = "auto"
		}
	}
	if v != "" {
		if err = applyWhere(&o, v, whereKnown(ctx, false)); err != nil {
			return exitForError(err)
		}
	}
	hooks := cirun.Hooks{}.Resolved()
	if err = o.Validate(hooks); err != nil {
		return exitForError(err)
	}
	p, digest, subst, err := preparePlan(ctx, o)
	if err != nil {
		return exitForError(err)
	}
	c := &cirun.Coordinator{Opts: o, Plan: p, ConfigDigest: digest, ConfigOutside: configOutside(ctx, o.Root), Substantive: subst, Out: os.Stdout}
	res, err := c.Run(ctx)
	if err != nil {
		return exitForError(err)
	}
	fmt.Fprintln(os.Stderr, "run "+res.RunID)
	return res.Exit
}

// rerunLookup finds the revision a recorded run was made for.
func rerunLookup(ctx context.Context) func(string) (string, error) {
	return func(ref string) (string, error) {
		st, err := openHistory(false)
		if err != nil {
			return "", err
		}
		if st == nil {
			return "", &cirun.Error{Code: cirun.CodeRun, Message: "this machine has no run history; pass a full sha"}
		}
		defer func() { _ = st.Close() }()
		row, err := cirun.ResolveRun(ctx, st, strings.TrimSpace(ref))
		if err != nil {
			return "", err
		}
		return row.Revision, nil
	}
}
