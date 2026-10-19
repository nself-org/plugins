package evidence

// Purpose: Begin reads the worktree state, the git version, the inputs and the
// repo id side by side (D-1008): serial funnel spawns on the start path cost
// about 11 ms each.

import (
	"context"
	"sync"
	"testing"
	"time"

	cirun "github.com/nself-org/plugins/free/ci/internal/run"
)

func TestBeginReadsGitFactsTogether(t *testing.T) {
	var mu sync.Mutex
	inFlight, peak := 0, 0
	capture := func(_ context.Context, _ ...string) (string, int, error) {
		mu.Lock()
		inFlight++
		peak = max(peak, inFlight)
		mu.Unlock()
		time.Sleep(40 * time.Millisecond)
		mu.Lock()
		inFlight--
		mu.Unlock()
		return "", 0, nil
	}
	Builder{}.Begin(context.Background(), "/repo", cirun.Revision{SHA: "ab" + "cd"}, capture)
	if peak < 3 {
		t.Fatalf("git facts were read nearly one at a time (peak %d in flight)", peak)
	}
}
