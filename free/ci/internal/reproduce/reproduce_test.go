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
	if r, c, ok := OutcomeOf(ev, "b"); !ok || r != "fail" || c != "infra" {
		t.Errorf("b = %q %q %v", r, c, ok)
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
