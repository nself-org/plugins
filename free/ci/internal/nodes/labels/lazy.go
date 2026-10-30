package labels

// Purpose: the production Reader of the world label source (P7-NODE-30): it reads the assigned labels from
// the coordinator's live store while a serve runs, else from the CI home store, opened on first use.
// Inputs:  Live (the serve's store or nil) and Path (the home store path) resolvers.
// Outputs: the node's labels; none when no store file exists yet (nothing was ever labelled).
// Constraints: one open handle per path for the life of the process (the world builder asks once per
//          runner per plan); a store that exists but cannot be read is an error, never an empty answer.

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

// LazyReader implements Reader over late-bound stores.
type LazyReader struct {
	Live func() *store.Store
	Path func() string

	mu     sync.Mutex
	stores map[string]*store.Store
}

// NodeLabels returns the node's assigned labels.
func (r *LazyReader) NodeLabels(ctx context.Context, nodeID string) ([]store.NodeLabel, error) {
	if r.Live != nil {
		if st := r.Live(); st != nil {
			return st.NodeLabels(ctx, nodeID)
		}
	}
	path := r.Path()
	r.mu.Lock()
	st := r.stores[path]
	if st == nil {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			r.mu.Unlock()
			return nil, nil
		}
		var err error
		if st, err = store.Open(path, store.Options{}); err != nil {
			r.mu.Unlock()
			return nil, err
		}
		if r.stores == nil {
			r.stores = map[string]*store.Store{}
		}
		r.stores[path] = st
	}
	r.mu.Unlock()
	return st.NodeLabels(ctx, nodeID)
}

// Close closes every store the reader opened.
func (r *LazyReader) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for p, st := range r.stores {
		_ = st.Close()
		delete(r.stores, p)
	}
}
