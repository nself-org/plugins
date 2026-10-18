package evidence

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/plan"
	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// rig is one repository, one run store and a counting run engine.
type rig struct {
	t       *testing.T
	dir     string
	home    string
	st      *store.Store
	runs    int
	policy  string
	plan    *plan.Plan
	onRun   func(tree string)
	failRun error
}

func newRig(t *testing.T) (*rig, string) {
	t.Helper()
	t.Setenv("TMPDIR", t.TempDir())
	dir, sha := newRepo(t, baseFiles)
	r := &rig{t: t, dir: dir, home: t.TempDir(), policy: "pol-1", plan: buildPlan(t, jobsOf("true"))}
	_, r.st = runPipeline(t, dir, r.home, r.plan, cirun.Hooks{Policy: fixedPolicy(r.policy)})
	return r, sha
}

func (r *rig) env() Env {
	return Env{Root: r.dir, Store: r.st, Policy: func(context.Context) (string, error) { return r.policy, nil },
		Run: func(_ context.Context, tree string) (model.Evidence, error) {
			r.runs++
			if r.onRun != nil {
				r.onRun(tree)
			}
			if r.failRun != nil {
				return model.Evidence{}, r.failRun
			}
			c := &cirun.Coordinator{Opts: cirun.Options{Root: tree, Home: r.home, NoStatus: true}, Plan: r.plan, ConfigDigest: "cfg",
				Hooks: cirun.Hooks{Policy: fixedPolicy(r.policy)}, Out: nil, Capacity: roomy}
			res, err := c.Run(context.Background())
			if err != nil {
				return model.Evidence{}, err
			}
			return res.Summary.Doc, nil
		}}
}

func TestVerifyRevisionReturnsHeldEvidenceWithoutRunning(t *testing.T) {
	r, sha := newRig(t)
	v, ran, err := VerifyRevision(context.Background(), r.env(), sha)
	ok(t, err)
	if ran || r.runs != 0 || v.Exit != ExitOK || v.Report.Evidence.Revision != sha || v.Report.Class != ClassDefault {
		t.Fatalf("ran=%v runs=%d verdict %+v", ran, r.runs, v)
	}
	write(t, r.dir, "go.sum", "edited but uncommitted\n") // the working tree plays no part
	if v, ran, err = VerifyRevision(context.Background(), r.env(), sha); err != nil || ran || v.Exit != ExitOK {
		t.Fatalf("an uncommitted edit changed the answer: ran=%v %+v %v", ran, v, err)
	}
}

func TestVerifyWorktree(t *testing.T) {
	r, _ := newRig(t)
	sha2 := commitFiles(t, r.dir, map[string]string{"go.sum": "h1:two\n"})
	write(t, r.dir, "a.txt", "uncommitted edit\n")
	write(t, r.dir, "untracked.txt", "untracked\n")
	var seen string
	r.onRun = func(tree string) {
		seen = tree
		if b, err := os.ReadFile(filepath.Join(tree, "go.sum")); err != nil || string(b) != "h1:two\n" {
			t.Errorf("the temporary worktree is not the commit: %q %v", b, err)
		}
		if b, _ := os.ReadFile(filepath.Join(tree, "a.txt")); string(b) != "a\n" {
			t.Errorf("an uncommitted edit reached the run: %q", b)
		}
		if _, err := os.Stat(filepath.Join(tree, "untracked.txt")); err == nil {
			t.Error("an untracked file reached the run")
		}
		if tree == r.dir {
			t.Error("verify ran in the working tree")
		}
	}
	v, ran, err := VerifyRevision(context.Background(), r.env(), sha2)
	ok(t, err)
	if !ran || r.runs != 1 || v.Exit != ExitOK || v.Report.Evidence.Revision != sha2 || v.Report.Binding.State != "bound" {
		t.Fatalf("ran=%v runs=%d %+v", ran, r.runs, v)
	}
	if _, err = os.Stat(seen); !os.IsNotExist(err) {
		t.Fatalf("the temporary worktree is still there: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "nself-ci-verify-*")); len(left) != 0 {
		t.Fatalf("leftovers: %v", left)
	}
	if n := len(strings.Split(gitRun(t, r.dir, "worktree", "list"), "\n")); n != 1 {
		t.Fatalf("%d worktrees registered", n)
	}
	if _, ran, err = VerifyRevision(context.Background(), r.env(), sha2); err != nil || ran || r.runs != 1 {
		t.Fatalf("the new evidence was not reused: ran=%v runs=%d %v", ran, r.runs, err)
	}
}

func TestVerifyRevisionIgnoresEvidenceThatCannotGate(t *testing.T) {
	for name, tc := range map[string]func(r *rig){
		"adhoc evidence":    func(r *rig) { r.plan.Pipeline.Kind = "adhoc" },
		"dirty tree":        func(r *rig) { write(r.t, r.dir, "a.txt", "dirty\n") },
		"another policy":    func(r *rig) { r.policy = "pol-2" },
		"a drill":           func(r *rig) { r.plan.Pipeline.Kind = "drill" },
		"stale inputs only": func(r *rig) { commitFiles(r.t, r.dir, map[string]string{"go.sum": "x\n"}) },
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			dir, _ := newRepo(t, baseFiles)
			r := &rig{t: t, dir: dir, home: t.TempDir(), policy: "pol-1", plan: buildPlan(t, jobsOf("true"))}
			switch name {
			case "adhoc evidence", "a drill", "dirty tree":
				if name == "dirty tree" {
					write(t, dir, "a.txt", "dirty\n")
				} else {
					tc(r)
				}
				_, r.st = runPipeline(t, dir, r.home, r.plan, cirun.Hooks{Policy: fixedPolicy("pol-1")})
				if name == "dirty tree" {
					gitRun(t, dir, "checkout", "-q", "a.txt")
				}
				r.plan = buildPlan(t, jobsOf("true"))
			default:
				_, r.st = runPipeline(t, dir, r.home, r.plan, cirun.Hooks{Policy: fixedPolicy("pol-1")})
				tc(r)
			}
			rev := gitRun(t, dir, "rev-parse", "HEAD")
			v, ran, err := VerifyRevision(context.Background(), r.env(), rev)
			ok(t, err)
			if !ran || r.runs != 1 || v.Exit != ExitOK {
				t.Fatalf("%s must not be returned: ran=%v runs=%d %+v", name, ran, r.runs, v)
			}
		})
	}
}

func TestVerifyRevisionReturnsAFailingResultWithoutRerunning(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir, sha := newRepo(t, baseFiles)
	r := &rig{t: t, dir: dir, home: t.TempDir(), policy: "pol-1", plan: buildPlan(t, jobsOf("false"))}
	_, r.st = runPipeline(t, dir, r.home, r.plan, cirun.Hooks{Policy: fixedPolicy("pol-1")})
	v, ran, err := VerifyRevision(context.Background(), r.env(), sha)
	ok(t, err)
	if ran || v.Exit != ExitFails || v.Report.Result != "fail" {
		t.Fatalf("ran=%v %+v", ran, v)
	}
}

func TestVerifyRevisionRefusals(t *testing.T) {
	r, sha := newRig(t)
	for _, bad := range []string{"HEAD", "main", sha[:12], strings.Repeat("0", 40), strings.ToUpper(sha)} {
		_, _, err := VerifyRevision(context.Background(), r.env(), bad)
		if e, isErr := AsError(err); !isErr || e.Code != CodeUsage {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if r.runs != 0 {
		t.Fatal("a refused revision ran the pipeline")
	}
	sha2 := commitFiles(t, r.dir, map[string]string{"go.sum": "n\n"})
	r.failRun = &Error{Code: CodeWorktree, Message: "boom", Exit: ExitInternal}
	if _, ran, err := VerifyRevision(context.Background(), r.env(), sha2); err == nil || !ran {
		t.Fatalf("run failure lost: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(os.Getenv("TMPDIR"), "nself-ci-verify-*")); len(left) != 0 {
		t.Fatalf("a failed run left %v", left)
	}
}

func TestShow(t *testing.T) {
	r, sha := newRig(t)
	ctx := context.Background()
	got, err := Show(ctx, r.env(), "latest")
	ok(t, err)
	if got.Evidence.Revision != sha || got.Evidence.Freshness.State != "current" {
		t.Fatalf("fresh run: %+v", got.Evidence.Freshness)
	}
	if byRev, err := Show(ctx, r.env(), sha); err != nil || byRev.Run.ID != got.Run.ID {
		t.Fatalf("by revision: %v", err)
	}
	if byPrefix, err := Show(ctx, r.env(), got.Run.ID[:12]); err != nil || byPrefix.Run.ID != got.Run.ID {
		t.Fatalf("by prefix: %v", err)
	}
	write(t, r.dir, "a.txt", "uncommitted\n")
	if d, _ := Show(ctx, r.env(), got.Run.ID); d.Evidence.Freshness.State != "stale" {
		t.Fatalf("an uncommitted change must read stale: %+v", d.Evidence.Freshness)
	}
	gitRun(t, r.dir, "checkout", "-q", "a.txt")
	commitFiles(t, r.dir, map[string]string{"go.sum": "changed\n"})
	d, err := Show(ctx, r.env(), got.Run.ID)
	ok(t, err)
	if d.Evidence.Freshness.State != "stale" || d.Evidence.Freshness.Reason == nil || !strings.Contains(*d.Evidence.Freshness.Reason, "go.sum") {
		t.Fatalf("a lockfile change must read stale: %+v", d.Evidence.Freshness)
	}
	for _, ref := range []string{"nope", strings.Repeat("0", 40)} {
		if _, err = Show(ctx, r.env(), ref); err == nil {
			t.Errorf("%q resolved", ref)
		}
	}
	env := r.env()
	env.Store = nil
	if _, err = Show(ctx, env, "latest"); err == nil {
		t.Fatal("no history must be an error")
	}
}

// A run cut short (Ctrl-C) leaves a stored document, but it answers nothing:
// verify runs the pipeline instead of returning it for ever.
func TestVerifyRevisionIgnoresACancelledRun(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	dir, sha := newRepo(t, baseFiles)
	r := &rig{t: t, dir: dir, home: t.TempDir(), policy: "pol-1", plan: buildPlan(t, jobsOf("sleep 60"))}
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(3 * time.Second); cancel() }()
	c := &cirun.Coordinator{Opts: cirun.Options{Root: dir, Home: r.home, NoStatus: true}, Plan: r.plan, ConfigDigest: "cfg",
		Hooks: cirun.Hooks{Policy: fixedPolicy(r.policy)}, Out: io.Discard, Capacity: roomy, Poll: 10 * time.Millisecond}
	res, err := c.Run(ctx)
	ok(t, err)
	if res.Summary.Doc.Result != "cancelled" {
		t.Fatalf("setup: the run ended %q, want cancelled", res.Summary.Doc.Result)
	}
	st, err := cirun.OpenStore(filepath.Join(r.home, "state.db"))
	ok(t, err)
	t.Cleanup(func() { _ = st.Close() })
	r.st, r.plan = st, buildPlan(t, jobsOf("true"))
	v, ran, err := VerifyRevision(context.Background(), r.env(), sha)
	ok(t, err)
	if !ran || v.Exit != ExitOK || r.runs != 1 {
		t.Fatalf("the cancelled run was returned: ran=%v runs=%d %+v", ran, r.runs, v)
	}
}
