package selftest

// Purpose: list the job-root folders no live attempt owns (the TRUST-09 orphan list), for
// the leak check and for the tidy plan, with the same filter hygiene.SweepOrphans applies.
// Inputs:  the job-root base (<home>/work), the minimum age, the time. Outputs: folder names.
// Constraints: read-only; a base that is a symlink or not a directory is an error, as for
// the sweep; only names shaped like an attempt (`<job row>.a<N>`) older than minAge count.

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/hygiene"
)

// OrphanMinAge protects a root a sibling process may still be creating.
const OrphanMinAge = time.Hour

var attemptShape = regexp.MustCompile(`^.+\.a[0-9]+$`)

// OrphanRoots returns the sorted names under base that a sweep would remove.
func OrphanRoots(base string, minAge time.Duration, now time.Time) ([]string, error) {
	if base == "" {
		return nil, nil
	}
	info, err := os.Lstat(base)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("job root base %s is not a plain directory", base)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !hygiene.ValidAttempt(name) || !attemptShape.MatchString(name) {
			continue
		}
		if fi, err := e.Info(); err == nil && now.Sub(fi.ModTime()) >= minAge {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}
