package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestRaiseIncarnation(t *testing.T) {
	ctx := context.Background()
	st, err := Open(filepath.Join(t.TempDir(), "state.db"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()
	if n, err := st.HighestIncarnation(ctx); err != nil || n != 1 {
		t.Fatalf("empty table: %d, %v; want 1", n, err)
	}
	if n, err := st.RaiseIncarnation(ctx, 7); err != nil || n != 7 {
		t.Fatalf("raise to 7: %d, %v", n, err)
	}
	if n, _ := st.HighestIncarnation(ctx); n != 7 {
		t.Fatalf("highest = %d, want 7", n)
	}
	if n, err := st.RaiseIncarnation(ctx, 3); err != nil || n != 7 {
		t.Fatalf("a lower floor must not lower it: %d, %v", n, err)
	}
	if _, err := st.Register(ctx, 4242, "t", "b"); err != nil { // a new process id starts at 1 but the highest stays 7
		t.Fatal(err)
	}
	if n, _ := st.HighestIncarnation(ctx); n != 7 {
		t.Fatalf("highest after a new process = %d, want 7", n)
	}
}
