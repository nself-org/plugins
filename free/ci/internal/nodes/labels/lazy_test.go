package labels

// Purpose: tests of the lazy production reader and of the world label source (P7-NODE-30): no store file
// means no labels, a live store wins, and Source renders the placement form.
// Inputs:  temp stores. Outputs: pass or fail.
// Constraints: the process-wide world hook is not touched (RegisterSource is exercised by the sim leg).

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestLazyReader(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.db")
	r := &LazyReader{Path: func() string { return path }}
	t.Cleanup(r.Close)
	if got, err := r.NodeLabels(ctx, "n1"); err != nil || len(got) != 0 {
		t.Fatalf("no store file: %v %v", got, err)
	}
	st, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if err = st.NodeLabelsReplace(ctx, "n1", []store.NodeLabel{{Key: "xcode"}, {Key: "arch", Value: "arm64"}}, "t"); err != nil {
		t.Fatal(err)
	}
	if err = st.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := Source(r)(ctx, "n1")
	if err != nil || !reflect.DeepEqual(got, []string{"arch=arm64", "xcode"}) {
		t.Fatalf("source: %v %v", got, err)
	}
	live := newEnv(t).Store
	if err = live.NodeLabelsReplace(ctx, "n1", []store.NodeLabel{{Key: "live"}}, "t"); err != nil {
		t.Fatal(err)
	}
	r.Live = func() *store.Store { return live }
	if got, err = Source(r)(ctx, "n1"); err != nil || !reflect.DeepEqual(got, []string{"live"}) {
		t.Fatalf("live store: %v %v", got, err)
	}
}
