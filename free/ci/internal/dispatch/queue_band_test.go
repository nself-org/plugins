package dispatch

// Purpose: D-1265. The background admission step asks the dispatcher whether an
// interactive job waits; only a queued interactive-band job answers yes.

import "testing"

func TestDispatcherInteractiveQueued(t *testing.T) {
	st := openStore(t)
	d := newDispatcher(t, Config{Store: st, World: liveWorld(st, nil, testRunner("r1", "local", 1))})
	if d.InteractiveQueued() {
		t.Fatal("an empty queue has no interactive job")
	}
	add := func(attempt, band string, foreground bool) *entry {
		j := testQueued(attempt)
		j.Sched.Pipeline.Band, j.Foreground = band, foreground
		e, err := d.enqueue(j)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	bg := add("bg", "background", false)
	add("norm", "", false) // no band and not foreground: normal
	if d.InteractiveQueued() {
		t.Fatal("background and normal jobs are not interactive work")
	}
	fg := add("fg", "", true) // a foreground job with no declared band is interactive (bandOf)
	if !d.InteractiveQueued() {
		t.Fatal("a queued foreground job is interactive work")
	}
	d.drop(fg, errClosed)
	if d.InteractiveQueued() {
		t.Fatal("a job that left the queue no longer counts")
	}
	add("fg2", "interactive", false)
	if !d.InteractiveQueued() {
		t.Fatal("a declared interactive band counts")
	}
	_ = bg
}
