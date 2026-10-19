package runtime

// Purpose: the handle book: which provider run an attempt was dispatched as, and for which lease
// epoch (Epic D4, idempotent dispatch). The driver writes a handle right after Dispatch and reads it
// first on every Run, so a second carrier of the same lease polls the run that exists instead of
// starting another; the lease profile's Liveness reads the same book by attempt id.
// Inputs:  an attempt id and an Entry (the providers.Handle and the lease epoch).
// Outputs: the entry that stands for the attempt.
// Constraints: for one attempt the entry of the highest epoch wins (a retried attempt has a higher
//          epoch and replaces its predecessor's run; an equal epoch keeps the first writer, and the
//          loser learns the winner from Put). Delete removes an entry only while it still names the
//          given run. The store-backed book is the durable one (dispatch_intent in the host-provider
//          set); the memory book serves tests and a coordinator with no store. An entry holds a run
//          id and a nonce, never a token.

import (
	"context"
	"sync"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// Entry is one dispatched run and the lease epoch it was dispatched for.
type Entry struct {
	Handle providers.Handle
	Epoch  int64
}

// Book remembers the handle of each dispatched attempt.
type Book interface {
	Get(ctx context.Context, attemptID string) (Entry, bool, error)
	// Put stores e unless the book already holds an entry of an equal or higher epoch, and returns the
	// entry that stands afterwards.
	Put(ctx context.Context, attemptID string, e Entry) (Entry, error)
	// Delete forgets the entry while it still names runID.
	Delete(ctx context.Context, attemptID, runID string) error
}

// NewMemBook returns a process-local book.
func NewMemBook() Book { return &memBook{m: map[string]Entry{}} }

type memBook struct {
	mu sync.Mutex
	m  map[string]Entry
}

func (b *memBook) Get(_ context.Context, id string) (Entry, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.m[id]
	return e, ok, nil
}

func (b *memBook) Put(_ context.Context, id string, e Entry) (Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if old, ok := b.m[id]; !ok || e.Epoch > old.Epoch {
		b.m[id] = e
	}
	return b.m[id], nil
}

func (b *memBook) Delete(_ context.Context, id, runID string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if old, ok := b.m[id]; ok && old.Handle.RunID == runID {
		delete(b.m, id)
	}
	return nil
}

// StoreBook returns a book backed by the host-provider store set.
func StoreBook(st *store.Store) Book { return storeBook{st} }

type storeBook struct{ st *store.Store }

func entryOf(d store.DispatchIntent) Entry {
	return Entry{Handle: providers.Handle{Provider: d.Provider, RunID: d.RunID, Nonce: d.Nonce, Ref: d.Ref}, Epoch: d.Epoch}
}

func (b storeBook) Get(ctx context.Context, id string) (Entry, bool, error) {
	d, ok, err := b.st.DispatchIntentGet(ctx, id)
	if err != nil || !ok {
		return Entry{}, false, err
	}
	return entryOf(d), true, nil
}

func (b storeBook) Put(ctx context.Context, id string, e Entry) (Entry, error) {
	d, err := b.st.DispatchIntentPut(ctx, store.DispatchIntent{AttemptID: id, Provider: e.Handle.Provider, Nonce: e.Handle.Nonce, RunID: e.Handle.RunID, Ref: e.Handle.Ref, Epoch: e.Epoch})
	if err != nil {
		return Entry{}, err
	}
	return entryOf(d), nil
}

func (b storeBook) Delete(ctx context.Context, id, runID string) error {
	return b.st.DispatchIntentDelete(ctx, id, runID)
}
