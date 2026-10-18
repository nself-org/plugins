package services

// Purpose: tests of the Pool lifecycle (P7-CI-42): per-key serialisation,
// release on Down, the context safety net that keeps the slot until Down, and
// a Down that waits for a teardown the safety net started.

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func newPoolRig(t *testing.T) (*Pool, *Provisioner, *stub) {
	t.Helper()
	s := &stub{}
	return NewPool(), newProv(t, s), s
}

func TestPoolSerialisesIdenticalRequests(t *testing.T) {
	pl, prov, _ := newPoolRig(t)
	specs := []Spec{{Name: "postgres"}}
	ctx := context.Background()
	if _, err := pl.Up(ctx, prov, "k1", specs); err != nil {
		t.Fatal(err)
	}
	second := make(chan error, 1)
	go func() { _, err := pl.Up(ctx, prov, "k1", specs); second <- err }()
	select {
	case err := <-second:
		t.Fatalf("the second Up must wait for the first Down, returned %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	// A different key is not held back.
	if _, err := pl.Up(ctx, prov, "k2", specs); err != nil {
		t.Fatalf("a different key must not wait: %v", err)
	}
	if err := pl.Down(ctx, "k2"); err != nil {
		t.Fatal(err)
	}
	if err := pl.Down(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second Up after Down: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the second Up never proceeded after Down")
	}
	if err := pl.Down(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
}

func TestPoolWaitEndsWithContext(t *testing.T) {
	pl, prov, _ := newPoolRig(t)
	specs := []Spec{{Name: "postgres"}}
	if _, err := pl.Up(context.Background(), prov, "k", specs); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := pl.Up(ctx, prov, "k", specs); err == nil {
		t.Fatal("a waiting Up must fail when its context ends")
	}
	if err := pl.Down(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
}

func (s *stub) saw(sub string) bool { return strings.Contains(s.joined(), sub) }

func TestPoolSafetyNetTearsDownAndKeepsSlotUntilDown(t *testing.T) {
	pl, prov, s := newPoolRig(t)
	specs := []Spec{{Name: "postgres"}}
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := pl.Up(ctx, prov, "k", specs); err != nil {
		t.Fatal(err)
	}
	cancel() // the job context ends before Down
	deadline := time.Now().Add(5 * time.Second)
	for !s.saw("stop --volumes --remove-orphans") {
		if time.Now().After(deadline) {
			t.Fatal("no teardown after the context ended")
		}
		time.Sleep(20 * time.Millisecond)
	}
	// The slot is still held: a later job must not start before the owner's
	// Down, or that Down would find the later job's entry.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel2()
	if _, err := pl.Up(ctx2, prov, "k", specs); err == nil {
		t.Fatal("the slot was released by the safety net before Down")
	}
	if err := pl.Down(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	if _, err := pl.Up(ctx3, prov, "k", specs); err != nil {
		t.Fatalf("slot not released by Down: %v", err)
	}
	_ = pl.Down(context.Background(), "k")
}

// blockRunner holds the `stop` call until released.
type blockRunner struct {
	*stub
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockRunner) Run(ctx context.Context, c Call) (Result, error) {
	if strings.Contains(strings.Join(c.Argv, " "), "stop --volumes") {
		b.once.Do(func() { close(b.started) })
		<-b.release
	}
	return b.stub.Run(ctx, c)
}

func TestPoolDownWaitsForARunningSafetyNet(t *testing.T) {
	b := &blockRunner{stub: &stub{}, started: make(chan struct{}), release: make(chan struct{})}
	prov := newProv(t, b.stub)
	prov.Run = b
	pl := NewPool()
	ctx, cancel := context.WithCancel(context.Background())
	if _, err := pl.Up(ctx, prov, "k", []Spec{{Name: "postgres"}}); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-b.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the safety net never started")
	}
	down := make(chan error, 1)
	go func() { down <- pl.Down(context.Background(), "k") }()
	select {
	case err := <-down:
		t.Fatalf("Down returned while the teardown was still running: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(b.release)
	select {
	case err := <-down:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Down never returned after the teardown finished")
	}
}
