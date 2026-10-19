package evidence

// D-1008: the status and the hidden-edit listing of ReadWorktree are independent
// funnel spawns; read one after the other they cost both, side by side the slower.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadWorktreeAsksStatusAndListingTogether(t *testing.T) {
	listing := make(chan struct{})
	var once sync.Once
	overlapped := false
	capture := func(_ context.Context, argv ...string) (string, int, error) {
		switch j := strings.Join(argv, " "); {
		case strings.Contains(j, " status "):
			select {
			case <-listing: // the listing started while the status was still running
				overlapped = true
			case <-time.After(2 * time.Second):
			}
		case strings.Contains(j, " ls-files "):
			once.Do(func() { close(listing) })
		}
		return "", 0, nil
	}
	if w := ReadWorktree(context.Background(), capture, t.TempDir()); w.Dirty {
		t.Fatalf("an empty status and listing read as dirty: %+v", w)
	}
	if !overlapped {
		t.Fatal("the hidden-edit listing waited for the status to finish")
	}
}
