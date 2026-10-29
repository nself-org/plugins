//go:build linux

package run

import "testing"

func TestHostSlotsIgnoreRunHome(t *testing.T) {
	privateSlots(t)
	var a, b slotPool
	first := a.take(slotDir(t.TempDir()))
	defer a.put(first)
	second := b.take(slotDir(t.TempDir()))
	defer b.put(second)
	if first < 0 || second < 0 || first == second {
		t.Fatalf("different homes received %d, %d", first, second)
	}
	a.put(first)
	reused := b.take(slotDir(t.TempDir()))
	defer b.put(reused)
	if reused != first {
		t.Fatalf("released host slot %d not reused: %d", first, reused)
	}
}

// Slots a test leaves held must not reach the next test: each gets a private namespace.
func TestPrivateSlotsDoNotLeakAcrossTests(t *testing.T) {
	var leaked slotPool
	t.Run("leaks slot 0", func(t *testing.T) {
		privateSlots(t)
		if n := leaked.take(slotDir(t.TempDir())); n != 0 {
			t.Fatalf("first take got %d", n)
		}
	})
	t.Run("sees slot 0 free", func(t *testing.T) {
		privateSlots(t)
		var p slotPool
		defer releaseAll(&p)
		if n := p.take(slotDir(t.TempDir())); n != 0 {
			t.Fatalf("leaked slot visible: got %d", n)
		}
	})
	releaseAll(&leaked)
}
