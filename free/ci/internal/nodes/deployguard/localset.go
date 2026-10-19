package deployguard

// Purpose: the exclusion set a host can build for itself: the entries cached in its own state.db and the
// live never-list. The agent has no coordinator to ask, so this is what its start guard compares against.
// Inputs:  the CI home.
// Outputs: Set and warnings.
// Constraints: a missing state.db is not an error and is never created; a busy store is retried briefly
//          and then skipped with a warning.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// LocalSet builds the set from the state.db under home and the never-list.
func LocalSet(ctx context.Context, home string) (Set, []string) {
	var s Set
	var warn []string
	cfg := ConfigPath(home)
	if es, err := LoadNeverList(cfg); err != nil {
		warn = append(warn, "never-list unreadable: "+err.Error())
	} else {
		for _, e := range es {
			s.Add(e)
		}
	}
	db := filepath.Join(filepath.Dir(cfg), "state.db")
	if _, err := os.Stat(db); err != nil {
		return s, warn
	}
	st, err := OpenStore(db)
	if err != nil {
		return s, append(warn, "state store unreadable; the cached exclusion entries were not used")
	}
	defer func() { _ = st.Close() }()
	rows, err := st.ExclusionEntries(ctx)
	if err != nil {
		return s, append(warn, "cached exclusion entries unreadable")
	}
	for _, r := range rows {
		s.Add(Entry{r.Kind, r.Value, r.Source})
	}
	return s, warn
}

// OpenStore opens the state database at path, retrying briefly while another process holds the writer.
func OpenStore(path string) (*store.Store, error) {
	for end := time.Now().Add(2 * time.Second); ; time.Sleep(50 * time.Millisecond) {
		st, err := store.Open(path, store.Options{})
		busy := err != nil && (strings.Contains(err.Error(), "SQLITE_BUSY") || strings.Contains(err.Error(), "database is locked"))
		if err == nil || !busy || time.Now().After(end) {
			return st, err
		}
	}
}
