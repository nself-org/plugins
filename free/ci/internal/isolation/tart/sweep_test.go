package tart

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestHelperCoordinator is not a test: it is the "coordinator" process the kill -9 test starts and kills.
func TestHelperCoordinator(t *testing.T) {
	if os.Getenv("TART_HELPER") != "1" {
		t.Skip("helper process")
	}
	r := &rig{t: t, dir: os.Getenv("FAKE_TART_DIR"), home: os.Getenv("TART_HELPER_HOME")}
	r.p = New("vm", Options{
		Tart: os.Getenv("TART_HELPER_TART"), SSH: os.Getenv("TART_HELPER_SSH"),
		BaseImage: func() string { return testImage }, Home: func() string { return r.home },
		GOOS: "darwin", GOARCH: "arm64", Log: func(string, ...any) {},
	})
	s := r.spec("k1", "sleep", "8")
	r.run(context.Background(), s)
}

func TestKillNineCoordinatorThenNextStartSweepsTheClone(t *testing.T) {
	r := newRig(t, nil)
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperCoordinator$")
	cmd.Env = append(os.Environ(), "TART_HELPER=1", "TART_HELPER_HOME="+r.home, "TART_HELPER_TART="+r.p.opts.Tart, "TART_HELPER_SSH="+r.p.opts.SSH)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(r.dir, "vms", "nself-ci-k1", "home", "work", "k1")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(work); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("the job never started in the clone")
		}
		time.Sleep(100 * time.Millisecond)
	}
	// while its owner lives the clone is not an orphan
	if gone := r.p.Sweep(context.Background(), r.p.opts.NewTool("next", r.home), stateDir(r.home)); len(gone) != 0 {
		t.Fatalf("a live attempt was swept: %v", gone)
	}
	if err := cmd.Process.Kill(); err != nil { // SIGKILL: no deferred cleanup runs
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if _, err := os.Stat(filepath.Join(r.dir, "vms", "nself-ci-k1")); err != nil {
		t.Fatal("expected the orphan clone to exist after kill -9")
	}
	// the next start: Prepare sweeps
	if err := r.p.Prepare(context.Background(), isolationAttemptFor("next-1", "coord-next")); err != nil {
		t.Fatal(err)
	}
	r.noClones()
	waitNoVMProcess(t, r)
}

func waitNoVMProcess(t *testing.T, r *rig) {
	t.Helper()
	// the fake VM process removes its pid file on TERM; there is no clone dir, so nothing may be running
	if _, err := os.Stat(filepath.Join(r.dir, "vms", "nself-ci-k1", "pid")); err == nil {
		t.Fatal("VM still running")
	}
}

func TestSweepSkipsUnlabelledAndLiveClones(t *testing.T) {
	r := newRig(t, nil)
	tool := r.p.opts.NewTool("c", r.home)
	for _, n := range []string{"nself-ci-hand-made", "nself-ci-live"} {
		if _, err := tool.Run(context.Background(), []string{r.p.opts.Tart, "clone", testImage, n}, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	live, err := claimMarker(stateDir(r.home), "nself-ci-live", marker{Name: "nself-ci-live"})
	if err != nil {
		t.Fatal(err)
	}
	defer live.release()
	if gone := r.p.Sweep(context.Background(), tool, stateDir(r.home)); len(gone) != 0 {
		t.Fatalf("removed %v", gone)
	}
	for _, n := range []string{"nself-ci-hand-made", "nself-ci-live"} {
		if _, err := os.Stat(filepath.Join(r.dir, "vms", n)); err != nil {
			t.Fatalf("%s must survive: %v", n, err)
		}
	}
}

func TestMarkerOfAMissingCloneIsDropped(t *testing.T) {
	r := newRig(t, nil)
	c, err := claimMarker(stateDir(r.home), "nself-ci-gone", marker{Name: "nself-ci-gone"})
	if err != nil {
		t.Fatal(err)
	}
	c.unlockKeep()
	if gone := r.p.Sweep(context.Background(), r.p.opts.NewTool("c", r.home), stateDir(r.home)); len(gone) != 1 {
		t.Fatalf("removed %v", gone)
	}
	r.noClones()
}

func TestSecondClaimOfALiveAttemptFails(t *testing.T) {
	dir := t.TempDir()
	c, err := claimMarker(dir, "nself-ci-x", marker{})
	if err != nil {
		t.Fatal(err)
	}
	defer c.release()
	if _, err := claimMarker(dir, "nself-ci-x", marker{}); err == nil {
		t.Fatal("a live marker must not be claimed twice")
	}
}
