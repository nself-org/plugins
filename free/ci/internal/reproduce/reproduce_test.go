package reproduce

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/model"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

// original runs the failing job once in repo and returns its run id.
func original(t *testing.T, repo, home string) string {
	t.Helper()
	res := runIn(t, repo, home, "", buildPlan(t, failingJobs()))
	if res.Exit == 0 {
		t.Fatal("the fixture job should fail")
	}
	return res.RunID
}

func TestReproduceLinks(t *testing.T) {
	repo, sha := newRepo(t)
	home := t.TempDir()
	orun := original(t, repo, home)
	eng := &engine{t: t, home: home, p: buildPlan(t, failingJobs())}
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: eng.run}

	res, err := Reproduce(context.Background(), env, Request{Job: "unit"})
	if err != nil {
		t.Fatalf("Reproduce: %v", err)
	}
	l := res.Link
	if l == nil || !l.Reproduced {
		t.Fatalf("the failure was not reproduced: %+v", res)
	}
	if l.Original.FailureClass != "code" || l.Reproduction.FailureClass != l.Original.FailureClass {
		t.Fatalf("failure classes differ: original %q, reproduction %q", l.Original.FailureClass, l.Reproduction.FailureClass)
	}
	if l.Original.RunID != orun || l.Reproduction.RunID == "" || l.Reproduction.RunID == orun {
		t.Fatalf("link run ids: %+v", l)
	}
	if l.Original.EvidenceID == "" || l.Reproduction.EvidenceID == "" || l.Original.EvidenceID == l.Reproduction.EvidenceID {
		t.Fatalf("both evidence records must be linked: %+v", l)
	}
	if l.Revision != sha || res.Exit == 0 {
		t.Fatalf("revision %q (want %q), exit %d", l.Revision, sha, res.Exit)
	}
	saved, err := Links(home, orun)
	if err != nil || len(saved) != 1 || saved[0].Reproduction.RunID != l.Reproduction.RunID {
		t.Fatalf("stored links = %+v, %v", saved, err)
	}
	if got := eng.seen[0]; got.Job != "unit" || got.Tree == repo || got.Container {
		t.Fatalf("spec = %+v", got)
	}
	if wt := worktrees(t, repo); len(wt) != 1 {
		t.Fatalf("the temporary worktree was not removed: %v", wt)
	}
}

func TestPlatformRefused(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	original(t, repo, home)
	eng := &engine{t: t, home: home, p: buildPlan(t, failingJobs())}
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: eng.run, Platform: "plan9/mips"}

	res, err := Reproduce(context.Background(), env, Request{Job: "unit"})
	if cirun.CodeOf(err) != CodePlatform {
		t.Fatalf("want %s, got %v", CodePlatform, err)
	}
	if !strings.Contains(err.Error(), HostPlatform()) || !strings.Contains(err.Error(), "plan9/mips") {
		t.Fatalf("the error must name both platforms: %v", err)
	}
	if res.Report.Platform != HostPlatform() || len(eng.seen) != 0 || len(worktrees(t, repo)) != 1 {
		t.Fatalf("a refused job must not run or leave a worktree: %+v seen=%d", res.Report, len(eng.seen))
	}
}

func TestUnboundWarns(t *testing.T) {
	repo, sha := newRepo(t)
	home := t.TempDir()
	writeFile(t, repo, "a.txt", "uncommitted edit\n") // the original ran on an edited tree
	original(t, repo, home)
	gitRun(t, repo, "checkout", "--", "a.txt")
	eng := &engine{t: t, home: home, p: buildPlan(t, failingJobs())}
	var reported []Report
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: eng.run, OnReport: func(r Report) { reported = append(reported, r) }}

	res, err := Reproduce(context.Background(), env, Request{Job: "unit"})
	if err != nil {
		t.Fatalf("an unbound original is a warning, not an error: %v", err)
	}
	if res.Report.Bound || len(res.Report.Warnings) == 0 || !strings.HasPrefix(res.Report.Warnings[0], CodeUnbound+":") {
		t.Fatalf("want an %s warning: %+v", CodeUnbound, res.Report)
	}
	if res.Link.Revision != sha || len(reported) != 1 {
		t.Fatalf("revision %q, reports %d", res.Link.Revision, len(reported))
	}
}

func TestDispatchHookReached(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	original(t, repo, home)
	var hits atomic.Int32
	cirun.Dispatch = func(ctx context.Context, s cirun.AttemptSpec) (cirun.Admitted, error) {
		hits.Add(1)
		return cirun.LocalAdmit(ctx, s)
	}
	t.Cleanup(func() { cirun.Dispatch = nil })
	eng := &engine{t: t, home: home, p: buildPlan(t, failingJobs())}
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: eng.run}

	if _, err := Reproduce(context.Background(), env, Request{Job: "unit"}); err != nil {
		t.Fatal(err)
	}
	if hits.Load() == 0 {
		t.Fatal("the reproduction attempt never reached run.Dispatch")
	}
}

func TestCheckRunsNothing(t *testing.T) {
	repo, sha := newRepo(t)
	home := t.TempDir()
	orun := original(t, repo, home)
	eng := &engine{t: t, home: home, p: buildPlan(t, failingJobs())}
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: eng.run}

	res, err := Reproduce(context.Background(), env, Request{Run: orun[:8], Job: "unit", Check: true})
	if err != nil || res.Link != nil || len(eng.seen) != 0 || res.Report.Revision != sha || len(worktrees(t, repo)) != 1 {
		t.Fatalf("--check must only report: %+v err=%v", res, err)
	}
}

func TestShellKeepsWorkspace(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	original(t, repo, home)
	env := Env{Root: repo, Home: home, Store: openStore(t, home)}

	res, err := Reproduce(context.Background(), env, Request{Job: "unit", Shell: true})
	if err != nil || res.Workspace == "" {
		t.Fatalf("shell: %+v %v", res, err)
	}
	t.Cleanup(func() { gitRun(t, repo, "worktree", "remove", "--force", res.Workspace) })
	if b, err := os.ReadFile(filepath.Join(res.Workspace, "a.txt")); err != nil || string(b) != "committed\n" {
		t.Fatalf("workspace is not the recorded commit: %q %v", b, err)
	}
}

func TestLocateRefusals(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	original(t, repo, home)
	st := openStore(t, home)
	for name, tc := range map[string]struct{ run, job string }{
		"unknown job": {"", "nope"}, "unknown run": {"zzz", "unit"}, "no job": {"", ""},
	} {
		_, err := Locate(context.Background(), st, tc.run, tc.job)
		var re *Error
		var rr *cirun.Error
		if err == nil || (!errors.As(err, &re) && !errors.As(err, &rr)) {
			t.Errorf("%s: want a coded error, got %v", name, err)
		}
	}
	if _, err := Locate(context.Background(), nil, "", "unit"); cirun.CodeOf(err) != evidence.CodeNone {
		t.Errorf("no history: %v", err)
	}
}

func TestOutcomeOf(t *testing.T) {
	ev := model.Evidence{Checks: []model.Check{
		{JobID: "a", Result: "pass"},
		{JobID: "b", Result: "error", FailureClass: "infra"},
		{JobID: "b", Result: "fail", FailureClass: "code"},
	}}
	if r, c, ok := OutcomeOf(ev, "b"); !ok || r != "fail" || c != "code" {
		t.Errorf("b = %q %q %v", r, c, ok)
	}
	rev := model.Evidence{Checks: []model.Check{
		{JobID: "d", Result: "fail", FailureClass: "code"},
		{JobID: "d", Result: "error", FailureClass: "infra"},
	}}
	if r, c, _ := OutcomeOf(rev, "d"); r != "fail" || c != "code" {
		t.Errorf("a later errored check must not change the failed one: %q %q", r, c)
	}
	if r, _, ok := OutcomeOf(ev, "a"); !ok || r != "pass" {
		t.Errorf("a = %q %v", r, ok)
	}
	if _, _, ok := OutcomeOf(ev, "c"); ok {
		t.Error("c has no checks")
	}
}

func TestRefusal(t *testing.T) {
	for _, err := range []error{fail(CodePlatform, "x"), fail(evidence.CodeNone, "x"), fail(evidence.CodeUsage, "x"), fail(evidence.CodeRejected, "x")} {
		if _, ok := Refusal(err); !ok {
			t.Errorf("%v must be a refusal", err)
		}
	}
	for _, err := range []error{fail(evidence.CodeWorktree, "git"), errors.New("disk full"), nil} {
		if code, ok := Refusal(err); ok {
			t.Errorf("%v must not be a refusal (%s)", err, code)
		}
	}
}

// tempTrees lists the temporary worktree directories AddWorktree made under
// tmp (the test points TMPDIR at it).
func tempTrees(t *testing.T, tmp string) []string {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(tmp, "nself-ci-verify-*"))
	if err != nil {
		t.Fatal(err)
	}
	return left
}

// A run engine that errors must not leave the temporary worktree, its git
// bookkeeping or a link behind, and the engine's error is the one returned.
func TestRunErrorRemovesWorktree(t *testing.T) {
	repo, _ := newRepo(t)
	home := t.TempDir()
	orun := original(t, repo, home)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	boom := errors.New("engine exploded")
	var tree string
	env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: func(_ context.Context, spec RunSpec) (RunOutcome, error) {
		tree = spec.Tree
		if _, err := os.Stat(filepath.Join(tree, "a.txt")); err != nil {
			t.Errorf("the run must start in a populated worktree: %v", err)
		}
		return RunOutcome{}, boom
	}}

	res, err := Reproduce(context.Background(), env, Request{Job: "unit"})
	if !errors.Is(err, boom) {
		t.Fatalf("want the engine error, got %v", err)
	}
	if res.Link != nil || res.Reproduction != nil {
		t.Fatalf("a failed run must not produce a link: %+v", res)
	}
	if links, _ := Links(home, orun); len(links) != 0 {
		t.Fatalf("a link was saved for a failed run: %+v", links)
	}
	if tree == "" {
		t.Fatal("the engine was never reached")
	}
	if _, statErr := os.Stat(tree); !os.IsNotExist(statErr) {
		t.Fatalf("the temporary worktree %s survived the failed run (%v)", tree, statErr)
	}
	if left := tempTrees(t, tmp); len(left) != 0 {
		t.Fatalf("temporary directories left behind: %v", left)
	}
	if wt := worktrees(t, repo); len(wt) != 1 {
		t.Fatalf("git still lists the temporary worktree: %v", wt)
	}
}

// Cancelling mid-run (SIGINT in the command) must remove the worktree even
// though the context the run was started with is already done, and must not
// save a link for the interrupted run.
func TestCancelMidRunRemovesWorktree(t *testing.T) {
	for name, finish := range map[string]func(ctx context.Context) (RunOutcome, error){
		"engine returns the context error": func(ctx context.Context) (RunOutcome, error) { return RunOutcome{}, ctx.Err() },
		"engine reports a cancelled run": func(ctx context.Context) (RunOutcome, error) {
			return RunOutcome{RunID: "r-cancelled", Exit: 2, Evidence: model.Evidence{Result: "cancelled"}}, nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			repo, _ := newRepo(t)
			home := t.TempDir()
			orun := original(t, repo, home)
			tmp := t.TempDir()
			t.Setenv("TMPDIR", tmp)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var tree string
			started := make(chan struct{})
			env := Env{Root: repo, Home: home, Store: openStore(t, home), Run: func(ctx context.Context, spec RunSpec) (RunOutcome, error) {
				tree = spec.Tree
				close(started)
				<-ctx.Done()
				return finish(ctx)
			}}
			go func() { <-started; cancel() }()

			res, _ := Reproduce(ctx, env, Request{Job: "unit"})
			if tree == "" {
				t.Fatal("the engine was never reached")
			}
			if _, err := os.Stat(tree); !os.IsNotExist(err) {
				t.Fatalf("the temporary worktree %s survived the cancel (%v)", tree, err)
			}
			if left := tempTrees(t, tmp); len(left) != 0 {
				t.Fatalf("temporary directories left behind: %v", left)
			}
			if wt := worktrees(t, repo); len(wt) != 1 {
				t.Fatalf("git still lists the temporary worktree: %v", wt)
			}
			if links, _ := Links(home, orun); len(links) != 0 {
				t.Fatalf("a link was saved for a cancelled run: %+v", links)
			}
			_ = res
		})
	}
}

// specOf carries the recorded pipeline, trigger and isolation to the engine.
func TestSpecOfPassThrough(t *testing.T) {
	doc := func(kind model.PipelineKind, name string, iso model.Isolation) Source {
		var s Source
		s.JobID = "unit"
		s.Doc.Trigger = "push"
		s.Doc.Pipeline.Kind, s.Doc.Pipeline.Name = kind, name
		s.Doc.Runner.Isolation = iso
		return s
	}
	for name, tc := range map[string]struct {
		src  Source
		want RunSpec
	}{
		"named pipeline":  {doc("named", "release", "process"), RunSpec{Tree: "/t", Job: "unit", Trigger: "push", Pipeline: "release"}},
		"drill pipeline":  {doc("drill", "restore", "process"), RunSpec{Tree: "/t", Job: "unit", Trigger: "push", Pipeline: "restore"}},
		"default":         {doc("default", "default", "process"), RunSpec{Tree: "/t", Job: "unit", Trigger: "push"}},
		"container level": {doc("default", "default", "container"), RunSpec{Tree: "/t", Job: "unit", Trigger: "push", Container: true}},
	} {
		if got := specOf(tc.src, "/t"); got != tc.want {
			t.Errorf("%s: spec = %+v, want %+v", name, got, tc.want)
		}
	}
}
