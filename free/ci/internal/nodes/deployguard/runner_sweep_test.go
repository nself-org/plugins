package deployguard

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A SIGKILL leaves the staged .env copies of a FunnelRun behind; the next FunnelRun removes the stale
// ones and leaves a recent dg-* directory (another run may be using it) and unrelated entries alone.
func TestFunnelRunSweepsStaleStagedDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NSELF_CI_HOME", home)
	work := filepath.Join(home, "work")
	mk := func(name string, age time.Duration) string {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, ".env"), []byte("SECRET=1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-age)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stale, fresh, other := mk("dg-1-1", time.Hour), mk("dg-2-2", time.Second), mk("job-1", time.Hour)
	if _, _, err := FunnelRun(context.Background(), "", "true"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("stale staged dir survived: %v", err)
	}
	for _, p := range []string{fresh, other} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s was removed: %v", p, err)
		}
	}
}
