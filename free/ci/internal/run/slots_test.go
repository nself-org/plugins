package run

import (
	"sync"
	"testing"
)

func TestSlotPoolHandsOutDistinctLowestSlots(t *testing.T) {
	var p slotPool
	dir := t.TempDir()
	a, b, c := p.take(dir), p.take(dir), p.take(dir)
	if a != 0 || b != 1 || c != 2 {
		t.Fatalf("%d %d %d", a, b, c)
	}
	p.put(1)
	if got := p.take(dir); got != 1 {
		t.Fatalf("freed slot not reused: %d", got)
	}
}

func TestSlotPoolConcurrentTakesNeverShare(t *testing.T) {
	var p slotPool
	dir := t.TempDir()
	var wg sync.WaitGroup
	got := make(chan int, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); got <- p.take(dir) }()
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

// Two pools stand in for two coordinators that share one home: they must never get the same slot.
func TestSlotPoolsOfTwoCoordinatorsNeverShareASlot(t *testing.T) {
	var a, b slotPool
	dir := t.TempDir()
	first, second := a.take(dir), b.take(dir)
	if first == second {
		t.Fatalf("both coordinators got slot %d", first)
	}
	a.put(first)
	if got := b.take(dir); got != first {
		t.Fatalf("slot %d not released across coordinators (got %d)", first, got)
	}
}

func TestSlotPoolWithoutADirIsProcessLocal(t *testing.T) {
	var p slotPool
	if a, b := p.take(""), p.take(""); a == b {
		t.Fatalf("%d %d", a, b)
	}
}
