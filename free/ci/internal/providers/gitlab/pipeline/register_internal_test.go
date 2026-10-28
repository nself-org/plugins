package pipeline

// Purpose: UseStore and ReleaseStore move the default provider's dispatch journal between the coordinator's
//          store and memory (D-1346).
// Inputs:  a temp store.
// Outputs: pass or fail.
// Constraints: no network.

import (
	"path/filepath"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestUseStoreAndReleaseStore(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close(); ReleaseStore() })
	if _, ok := defaultProvider.jrnl().(*memJournal); !ok {
		t.Fatalf("journal before UseStore is %T, want the memory journal", defaultProvider.jrnl())
	}
	UseStore(st)
	if _, ok := defaultProvider.jrnl().(storeJournal); !ok {
		t.Fatalf("journal after UseStore is %T, want the store journal", defaultProvider.jrnl())
	}
	ReleaseStore()
	if _, ok := defaultProvider.jrnl().(*memJournal); !ok {
		t.Fatalf("journal after ReleaseStore is %T, want the memory journal", defaultProvider.jrnl())
	}
}
