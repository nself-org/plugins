package services

// Purpose: tests of the Pool lifecycle (P7-CI-42, D-0975): attempts run side
// by side, a live key is refused, the context safety net keeps the entry until
// Down, and a Down waits for a teardown the safety net started.

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

func (s *stub) saw(sub string) bool { return strings.Contains(s.joined(), sub) }

func TestPoolSafetyNetTearsDownAndKeepsKeyUntilDown(t *testing.T) {
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
	// The key is still live: a repeat Up must not start before the owner's
	// Down, or that Down would find the later job's entry.
	if _, err := pl.Up(context.Background(), prov, "k", specs); err == nil {
		t.Fatal("the key was released by the safety net before Down")
	}
	if err := pl.Down(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	if _, err := pl.Up(ctx3, prov, "k", specs); err != nil {
		t.Fatalf("key not released by Down: %v", err)
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

func TestPoolRunsDifferentAttemptsAtOnce(t *testing.T) {
	pl, prov, _ := newPoolRig(t)
	specs := []Spec{{Name: "postgres"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pl.Up(ctx, prov, "attempt-1", specs); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Up(ctx, prov, "attempt-2", specs); err != nil {
		t.Fatalf("a second attempt must not wait for the first: %v", err)
	}
	if err := pl.Down(ctx, "attempt-1"); err != nil {
		t.Fatal(err)
	}
	if err := pl.Down(ctx, "attempt-2"); err != nil {
		t.Fatal(err)
	}
}

func TestPoolRefusesALiveAttemptKey(t *testing.T) {
	pl, prov, _ := newPoolRig(t)
	specs := []Spec{{Name: "postgres"}}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := pl.Up(ctx, prov, "attempt-1", specs); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := pl.Up(ctx, prov, "attempt-1", specs); err == nil || time.Since(start) > time.Second {
		t.Fatalf("a second Up for a live attempt must fail at once, not wait (err %v after %v)", err, time.Since(start))
	}
	if err := pl.Down(ctx, "attempt-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := pl.Up(ctx, prov, "attempt-1", specs); err != nil {
		t.Fatalf("the key is free again after Down: %v", err)
	}
	_ = pl.Down(ctx, "attempt-1")
}
