package yjs

import (
	"context"

	"github.com/nself-org/nself-crdt/internal/store"
)

// Persistence wraps Store with Yjs-specific helpers.
type Persistence struct {
	st *store.Store
}

// NewPersistence creates a Persistence helper.
func NewPersistence(st *store.Store) *Persistence {
	return &Persistence{st: st}
}

// LoadState returns the raw Yjs binary state for a doc, or nil if not found.
func (p *Persistence) LoadState(ctx context.Context, docID string) ([]byte, error) {
	doc, err := p.st.LoadDocument(ctx, docID)
	if err == store.ErrNotFound {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return doc.State, nil
}

// SaveState persists a full Yjs document state (replaces prior).
func (p *Persistence) SaveState(ctx context.Context, docID string, state []byte) error {
	return p.st.UpsertDocument(ctx, docID, "yjs", state)
}
