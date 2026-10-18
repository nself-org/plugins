package run

import (
	"sync"
	"testing"
)

func TestSlotPoolHandsOutDistinctLowestSlots(t *testing.T) {
	var p slotPool
	a, b, c := p.take(), p.take(), p.take()
	if a != 0 || b != 1 || c != 2 {
		t.Fatalf("%d %d %d", a, b, c)
	}
	p.put(1)
	if got := p.take(); got != 1 {
		t.Fatalf("freed slot not reused: %d", got)
	}
}

func TestSlotPoolConcurrentTakesNeverShare(t *testing.T) {
	var p slotPool
	var wg sync.WaitGroup
	got := make(chan int, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); got <- p.take() }()
	}
	wg.Wait()
	close(got)
	seen := map[int]bool{}
	for n := range got {
		if seen[n] {
			t.Fatalf("slot %d handed out twice", n)
		}
		seen[n] = true
	}
}
