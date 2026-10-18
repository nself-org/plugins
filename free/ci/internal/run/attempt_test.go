package run

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestCreateAttemptOnly: no non-test Go file outside internal/store,
// internal/lease, internal/dispatch and internal/run/attempt.go calls
// AdmitAndLease or the store's CreateAttempt.
func TestCreateAttemptOnly(t *testing.T) {
	root := filepath.Join("..", "..")
	allowed := func(path string) bool {
		p := filepath.ToSlash(path)
		for _, d := range []string{"/internal/store/", "/internal/lease/", "/internal/dispatch/"} {
			if strings.Contains(p, d) {
				return true
			}
		}
		return strings.HasSuffix(p, "internal/run/attempt.go")
	}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() && (d.Name() == "vendor" || d.Name() == "testdata") {
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || allowed(path) {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, banned := range []string{"AdmitAndLease(", ".CreateAttempt("} {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, banned) && !strings.HasPrefix(strings.TrimSpace(line), "//") {
					// run.CreateAttempt is the one public function; only the store method is banned.
					if banned == ".CreateAttempt(" && !strings.Contains(line, "Store.CreateAttempt(") && !strings.Contains(line, "st.CreateAttempt(") {
						continue
					}
					t.Errorf("%s calls %s", path, banned)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type spyDispatch struct {
	mu     sync.Mutex
	seen   []string
	refuse string
	passOn bool
}

func (d *spyDispatch) hook(ctx context.Context, s AttemptSpec) (Admitted, error) {
	d.mu.Lock()
	d.seen = append(d.seen, s.Job.Run[len(s.Job.Run)-1])
	refuse := d.refuse != "" && strings.Contains(s.Job.Run[len(s.Job.Run)-1], d.refuse)
	d.mu.Unlock()
	if refuse {
		return Admitted{}, errors.New("E693: no runner admits this job")
	}
	return LocalAdmit(ctx, s)
}

func TestDispatchHook(t *testing.T) {
	t.Cleanup(func() { Dispatch = nil })
	spy := &spyDispatch{refuse: "REFUSED"}
	Dispatch = spy.hook
	r := newRig(t, map[string]model.Job{"a": sh("true"), "b": sh("echo REFUSED")}, gitFake{})
	res := r.run(t)
	if len(spy.seen) != 2 {
		t.Fatalf("dispatch saw %v", spy.seen)
	}
	if o := outcome(res, "b"); o.State != StateFailed || o.Code != "E693" {
		t.Fatalf("refused job: %+v", o)
	}
	if o := outcome(res, "a"); o.State != StatePassed {
		t.Fatalf("a: %+v", o)
	}
	// never partial: the refused attempt was cancelled, not left queued
	st := r.store(t)
	page, _ := st.History(context.Background(), 0, 5)
	at, _ := st.PipelineAttempts(context.Background(), page.Pipelines[0].ID)
	for _, a := range at {
		if a.State == "queued" {
			t.Fatalf("partial attempt left behind: %+v", a)
		}
	}
	Dispatch = nil
	r2 := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	if res := r2.run(t); res.Exit != 0 {
		t.Fatalf("nil Dispatch must admit locally:\n%s", r2.Out)
	}
}

func TestPolicyResolver(t *testing.T) {
	digests := map[string]bool{}
	for i := 0; i < 2; i++ {
		r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
		res := r.run(t)
		row, ok, err := r.store(t).PipelineByID(context.Background(), res.RunID)
		if err != nil || !ok || len(row.PolicyDigest) != 64 {
			t.Fatalf("policy digest on the pipeline row: %+v %v %v", row, ok, err)
		}
		digests[row.PolicyDigest] = true
		if res.Summary.Result != "pass" {
			t.Fatal("run failed")
		}
	}
	if len(digests) != 1 {
		t.Fatalf("default digest is not stable: %v", digests)
	}
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Policy = fakePolicy{"feedface"}
	res := r.run(t)
	row, _, _ := r.store(t).PipelineByID(context.Background(), res.RunID)
	if row.PolicyDigest != "feedface" {
		t.Fatalf("fake resolver digest not stored: %q", row.PolicyDigest)
	}
}

type fakePolicy struct{ digest string }

func (f fakePolicy) Resolve(context.Context, PolicyRequest) (PolicyResult, error) {
	return PolicyResult{Digest: f.digest}, nil
}

func TestTrustFlagNeedsClassifier(t *testing.T) {
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Opts.Trust = "owner"
	if _, err := r.C.Run(context.Background()); CodeOf(err) != CodeTrust {
		t.Fatalf("want E625, got %v", err)
	}
	r.C.Hooks.Trust = fakeTrust("owner")
	if res, err := r.C.Run(context.Background()); err != nil || res.Exit != 0 {
		t.Fatalf("classifier registered: %v %+v", err, res)
	}
	r.C.Hooks.Trust = fakeTrust("untrusted")
	if _, err := r.C.Run(context.Background()); CodeOf(err) != CodeTrust {
		t.Fatalf("mismatched class: %v", err)
	}
}

type fakeTrust string

func (f fakeTrust) Classify(context.Context, TrustRequest) (model.TrustClass, error) {
	return model.TrustClass(f), nil
}

func TestSHAIsE624(t *testing.T) {
	o := Options{SHA: "abc"}
	if err := o.Validate(Hooks{}); CodeOf(err) != CodeSHA || !strings.Contains(err.Error(), "evidence verify abc") {
		t.Fatalf("%v", err)
	}
}

func TestSessionRequiredForCreateAttempt(t *testing.T) {
	if _, err := CreateAttempt(context.Background(), AttemptSpec{}); CodeOf(err) != CodeRun {
		t.Fatalf("%v", err)
	}
	_ = store.Coordinator{}
}
