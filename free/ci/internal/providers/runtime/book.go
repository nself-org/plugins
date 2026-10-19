package runtime

// Purpose: the handle book: which provider run an attempt was dispatched as (Epic D4, idempotent
// dispatch). The driver writes a handle right after Dispatch and reads it first on every Run, so a
// restarted coordinator polls the run it already started instead of starting a second one; the lease
// profile's Liveness reads the same book by attempt id.
// Inputs:  an attempt id and a providers.Handle.
// Outputs: the stored handle.
// Constraints: the first handle written for an attempt wins. The store-backed book is the durable
//          one (dispatch_intent in the host-provider set); the memory book serves tests and a
//          coordinator with no store. A handle holds a run id and a nonce, never a token.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Book remembers the handle of each dispatched attempt.
type Book interface {
	Get(ctx context.Context, attemptID string) (providers.Handle, bool, error)
	Put(ctx context.Context, attemptID string, h providers.Handle) error
}

// NewMemBook returns a process-local book.
func NewMemBook() Book { return &memBook{m: map[string]providers.Handle{}} }

type memBook struct {
	mu sync.Mutex
	m  map[string]providers.Handle
}

func (b *memBook) Get(_ context.Context, id string) (providers.Handle, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	h, ok := b.m[id]
	return h, ok, nil
}

func (b *memBook) Put(_ context.Context, id string, h providers.Handle) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.m[id]; !ok {
		b.m[id] = h
	}
	return nil
}

// StoreBook returns a book backed by the host-provider store set.
func StoreBook(st *store.Store) Book { return storeBook{st} }

type storeBook struct{ st *store.Store }

func (b storeBook) Get(ctx context.Context, id string) (providers.Handle, bool, error) {
	d, ok, err := b.st.DispatchIntentGet(ctx, id)
	if err != nil || !ok {
		return providers.Handle{}, false, err
	}
	return providers.Handle{Provider: d.Provider, RunID: d.RunID, Nonce: d.Nonce, Ref: d.Ref}, true, nil
}

func (b storeBook) Put(ctx context.Context, id string, h providers.Handle) error {
	return b.st.DispatchIntentPut(ctx, store.DispatchIntent{AttemptID: id, Provider: h.Provider, Nonce: h.Nonce, RunID: h.RunID, Ref: h.Ref})
}
