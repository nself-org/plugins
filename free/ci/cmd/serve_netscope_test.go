package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/store"
)

func TestCoordinatorExclusionsReadLiveStore(t *testing.T) {
	st := serveTestStore(t)
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci:\n  never_hosts: [192.0.2.9]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := coordinatorExclusions(st, home)
	ctx := context.Background()
	for _, address := range []string{"192.0.2.1", "192.0.2.2"} {
		t.Run(address, func(t *testing.T) {
			if err := st.ExclusionReplace(ctx, "project", []store.ExclusionEntry{{Root: "project", Kind: "address", Value: address, Source: "inventory"}}); err != nil {
				t.Fatal(err)
			}
			got, err := source(ctx)
			if err != nil || !reflect.DeepEqual(got.Addresses, []string{address, "192.0.2.9"}) {
				t.Fatalf("live exclusions: %+v %v", got, err)
			}
		})
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("ci: [invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := source(ctx); err == nil {
		t.Fatal("unreadable never-list was accepted")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := source(ctx); err == nil {
		t.Fatal("unreadable coordinator source was accepted")
	}
}
