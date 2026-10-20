package main

import (
	"context"
	"slices"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/providers/ephemeral/cloudvm"
)

// TestServeBurstBindsAnEngineOnlyForAnEnabledProvider: `serve` builds and binds the burst engine when the
// operator policy enables a provider it can build, keeps the coordinator id across restarts (the label the
// sweeper deletes by), and unbinds it when serve ends (D-1329).
func TestServeBurstBindsAnEngineOnlyForAnEnabledProvider(t *testing.T) {
	r := newEnrollRig(t)
	ctx := context.Background()

	undo, err := serveBurst(ctx, r.st, r.home)
	if err != nil {
		t.Fatal(err)
	}
	if burst.Bound() != nil {
		t.Fatal("no provider is enabled, yet an engine is bound")
	}
	undo()

	r.setBurst(cloudvm.DefaultProviderID, true)
	undo, err = serveBurst(ctx, r.st, r.home)
	if err != nil {
		t.Fatal(err)
	}
	e := burst.Bound()
	if e == nil || !slices.Equal(e.ProviderIDs(), []string{cloudvm.DefaultProviderID}) {
		t.Fatalf("bound engine = %v, want one with the %s provider", e, cloudvm.DefaultProviderID)
	}
	id := e.Coordinator()
	undo()
	if burst.Bound() != nil {
		t.Fatal("serve ended but the engine is still bound")
	}

	undo, err = serveBurst(ctx, r.st, r.home)
	if err != nil {
		t.Fatal(err)
	}
	defer undo()
	if again := burst.Bound(); again == nil || again.Coordinator() != id {
		t.Fatalf("coordinator id changed across a restart: %q then %v", id, again)
	}
}
