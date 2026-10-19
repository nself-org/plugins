package run

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

type wsFn func(AttemptInfo, *os.Root) error

func (f wsFn) AfterJob(_ context.Context, a AttemptInfo, ws *os.Root) error { return f(a, ws) }

type finFn func(AttemptInfo, int64) error

func (f finFn) AttemptFinalize(_ context.Context, a AttemptInfo, e int64) error { return f(a, e) }

func TestWorkspaceHookSeesOutputsOfAPassingJobOnly(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	seen := map[string]string{}
	r := newRig(t, map[string]model.Job{"ok": sh(`echo built > out.txt`), "bad": sh(`echo nope > out.txt; exit 3`)}, gitFake{})
	r.C.Hooks.Workspaces = []WorkspaceHook{wsFn(func(a AttemptInfo, ws *os.Root) error {
		b, err := ws.ReadFile("out.txt")
		mu.Lock()
		defer mu.Unlock()
		seen[a.JobName] = strings.TrimSpace(string(b))
		return err
	})}
	r.run(t)
	if len(seen) != 1 || seen["ok"] != "built" {
		t.Fatalf("workspace hook calls: %v", seen)
	}
}

func TestWorkspaceHookErrorFailsTheAttemptWithItsCode(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Workspaces = []WorkspaceHook{wsFn(func(AttemptInfo, *os.Root) error { return errors.New("E712 missing: nothing collected") })}
	res := r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed || o.Code != "E712" || o.FailureClass != "infra" || res.Exit == 0 {
		t.Fatalf("a: %+v", o)
	}
}

func TestWorkspaceHookPanicIsAnError(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Workspaces = []WorkspaceHook{wsFn(func(AttemptInfo, *os.Root) error { panic("boom") })}
	res := r.run(t)
	if o := outcome(res, "a"); o.State != StateFailed || o.Code != CodeOptions {
		t.Fatalf("a: %+v", o)
	}
}

func TestFinalizeHookRunsWhileFinalizingAtTheCurrentEpoch(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var got []string
	r := newRig(t, map[string]model.Job{"a": sh("true"), "bad": sh("exit 1")}, gitFake{})
	st := r.store(t)
	r.C.Hooks.AttemptFinalizes = []AttemptFinalizeHook{finFn(func(a AttemptInfo, epoch int64) error {
		state, cur, err := st.AttemptStateEpoch(context.Background(), a.AttemptID)
		mu.Lock()
		defer mu.Unlock()
		got = append(got, a.JobName+":"+string(state))
		if err != nil || cur != epoch || a.Store == nil {
			t.Errorf("epoch %d vs %d, store %v, err %v", epoch, cur, a.Store, err)
		}
		return nil
	})}
	r.run(t)
	if len(got) != 1 || got[0] != "a:finalizing" {
		t.Fatalf("finalize calls: %v", got)
	}
}

func TestFinalizeHookRefusalFailsInfraAndStopsLaterHooks(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	called := 0
	r.C.Hooks.AttemptFinalizes = []AttemptFinalizeHook{
		finFn(func(AttemptInfo, int64) error { return errors.New("E712 digest_mismatch: blob does not verify") }),
		finFn(func(AttemptInfo, int64) error { called++; return nil }),
	}
	res := r.run(t)
	o := outcome(res, "a")
	if o.State != StateFailed || o.Code != "E712" || o.FailureClass != "infra" || res.Exit == 0 || called != 0 {
		t.Fatalf("a: %+v called %d", o, called)
	}
	if got := attemptState(t, r, o.AttemptID); got != "failed" {
		t.Fatalf("attempt state %s", got)
	}
}

func TestFinalizeHookPanicIsAnError(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.AttemptFinalizes = []AttemptFinalizeHook{finFn(func(AttemptInfo, int64) error { panic("boom") })}
	if o := outcome(r.run(t), "a"); o.State != StateFailed || o.Code != CodeOptions || o.FailureClass != "infra" {
		t.Fatalf("a: %+v", o)
	}
}

func TestResolvedAddsRegisteredSeams(t *testing.T) {
	saved := Registered
	t.Cleanup(func() { Registered = saved })
	Registered.Workspaces = nil
	Registered.AttemptFinalizes = nil
	RegisterWorkspace(wsFn(func(AttemptInfo, *os.Root) error { return nil }))
	RegisterAttemptFinalize(finFn(func(AttemptInfo, int64) error { return nil }))
	local := Hooks{Workspaces: []WorkspaceHook{wsFn(func(AttemptInfo, *os.Root) error { return nil })}}
	got := local.Resolved()
	if len(got.Workspaces) != 2 || len(got.AttemptFinalizes) != 1 {
		t.Fatalf("workspaces %d, finalizes %d", len(got.Workspaces), len(got.AttemptFinalizes))
	}
}

type wantsFin struct {
	wants bool
	calls *int
}

func (w wantsFin) Wants(AttemptInfo) bool { return w.wants }
func (w wantsFin) AttemptFinalize(context.Context, AttemptInfo, int64) error {
	*w.calls++
	return nil
}

func TestFinalizeHooksThatDoNotWantTheAttemptAreSkipped(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	skipped, wanted := 0, 0
	r.C.Hooks.AttemptFinalizes = []AttemptFinalizeHook{wantsFin{false, &skipped}, wantsFin{true, &wanted}}
	if res := r.run(t); res.Exit != 0 || skipped != 0 || wanted != 1 {
		t.Fatalf("exit %d skipped %d wanted %d", res.Exit, skipped, wanted)
	}
}

type prepFn struct {
	wsFn
	before func(AttemptInfo, *os.Root) error
}

func (p prepFn) BeforeJob(_ context.Context, a AttemptInfo, ws *os.Root) error {
	return p.before(a, ws)
}

func TestWorkspacePrepareHookRunsBeforeTheCommand(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"ok": sh(`test "$(cat prepared.txt)" = ready`)}, gitFake{})
	r.C.Hooks.Workspaces = []WorkspaceHook{prepFn{wsFn: func(AttemptInfo, *os.Root) error { return nil },
		before: func(_ AttemptInfo, ws *os.Root) error { return ws.WriteFile("prepared.txt", []byte("ready"), 0o644) }}}
	if o := outcome(r.run(t), "ok"); o.State != StatePassed {
		t.Fatalf("ok: %+v", o)
	}
}

func TestWorkspacePrepareHookErrorFailsTheAttemptInfra(t *testing.T) {
	t.Parallel()
	r := newRig(t, map[string]model.Job{"a": sh("true")}, gitFake{})
	r.C.Hooks.Workspaces = []WorkspaceHook{prepFn{wsFn: func(AttemptInfo, *os.Root) error { return nil },
		before: func(AttemptInfo, *os.Root) error { return errors.New("prepare failed") }}}
	if o := outcome(r.run(t), "a"); o.State != StateFailed || o.FailureClass != "infra" {
		t.Fatalf("a: %+v", o)
	}
}
