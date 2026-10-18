package run

// Purpose: open the shared run store without losing the first-run race. The
// WAL switch of a brand-new state.db needs an exclusive lock that SQLite's
// busy timeout does not wait for, so two first-ever runs started together
// (two shells, or `ci runs` beside a run) can get SQLITE_BUSY from Open.
// Inputs: a state.db path ("" = the default). Outputs: the open store.
// Constraints: only a busy error is retried (about 10 s); anything else
// returns at once.

import (
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// OpenStore is store.Open with a bounded retry on SQLITE_BUSY.
func OpenStore(path string) (*store.Store, error) {
	var st *store.Store
	var err error
	for end := time.Now().Add(10 * time.Second); ; time.Sleep(25 * time.Millisecond) {
		st, err = store.Open(path, store.Options{})
		if err == nil || !isBusy(err) || time.Now().After(end) {
			return st, err
		}
	}
}

func isBusy(err error) bool {
	return strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked")
}
