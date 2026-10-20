package runtime

// Purpose: one call that points the runtime at the coordinator's store (Epic D4).
// Inputs:  the coordinator's *store.Store (the host-provider set migrates with it).
// Outputs: the driver keeps its handles in dispatch_intent, so a restart polls the runs it started;
//          the contributor takes reservations from the same store. It returns the Book to hand to
//          RegisterProfiles so the lease profile reads the same handles.
// Constraints: call once at coordinator start, before the first dispatch.

import "github.com/nself-org/plugins/free/ci/internal/store"

// ReleaseStore detaches the driver and the contributor from the coordinator's store (the coordinator is
// stopping and the store is about to close): handles go back to an in-memory book.
func ReleaseStore() {
	defaultDriver.SetBook(NewMemBook())
	defaultContributor.SetStore(nil)
}

// UseStore makes the default driver and contributor use st and returns the durable Book.
func UseStore(st *store.Store) Book {
	b := StoreBook(st)
	defaultDriver.SetBook(b)
	defaultContributor.SetStore(st)
	return b
}
