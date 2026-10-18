package services

// Purpose: a Sweep ends the CLI calls a dead coordinator left running, and
// leaves the calls of a live owner alone (P7-CI-42 review: an orphaned
// `nself build` would otherwise create resources after the project was torn down).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func startSleeper(t *testing.T, f *Funnel, project string, secs string) (<-chan Result, string) {
	t.Helper()
	dir := filepath.Join(f.Home, "work", project, "svc")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	done := make(chan Result, 1)
	go func() {
		res, _ := f.Run(context.Background(), Call{Dir: dir, Argv: []string{"sleep", secs}})
		done <- res
	}()
	markers := filepath.Join(f.Home, funnelDir, "processes")
	deadline := time.Now().Add(20 * time.Second)
	for {
		if m, _ := filepath.Glob(filepath.Join(markers, "svccall-*.json")); len(m) > 0 {
			return done, dir
		}
		if time.Now().After(deadline) {
			t.Fatal("the call never started")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKillStaleEndsOnlyCallsOfDeadOwners(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("service dependencies need a POSIX host")
	}
	f := &Funnel{Home: t.TempDir()}
	alive, _ := startSleeper(t, f, "nselfci-aaaa1111", "60")
	f.KillStale(context.Background(), func(owner string) bool { return owner == "nselfci-bbbb2222" })
	select {
	case <-alive:
		t.Fatal("the call of a live owner was killed")
	case <-time.After(300 * time.Millisecond):
	}
	f.KillStale(context.Background(), func(owner string) bool { return owner == "nselfci-aaaa1111" })
	select {
	case res := <-alive:
		if res.ExitCode == 0 {
			t.Fatalf("the call of a dead owner ended normally: %+v", res)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the call of a dead owner survived KillStale")
	}
}

func TestOwnerOfNamesTheProvision(t *testing.T) {
	for dir, want := range map[string]string{
		"/h/work/nselfci-ab12cd34/svc": "nselfci-ab12cd34",
		"/h/work/nselfci-ab12cd34":     "nselfci-ab12cd34",
		"":                             "none",
		"/tmp/other":                   "none",
	} {
		if got := ownerOf(dir); got != want {
			t.Errorf("ownerOf(%q) = %q, want %q", dir, got, want)
		}
	}
}
