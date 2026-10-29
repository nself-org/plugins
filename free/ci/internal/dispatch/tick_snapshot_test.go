package dispatch

import (
	"context"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/sched"
)

// TestTickWorldsShareOneSnapshot pins D-1412: the worlds of a tick's pipelines are built from one store
// snapshot. The one slot is held when the first world is built and frees right after it; without the
// shared snapshot the second pipeline's world sees the slot free and its job overtakes the older job of
// the first pipeline.
func TestTickWorldsShareOneSnapshot(t *testing.T) {
	st := openStore(t)
	clock := newClock()
	live := liveWorld(st, nil, testRunner("local", "local", 1))
	holder := queueAttempt(t, st, "pipe-hold", "project", "h")
	hd := newDispatcher(t, Config{Store: st, World: live, Clock: clock.Now})
	he := enqueueAt(t, hd, clock, holder)
	if err := hd.Tick(context.Background()); err != nil || !resolved(he[0]) {
		t.Fatalf("the holder must be admitted: %v", err)
	}
	builds := 0
	world := func(ctx context.Context, p sched.Pipeline) (sched.World, string, error) {
		w, digest, err := live(ctx, p)
		if builds++; builds == 1 {
			pass(t, st, holder) // capacity frees between the first build and the second
		}
		return w, digest, err
	}
	d := newDispatcher(t, Config{Store: st, World: world, Clock: clock.Now})
	older, younger := queueAttempt(t, st, "pipe-a", "project", "j"), queueAttempt(t, st, "pipe-b", "project", "j")
	olderJob, youngerJob := testQueued(older), testQueued(younger)
	olderJob.Sched.Pipeline.ID = "pipe-a"
	youngerJob.Sched.Pipeline.ID = "pipe-b"
	eo, err := d.enqueue(olderJob)
	if err != nil {
		t.Fatal(err)
	}
	clock.Advance(1e9)
	ey, err := d.enqueue(youngerJob)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if builds != 2 {
		t.Fatalf("%d worlds built, want 2", builds)
	}
	if resolved(ey) {
		t.Fatalf("the younger pipeline's job was admitted ahead of the older one (older admitted: %v)", resolved(eo))
	}
	if err = d.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !resolved(eo) || resolved(ey) {
		t.Fatalf("after the release the older job goes first: older %v, younger %v", resolved(eo), resolved(ey))
	}
}
