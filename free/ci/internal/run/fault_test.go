//go:build fault

package run

// Fault tests: a coordinator killed with SIGKILL mid-run, and two runs that
// share one host reservation.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	ciexec "github.com/nself-org/plugins/free/ci/internal/exec"
	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// TestFaultChild is the coordinator process TestCoordinatorKill kills.
func TestFaultChild(t *testing.T) {
	home := os.Getenv("CI37_HOME")
	if home == "" {
		return // not the child
	}
	dir := os.Getenv("CI37_DIR")
	pidJob := func(name string, idem *bool) model.Job {
		j := sh("echo $$ > " + dir + "/" + name + ".pid; exec sleep 120")
		j.Idempotent = idem
		return j
	}
	c := &Coordinator{Opts: Options{Root: os.Getenv("CI37_ROOT"), Home: home, NoStatus: true}, ConfigDigest: "cfg",
		Plan: buildPlan(t, map[string]model.Job{"pure": pidJob("pure", nil), "once": pidJob("once", no())}), Capacity: roomy, Out: os.Stdout}
	_, _ = c.Run(context.Background())
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func readPID(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(b)))
	return n
}

func TestCoordinatorKill(t *testing.T) {
	home, dir, root := t.TempDir(), t.TempDir(), t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestFaultChild$", "-test.v")
	cmd.Env = append(os.Environ(), "CI37_HOME="+home, "CI37_DIR="+dir, "CI37_ROOT="+root, "NSELF_CI_HOME="+home)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var pure, once int
	for end := time.Now().Add(60 * time.Second); time.Now().Before(end) && (pure == 0 || once == 0); time.Sleep(100 * time.Millisecond) {
		pure, once = readPID(t, filepath.Join(dir, "pure.pid")), readPID(t, filepath.Join(dir, "once.pid"))
	}
	if pure == 0 || once == 0 {
		_ = cmd.Process.Kill()
		t.Fatal("jobs never started")
	}
	time.Sleep(300 * time.Millisecond) // let both reach running in the store
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	t.Cleanup(func() { _ = syscall.Kill(pure, syscall.SIGKILL); _ = syscall.Kill(once, syscall.SIGKILL) })

	st, err := store.Open(filepath.Join(home, "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	ctx := context.Background()
	t.Setenv("NSELF_CI_HOME", home)
	rep := Recover(ctx, st, "", home)
	if len(rep.Errors) != 0 || rep.Lost != 2 {
		t.Fatalf("recover: %+v", rep)
	}
	page, _ := st.History(ctx, 0, 5)
	if len(page.Pipelines) != 1 {
		t.Fatalf("runs: %+v", page.Pipelines)
	}
	got := map[string]string{}
	at, _ := st.PipelineAttempts(ctx, page.Pipelines[0].ID)
	for _, a := range at {
		name := a.JobID[strings.LastIndex(a.JobID, ".")+1:]
		got[name[:strings.LastIndex(name, "-")]] = a.State
	}
	if got["pure"] != "retryable" || got["once"] != "needs-operator" {
		t.Fatalf("attempt states after recovery: %v", got)
	}
	if row, ok, _ := st.PipelineByID(ctx, page.Pipelines[0].ID); !ok || row.Status != "lost" {
		t.Fatalf("a run whose coordinator died must not stay running: %+v", row)
	}
	for end := time.Now().Add(20 * time.Second); time.Now().Before(end) && (alive(pure) || alive(once)); time.Sleep(100 * time.Millisecond) {
	}
	if alive(pure) || alive(once) {
		t.Fatalf("orphans survived the next start: pure alive=%v once alive=%v", alive(pure), alive(once))
	}
}

func TestConcurrentRuns(t *testing.T) {
	shared, home := t.TempDir(), t.TempDir()
	probe := "d=" + shared + "; mkdir $d/in.$$; ls $d | grep -c '^in\\.' >> $d/count.log; sleep 0.4; rmdir $d/in.$$"
	jobs := func() map[string]model.Job {
		return map[string]model.Job{"a": sh(probe), "b": sh(probe), "c": sh(probe), "d": sh(probe)}
	}
	capacity := func() ciexec.Capacity {
		return ciexec.Capacity{CPUs: 3, MemoryMB: 16384, FreeMemoryMB: 12000, BatteryPercent: -1, PluggedIn: true, Interactive: true}
	}
	rigs := []*rig{newRig(t, jobs(), gitFake{}), newRig(t, jobs(), gitFake{})}
	t.Setenv("NSELF_CI_HOME", home)
	var wg sync.WaitGroup
	results := make([]Result, 2)
	for i, r := range rigs {
		i, r := i, r
		r.Home = home
		r.C.Opts.Home, r.C.Capacity, r.C.Opts.NoStatus = home, capacity, true
		wg.Add(1)
		go func() {
			defer wg.Done()
			var rerr error
			results[i], rerr = r.C.Run(context.Background())
			if rerr != nil {
				t.Errorf("run %d: %v", i, rerr)
			}
		}()
	}
	wg.Wait()
	slots := ciexec.Admit(model.Job{}, ciexec.Foreground, capacity(), ciexec.AdmissionPolicy{}).Slots
	b, _ := os.ReadFile(filepath.Join(shared, "count.log"))
	max := 0
	for _, f := range strings.Fields(string(b)) {
		n, _ := strconv.Atoi(f)
		max = maxInt(max, n)
	}
	if len(strings.Fields(string(b))) != 8 || max > slots {
		t.Fatalf("8 jobs expected, concurrent max %d must not exceed the reservation %d (log %q)\n%+v\n%+v", max, slots, b, results[0].Summary.Jobs, results[1].Summary.Jobs)
	}
	st := rigs[0].store(t)
	for _, res := range results {
		if res.Exit != 0 {
			t.Fatalf("a run failed: %+v", res.Summary.Jobs)
		}
		at, _ := st.PipelineAttempts(context.Background(), res.RunID)
		if len(at) != 4 {
			t.Fatalf("run %s has %d attempts, want its own 4", res.RunID, len(at))
		}
		for _, a := range at {
			if !strings.HasPrefix(a.JobID, res.RunID+".") || a.State != "passed" {
				t.Fatalf("foreign or unfinished attempt in %s: %+v", res.RunID, a)
			}
		}
	}
	if results[0].RunID == results[1].RunID {
		t.Fatal("runs share an id")
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
