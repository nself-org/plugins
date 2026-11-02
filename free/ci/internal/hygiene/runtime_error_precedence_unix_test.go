//go:build unix

package hygiene

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/exec"
)

func TestConcurrentReductionPreservesCompletedErrorAfterCancellation(t *testing.T) {
	starts, _ := lifecycleRuntimes(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inner, reference := &ExecRuntime{}, &ExecRuntime{}
	for _, bin := range inner.binaries() {
		inner.recordProbe(bin, runtimeUp)
		reference.recordProbe(bin, runtimeUp)
	}
	_, wantErr := (serialReferenceRuntime{reference}).List(ctx, exec.KindContainer, "")
	if wantErr == nil || !strings.Contains(wantErr.Error(), "unsafe name or label") {
		t.Fatalf("serial error = %v; want malformed docker resource", wantErr)
	}

	results := make([][]listingResult, len(Kinds))
	for i := range results {
		results[i] = make([]listingResult, len(inner.binaries()))
	}
	// Complete docker ps before cancellation, then let every other worker block. This
	// controls the interleaving without relying on a delay for parsing to finish.
	results[0][0] = queryListing(ctx, "docker", exec.KindContainer, "")
	var workers sync.WaitGroup
	for i, kind := range Kinds {
		for j, bin := range inner.binaries() {
			if i == 0 && j == 0 {
				continue
			}
			workers.Add(1)
			go func(i, j int, bin string, kind exec.ResourceKind) {
				defer workers.Done()
				results[i][j] = queryListing(ctx, bin, kind, "")
			}(i, j, bin, kind)
		}
	}
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	defer func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("listing workers did not join after cancellation")
		}
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	// One serial call plus all six concurrent listing calls must have started.
	for len(strings.Fields(readCalls(t, starts))) < 21 {
		select {
		case <-deadline.C:
			t.Fatal("listing workers did not all start")
		case <-tick.C:
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation did not join listing workers")
	}
	if !errors.Is(results[0][1].contextErr, context.Canceled) {
		t.Fatalf("blocked podman listing cancellation = %v", results[0][1].contextErr)
	}
	snapshots := inner.reduceKinds(ctx, inner.binaries(), results)
	assertConcurrentError(t, snapshots[exec.KindContainer].err, wantErr)
}

func TestConcurrentReductionKeepsEarlierBinaryCancellation(t *testing.T) {
	inner := &ExecRuntime{}
	for _, bin := range inner.binaries() {
		inner.recordProbe(bin, runtimeUp)
	}
	parseErr := errors.New("malformed later listing")
	results := make([][]listingResult, len(Kinds))
	for i := range results {
		results[i] = []listingResult{
			{err: errors.New("command killed"), contextErr: context.Canceled, queried: true},
			{err: parseErr, queried: true},
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for kind, snapshot := range inner.reduceKinds(ctx, inner.binaries(), results) {
		if !errors.Is(snapshot.err, context.Canceled) {
			t.Errorf("%s error = %v; want earlier binary cancellation", kind, snapshot.err)
		}
	}
}
