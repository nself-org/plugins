package slots

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func open(t *testing.T, dir string, total int) *Table {
	t.Helper()
	tb, err := Open(dir, total)
	if err != nil {
		t.Fatal(err)
	}
	return tb
}

func isFull(err error) bool {
	var se *Error
	return errors.As(err, &se) && se.Reason == "slots_full" && strings.HasPrefix(err.Error(), "E654")
}

func skipWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the windows lock is a stub")
	}
}

// TestSlotsCapAndRelease proves the table never hands out more than its slot
// count and that a release makes the slot available again (X10).
func TestSlotsCapAndRelease(t *testing.T) {
	skipWindows(t)
	tb := open(t, t.TempDir(), 2)
	a, err := tb.Acquire(Self("c1", "l1"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := tb.Acquire(Self("c2", "l2"))
	if err != nil || a.Index == b.Index {
		t.Fatalf("second acquire: %v %v %v", a, b, err)
	}
	if _, err := tb.Acquire(Self("c3", "l3")); !isFull(err) {
		t.Fatalf("third acquire = %v, want E654 slots_full", err)
	}
	if n, _ := tb.Used(); n != 2 {
		t.Fatalf("used %d", n)
	}
	if err := tb.Release(a); err != nil {
		t.Fatal(err)
	}
	if err := tb.Release(a); err != nil { // idempotent
		t.Fatal(err)
	}
	if c, err := tb.Acquire(Self("c3", "l3")); err != nil || c.Index != a.Index {
		t.Fatalf("reacquire: %v %v", c, err)
	}
}

// TestSlotsRaceNeverExceedsTotal proves concurrent acquirers (separate table
// handles, as two coordinators' sessions have) never overshoot the cap.
func TestSlotsRaceNeverExceedsTotal(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	const total, tries = 3, 24
	var wg sync.WaitGroup
	var mu sync.Mutex
	won, full := 0, 0
	for i := range tries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := open(t, dir, total).Acquire(Self("c"+strconv.Itoa(i%2), "l"+strconv.Itoa(i)))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case isFull(err):
				full++
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if won != total || full != tries-total {
		t.Fatalf("won %d full %d", won, full)
	}
}

// TestSlotsReclaimDeadOwner proves a crashed holder's slot comes back: its pid
// is gone, or its pid was reused by a process with another start time.
func TestSlotsReclaimDeadOwner(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	tb := open(t, dir, 2)
	dead := Owner{PID: 2147483646, Start: "1", Coordinator: "gone", Lease: "x"}
	reused := Self("reused", "y")
	reused.Start = "not-the-start-time"
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		reused = dead
	}
	table := `{"slots":[{"index":0,"since":"2026-01-01T00:00:00Z","pid":` + strconv.Itoa(dead.PID) + `,"start":"1","coordinator":"gone","lease":"x"},` +
		`{"index":1,"since":"2026-01-01T00:00:00Z","pid":` + strconv.Itoa(reused.PID) + `,"start":"` + reused.Start + `","coordinator":"reused","lease":"y"}]}`
	if err := os.WriteFile(filepath.Join(dir, tableName), []byte(table), 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := tb.Used(); err != nil || n != 0 {
		t.Fatalf("used %d err %v after reclaim", n, err)
	}
	if _, err := tb.Acquire(Self("c", "l")); err != nil {
		t.Fatal(err)
	}
}

// TestSlotsLiveOwnerKeepsSlot proves a live holder is never reclaimed.
func TestSlotsLiveOwnerKeepsSlot(t *testing.T) {
	skipWindows(t)
	tb := open(t, t.TempDir(), 1)
	if _, err := tb.Acquire(Self("c", "l")); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.Acquire(Self("d", "m")); !isFull(err) {
		t.Fatalf("got %v", err)
	}
}

func TestSlotsOpenRejectsZero(t *testing.T) {
	if _, err := Open(t.TempDir(), 0); err == nil {
		t.Fatal("accepted 0 slots")
	}
}

// TestSlotsCorruptTableIsAnError proves a damaged table is reported, never
// treated as empty (which would let a node over-commit).
func TestSlotsCorruptTableIsAnError(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, tableName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if tb, err := Open(dir, 1); err == nil || isFull(err) || tb != nil {
		t.Fatalf("got %v", err)
	}
}

// TestSlotsCapCannotBeRaisedByAnotherOpen proves the host cap is the one the
// table was created with: a second serve process started with a larger --slots
// (a coordinator picks its own command line) does not widen it; only SetCap does.
func TestSlotsCapCannotBeRaisedByAnotherOpen(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	small := open(t, dir, 2)
	big := open(t, dir, 64)
	if got := open(t, dir, 1); got.Total() != 2 {
		t.Fatalf("a smaller ask must not shrink the host cap: %d", got.Total())
	}
	if big.Total() != 2 {
		t.Fatalf("second open got %d slots, want the host cap 2", big.Total())
	}
	for i := range 2 {
		if _, err := big.Acquire(Self("b", "l"+strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := small.Acquire(Self("a", "x")); !isFull(err) {
		t.Fatalf("got %v", err)
	}
	if err := SetCap(dir, 3); err != nil {
		t.Fatal(err)
	}
	if got := open(t, dir, 64); got.Total() != 3 {
		t.Fatalf("after SetCap: %d", got.Total())
	}
	if err := SetCap(dir, 0); err == nil {
		t.Fatal("accepted a zero cap")
	}
}

// TestSlotsLoweredCapIsHonouredByOpenHandles proves lowering the cap applies to
// handles that were already open.
func TestSlotsLoweredCapIsHonouredByOpenHandles(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	tb := open(t, dir, 3)
	if _, err := tb.Acquire(Self("a", "1")); err != nil {
		t.Fatal(err)
	}
	if err := SetCap(dir, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := tb.Acquire(Self("a", "2")); !isFull(err) {
		t.Fatalf("got %v", err)
	}
}

// TestSlotsStaleReleaseDoesNotFreeNewerSlot proves releasing a slot twice
// cannot free the same index when it was reacquired by the same owner.
func TestSlotsStaleReleaseDoesNotFreeNewerSlot(t *testing.T) {
	skipWindows(t)
	tb := open(t, t.TempDir(), 1)
	owner := Self("c", "l")
	a, _ := tb.Acquire(owner)
	_ = tb.Release(a)
	b, err := tb.Acquire(owner)
	if err != nil {
		t.Fatal(err)
	}
	_ = tb.Release(a) // stale handle
	if n, _ := tb.Used(); n != 1 {
		t.Fatalf("stale release freed the newer slot (used %d)", n)
	}
	_ = tb.Release(b)
}

// TestSlotsLowerOnlyLowers proves a handle can lower itself in memory and never raise.
func TestSlotsLowerOnlyLowers(t *testing.T) {
	skipWindows(t)
	tb := open(t, t.TempDir(), 3)
	tb.Lower(5)
	if tb.Total() != 3 {
		t.Fatalf("raised to %d", tb.Total())
	}
	tb.Lower(1)
	if tb.Total() != 1 {
		t.Fatalf("lowered to %d", tb.Total())
	}
	if cap, _, _ := Status(tb.dir); cap != 3 {
		t.Fatalf("on-disk cap changed to %d", cap)
	}
}
