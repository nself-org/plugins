package deployguard

// Purpose: remove the staged project files a killed FunnelRun left behind.
// Inputs:  the CI home as the executor resolves it ($NSELF_CI_HOME, else ~/.nself/ci).
// Outputs: none; a failed removal is ignored (the next call tries again).
// Constraints: only `dg-*` directories directly under <home>/work, older than staleAfter, are removed;
//          a younger one may belong to a FunnelRun in another process.

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// staleAfter is well past runTimeout, so no live run's directory is old enough to be taken.
const staleAfter = 10 * time.Minute

func sweepStaged() {
	home := os.Getenv("NSELF_CI_HOME")
	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return
		}
		home = filepath.Join(h, ".nself", "ci")
	}
	work := filepath.Join(home, "work")
	ents, err := os.ReadDir(work)
	if err != nil {
		return
	}
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "dg-") {
			continue
		}
		if fi, err := e.Info(); err == nil && time.Since(fi.ModTime()) > staleAfter {
			_ = os.RemoveAll(filepath.Join(work, e.Name()))
		}
	}
}
