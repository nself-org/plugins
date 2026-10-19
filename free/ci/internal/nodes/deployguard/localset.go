package deployguard

// Purpose: the exclusion set a host can build for itself: the entries cached in its own state.db and the
// live never-list. The agent has no coordinator to ask, so this is what its start guard compares against.
// Inputs:  the CI home.
// Outputs: Set, or an error when an existing source cannot be read.
// Constraints: a missing state.db or config is an empty source and nothing is created; a busy store is
//          retried briefly; any other failure on an existing file refuses the start.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// LocalSet builds the set from the state.db under home and the never-list. A file that does not exist
// is an empty source; a file that exists and cannot be read, parsed or opened is an error (E663,
// identity_unreadable): the guard must not start on less than the operator wrote down.
func LocalSet(ctx context.Context, home string) (Set, error) {
	var s Set
	cfg := ConfigPath(home)
	es, err := LoadNeverList(cfg)
	if err != nil {
		return s, newErr(ReasonNoIdentity, "the never-list %s cannot be read (%v); fix or remove it to start", cfg, err)
	}
	for _, e := range es {
		s.Add(e)
	}
	db := filepath.Join(filepath.Dir(cfg), "state.db")
	if _, err = os.Stat(db); os.IsNotExist(err) {
		return s, nil
	} else if err != nil {
		return s, newErr(ReasonNoIdentity, "the state database %s cannot be read: %v", db, err)
	}
	st, err := OpenStore(db)
	if err != nil {
		return s, newErr(ReasonNoIdentity, "the state database %s cannot be opened: %v", db, err)
	}
	defer func() { _ = st.Close() }()
	rows, err := st.ExclusionEntries(ctx)
	if err != nil {
		return s, newErr(ReasonNoIdentity, "the cached exclusion entries in %s cannot be read: %v", db, err)
	}
	for _, r := range rows {
		s.Add(Entry{Kind: r.Kind, Value: r.Value, Source: r.Source})
	}
	return s, nil
}

// Retry window and step for a busy state database, as plain durations (2 s and 50 ms).
const (
	openWait = time.Duration(2e9)
	openStep = time.Duration(5e7)
)

// OpenStore opens the state database at path, retrying briefly while another process holds the writer.
func OpenStore(path string) (*store.Store, error) {
	return openRetry(func(p string) (*store.Store, error) { return store.Open(p, store.Options{}) }, path, openWait, openStep)
}

func isBusy(err error) bool {
	return strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked")
}

// openRetry calls open until it succeeds, fails with something other than a busy database, or wait ends.
func openRetry(open func(string) (*store.Store, error), path string, wait, step time.Duration) (*store.Store, error) {
	end := time.Now().Add(wait)
	for {
		st, err := open(path)
		if err == nil || !isBusy(err) || time.Now().After(end) {
			return st, err
		}
		time.Sleep(step)
	}
}
