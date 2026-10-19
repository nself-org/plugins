package checkout

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

func TestHoldMarksADirectoryInUseUntilReleased(t *testing.T) {
	dir := t.TempDir()
	if InUse(dir) {
		t.Fatal("a directory nobody holds reports in use")
	}
	release, err := Hold(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !InUse(dir) {
		t.Fatal("a held directory is not reported in use")
	}
	if _, err := Hold(dir); err == nil {
		t.Fatal("a second Hold of the same directory succeeded")
	}
	release()
	if InUse(dir) {
		t.Fatal("a released directory still reports in use")
	}
}

func TestOpenTransfersAreBoundedAndExpire(t *testing.T) {
	_, rev := makeRepo(t)
	old := Deadline
	Deadline = 150 * time.Millisecond
	defer func() { Deadline = old }()
	base := filepath.Join(t.TempDir(), "stage")
	r := NewReceiver(base, nil)
	good := "sha256:" + strings.Repeat("0", 64)
	for i := range MaxOpen {
		l := leaseFor(rev, good, 10)
		l.LeaseID = "lease-" + string(rune('a'+i))
		if err := r.Expect(l); err != nil {
			t.Fatalf("transfer %d: %v", i, err)
		}
	}
	extra := leaseFor(rev, good, 10)
	extra.LeaseID = "one-too-many"
	if err := r.Expect(extra); !errors.Is(err, ErrBusy) {
		t.Fatalf("Expect beyond MaxOpen = %v, want ErrBusy", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, _ := os.ReadDir(base)
		r.mu.Lock()
		open := len(r.open)
		r.mu.Unlock()
		if len(entries) == 0 && open == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("unclaimed transfers were not dropped: %d open, %d staging roots", open, len(entries))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestAnUnfinishedTransferWakesStageAtTheDeadline(t *testing.T) {
	_, rev := makeRepo(t)
	old := Deadline
	Deadline = 150 * time.Millisecond
	defer func() { Deadline = old }()
	r := NewReceiver(filepath.Join(t.TempDir(), "stage"), nil)
	l := leaseFor(rev, "sha256:"+strings.Repeat("0", 64), 10)
	if err := r.Expect(l); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Stage(context.Background(), l, t.TempDir()); !errors.Is(err, ErrDeadline) {
		t.Fatalf("Stage on a transfer that never finished = %v, want ErrDeadline", err)
	}
}

func TestGitRunsWithAMinimalEnvironment(t *testing.T) {
	t.Setenv("OPERATOR_SECRET", "must-not-reach-git")
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "filter.x.smudge")
	t.Setenv("GIT_CONFIG_VALUE_0", "touch /tmp/should-not-exist")
	env, err := gitEnv{home: "/h"}.Environment(context.Background(), model.Job{})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, bad := range []string{"OPERATOR_SECRET", "GIT_CONFIG_COUNT", "GIT_CONFIG_KEY_0", "filter.x"} {
		if strings.Contains(joined, bad) {
			t.Errorf("git environment holds %s", bad)
		}
	}
	for _, want := range []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "HOME=/h", "GIT_TERMINAL_PROMPT=0"} {
		if !strings.Contains(joined, want) {
			t.Errorf("git environment lacks %s", want)
		}
	}
}
