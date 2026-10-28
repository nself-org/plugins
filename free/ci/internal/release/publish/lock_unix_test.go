//go:build unix

package publish

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPublicationSweepsOnlyStaleIdleLocks(t *testing.T) {
	home := t.TempDir()
	unlock, err := lockRun(context.Background(), home, "active")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	old := time.Now().Add(-2 * staleAfter)
	for _, name := range []string{"active", "stale", "fresh"} {
		path := filepath.Join(releasesDir(home), ".lock-"+name)
		if name != "active" {
			if err := os.WriteFile(path, nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if name != "fresh" {
			if err := os.Chtimes(path, old, old); err != nil {
				t.Fatal(err)
			}
		}
	}
	next, err := lockRun(context.Background(), home, "next")
	if err != nil {
		t.Fatal(err)
	}
	defer next()
	if _, err := os.Lstat(filepath.Join(releasesDir(home), ".lock-stale")); !os.IsNotExist(err) {
		t.Fatalf("stale lock survived: %v", err)
	}
	for _, name := range []string{"active", "fresh"} {
		if _, err := os.Lstat(filepath.Join(releasesDir(home), ".lock-"+name)); err != nil {
			t.Fatalf("%s removed: %v", name, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), lockPoll)
	defer cancel()
	if release, err := lockRun(ctx, home, "active"); err == nil {
		release()
		t.Fatal("active lock lost exclusion")
	}
}
