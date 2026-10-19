package run

// Purpose: GitRevision asks for HEAD and for the worktree state at the same time
// (D-1008): two serial funnel spawns on the start path cost about 11 ms each.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestGitRevisionAsksHeadAndStatusTogether(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	sha := strings.Repeat("ab", 20)
	capture := func(_ context.Context, argv ...string) (string, int, error) {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(60 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		if argv[len(argv)-1] == "HEAD" {
			return sha + "\n", 0, nil
		}
		return "", 0, nil
	}
	rev := GitRevision(context.Background(), "/repo", capture)
	if rev.SHA != sha || rev.Dirty {
		t.Fatalf("revision %+v", rev)
	}
	if peak < 2 {
		t.Fatalf("git was asked one question at a time (peak %d in flight)", peak)
	}
}

func TestGitRevisionWithoutACommitIsEmptyAndNeverDirty(t *testing.T) {
	capture := func(_ context.Context, argv ...string) (string, int, error) {
		if argv[len(argv)-1] == "HEAD" {
			return "fatal: ambiguous argument 'HEAD'\n", 128, nil
		}
		return "?? file\n", 0, nil
	}
	if rev := GitRevision(context.Background(), "/repo", capture); rev.SHA != "" || rev.Dirty {
		t.Fatalf("revision %+v", rev)
	}
}
