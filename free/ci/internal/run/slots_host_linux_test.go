//go:build linux

package run

import "testing"

func TestHostSlotsIgnoreRunHome(t *testing.T) {
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
