package main

// Tests of `nself ci runs wait` (P7-CI-63): terminal and running exits, the
// timeout and invoke-deadline caps, a run that finishes while waiting, a run
// whose coordinator died (lost), unknown runs and remote resolvers. The shared
// store fixtures (seedRun, finishRun) are used by runs_follow_test.go too.

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// noContainerRuntime puts a docker and a podman that refuse every command first on PATH. Reading a run
// recovers after dead coordinators, which asks each installed container runtime for orphans (`info`, `ps`);
// a host with a live daemon (a CI runner) spends about a second on that, and these tests are about the
// run's stream, not the host's runtimes.
func noContainerRuntime(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}
	dir := t.TempDir()
	for _, name := range []string{"docker", "podman"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runsHome points NSELF_CI_HOME at a fresh directory and speeds the poll up.
func runsHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	noContainerRuntime(t)
	t.Setenv("NSELF_CI_HOME", home)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("NSELF_INVOKE_DEADLINE_MS", "")
	t.Setenv("NSELF_CI_COORDINATOR", "")
	oldPoll, oldIdle := waitPoll, followIdle
	waitPoll, followIdle = 10*time.Millisecond, 30*time.Millisecond
	t.Cleanup(func() { waitPoll, followIdle = oldPoll, oldIdle })
	return home
}

// seedRun records a pipeline with one row per job name (job states by name:state).
func seedRun(t *testing.T, home, id, status string, created time.Time, jobs map[string]string) {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{Clock: func() time.Time { return created }})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	p := store.PipelineRow{ID: id, Revision: "0123456789abcdef0123456789abcdef01234567", Project: "proj", Trigger: "local", SourceTrust: "owner", PrivacyZone: "local-only",
		PolicyDigest: "p", InputDigest: "i", SelectionMode: "full", Status: status}
	if err = st.CreatePipeline(ctx, p); err != nil {
		t.Fatal(err)
	}
	for name, state := range jobs {
		jid := id + "-" + name
		if err = st.CreateJob(ctx, store.JobRow{ID: jid, PipelineID: id, Name: name, Kind: "test", Idempotent: true, InfraMax: 1, InputDigest: "i"}); err != nil {
			t.Fatal(err)
		}
		if err = st.SetJobStatus(ctx, jid, state); err != nil {
			t.Fatal(err)
		}
	}
}

// finishRun ends a seeded run, as the coordinator does.
func finishRun(t *testing.T, home, id, status string) {
	t.Helper()
	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Error(err) // callers may run in a goroutine: no FailNow
		return
	}
	defer func() { _ = st.Close() }()
	if err = st.FinishPipeline(context.Background(), id, status); err != nil {
		t.Error(err)
	}
}

type waitOut struct {
	Data struct {
		RunID    string `json:"run_id"`
		State    string `json:"state"`
		Terminal bool   `json:"terminal"`
		WaitedMS int64  `json:"waited_ms"`
		Jobs     []struct{ Name, State string }
	} `json:"data"`
}

func waitJSON(t *testing.T, args ...string) (int, waitOut, time.Duration) {
	t.Helper()
	start := time.Now()
	code, out, errs := capture(t, func() int { return runWait(args) })
	took := time.Since(start)
	var w waitOut
	if out != "" {
		if err := json.Unmarshal([]byte(out), &w); err != nil {
			t.Fatalf("not an envelope (exit %d): %v\n%s\n%s", code, err, out, errs)
		}
	}
	return code, w, took
}

func TestWaitBudget(t *testing.T) {
	for _, c := range []struct {
		timeout  time.Duration
		deadline string
		want     time.Duration
	}{
		{240 * time.Second, "", 240 * time.Second},
		{240 * time.Second, "15000", 5 * time.Second},
		{2 * time.Second, "15000", 2 * time.Second},
		{240 * time.Second, "9000", 0},
		{240 * time.Second, "300000", 240 * time.Second},
		{240 * time.Second, "garbage", 240 * time.Second},
		{-time.Second, "", 0},
	} {
		if got := waitBudget(c.timeout, c.deadline); got != c.want {
			t.Errorf("waitBudget(%v, %q) = %v, want %v", c.timeout, c.deadline, got, c.want)
		}
	}
}

func TestRunsWait(t *testing.T) {
	t.Run("terminal run returns exit 0 with the state", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-pass", "passed", time.Now(), map[string]string{"build": "passed"})
		code, w, took := waitJSON(t, "r-pass", "--json")
		if code != 0 || w.Data.State != "passed" || !w.Data.Terminal || len(w.Data.Jobs) != 1 || took > 3*time.Second {
			t.Fatalf("exit %d %+v in %v", code, w, took)
		}
		code, w, _ = waitJSON(t, "latest", "--json")
		if code != 0 || w.Data.RunID != "r-pass" {
			t.Fatalf("latest: exit %d %+v", code, w)
		}
	})
	t.Run("a failed run is still exit 0 (the state carries the result)", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-fail", "failed", time.Now(), map[string]string{"test": "failed"})
		if code, w, _ := waitJSON(t, "r-fail", "--json"); code != 0 || w.Data.State != "failed" {
			t.Fatalf("exit %d %+v", code, w)
		}
	})
	t.Run("a running run times out with exit 10", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-run", "running", time.Now(), map[string]string{"build": "running"})
		code, w, took := waitJSON(t, "r-run", "--timeout", "300ms", "--json")
		// The budget covers opening the store and the first read, so on a loaded host it can end before
		// that read: the state is then "unknown" (what the exit and the caps below assert is the budget,
		// not the host's speed). The state of a run that was read is pinned by the capped-wait test.
		if code != 10 || (w.Data.State != "running" && w.Data.State != "unknown") || w.Data.Terminal || took < 300*time.Millisecond || took > 3*time.Second {
			t.Fatalf("exit %d %+v in %v", code, w, took)
		}
	})
	t.Run("returns shortly after the run finishes", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-live", "running", time.Now(), map[string]string{"build": "running"})
		finished := make(chan time.Time, 1)
		go func() {
			time.Sleep(400 * time.Millisecond)
			finishRun(t, home, "r-live", "passed")
			finished <- time.Now()
		}()
		code, w, _ := waitJSON(t, "r-live", "--timeout", "20s", "--json")
		returned := time.Now()
		if code != 0 || w.Data.State != "passed" {
			t.Fatalf("exit %d %+v", code, w)
		}
		if lag := returned.Sub(<-finished); lag > time.Second {
			t.Fatalf("returned %v after the run finished", lag)
		}
	})
	t.Run("the invoke deadline caps the wait", func(t *testing.T) {
		runsHome(t)
		// A resolver that answers at once: the 200 ms budget must not also have to pay for opening the
		// store and recovering dead coordinators, which on a loaded host takes longer than the budget.
		old := cirun.Registered.Remote
		cirun.Registered.Remote = []cirun.RemoteResolver{steadyRemote{ref: "r-cap"}}
		t.Cleanup(func() { cirun.Registered.Remote = old })
		t.Setenv("NSELF_INVOKE_DEADLINE_MS", "10200") // 10.2 s minus the 10 s margin = 200 ms
		code, w, took := waitJSON(t, "r-cap", "--json")
		if code != 10 || w.Data.State != "running" || took > 3*time.Second {
			t.Fatalf("exit %d %+v in %v", code, w, took)
		}
	})
	t.Run("a run whose coordinator died is lost, terminal, exit 0", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-dead", "running", time.Now().Add(-10*time.Minute), nil)
		code, w, _ := waitJSON(t, "r-dead", "--timeout", "10s", "--json")
		if code != 0 || w.Data.State != "lost" || !w.Data.Terminal {
			t.Fatalf("exit %d %+v", code, w)
		}
	})
	t.Run("unknown run and bad usage", func(t *testing.T) {
		runsHome(t)
		if code, _, errs := capture(t, func() int { return runWait([]string{"nope", "--json"}) }); code != 1 {
			t.Fatalf("unknown run: exit %d %s", code, errs)
		}
		if code, _, _ := capture(t, func() int { return runWait(nil) }); code != 1 {
			t.Fatalf("no run: exit %d", code)
		}
		if code, _, _ := capture(t, func() int { return runWait([]string{"a", "b"}) }); code != 1 {
			t.Fatalf("two runs: exit %d", code)
		}
	})
	t.Run("a remote resolver answers first and --coordinator reaches it", func(t *testing.T) {
		runsHome(t)
		fake := &fakeRemote{views: []cirun.RunView{{ID: "remote-1", Status: "running"}, {ID: "remote-1", Status: "passed", Jobs: []cirun.JobView{{Name: "build", State: "passed"}}}}}
		old := cirun.Registered.Remote
		cirun.Registered.Remote = []cirun.RemoteResolver{fake} // instead of the real client (P7-NODE-19), which pins https and a root
		t.Cleanup(func() { cirun.Registered.Remote = old })
		code, w, _ := waitJSON(t, "remote-1", "--coordinator", "http://127.0.0.1:9", "--timeout", "10s", "--json")
		if code != 0 || w.Data.State != "passed" || len(w.Data.Jobs) != 1 {
			t.Fatalf("exit %d %+v", code, w)
		}
		if fake.target != "http://127.0.0.1:9" {
			t.Fatalf("the resolver saw coordinator %q", fake.target)
		}
	})
	t.Run("a stalled resolver cannot outlive the budget", func(t *testing.T) {
		runsHome(t)
		old := cirun.Registered.Remote
		cirun.Registered.Remote = append(append([]cirun.RemoteResolver(nil), old...), &stalledRemote{})
		t.Cleanup(func() { cirun.Registered.Remote = old })
		code, w, took := waitJSON(t, "remote-stall", "--timeout", "200ms", "--json")
		if code != 10 || w.Data.Terminal || took > 700*time.Millisecond {
			t.Fatalf("exit %d %+v in %v", code, w, took)
		}
	})
	t.Run("human output names the run and its jobs", func(t *testing.T) {
		home := runsHome(t)
		seedRun(t, home, "r-human", "passed", time.Now(), map[string]string{"build": "passed"})
		code, out, _ := capture(t, func() int { return runWait([]string{"r-human"}) })
		if code != 0 || out != "run r-human: passed\n  passed         build \n" {
			t.Fatalf("exit %d %q", code, out)
		}
		if _, err := os.Stat(filepath.Join(home, "state.db")); err != nil {
			t.Fatal(err)
		}
	})
}

// fakeRemote answers like a coordinator client: the views in turn, the last one repeated.
type fakeRemote struct {
	views  []cirun.RunView
	calls  int
	target string
}

func (f *fakeRemote) Resolve(_ context.Context, ref string) (cirun.RunView, bool, error) {
	f.target = os.Getenv("NSELF_CI_COORDINATOR")
	if ref != "remote-1" {
		return cirun.RunView{}, false, nil
	}
	i := min(f.calls, len(f.views)-1)
	f.calls++
	return f.views[i], true, nil
}

// steadyRemote reports its run as running, at once, on every look.
type steadyRemote struct{ ref string }

func (r steadyRemote) Resolve(_ context.Context, ref string) (cirun.RunView, bool, error) {
	if ref != r.ref {
		return cirun.RunView{}, false, nil
	}
	return cirun.RunView{ID: ref, Status: "running"}, true, nil
}

// stalledRemote answers the first look at once, then blocks until its context ends. The look count
// belongs to the value, so every test (and every -count repeat) starts with a first look that answers.
type stalledRemote struct{ looks int }

func (r *stalledRemote) Resolve(ctx context.Context, ref string) (cirun.RunView, bool, error) {
	if ref != "remote-stall" {
		return cirun.RunView{}, false, nil
	}
	r.looks++
	if r.looks == 1 {
		return cirun.RunView{ID: ref, Status: "running"}, true, nil
	}
	<-ctx.Done()
	return cirun.RunView{}, false, ctx.Err()
}

func TestOpenStoreCtxGivesUpWhenTheContextEnds(t *testing.T) {
	home := runsHome(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	st, err := openStoreCtx(ctx, filepath.Join(home, "state.db"))
	if err == nil || st != nil || time.Since(start) > time.Second {
		t.Fatalf("store %v err %v in %v", st, err, time.Since(start))
	}
	ok, err := openStoreCtx(context.Background(), filepath.Join(home, "state.db"))
	if err != nil || ok == nil {
		t.Fatalf("open: %v", err)
	}
	_ = ok.Close()
}
