package entries

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
)

// fakeBackend scripts CachePrune results so the batching loop is testable alone.
type fakeBackend struct {
	Backend
	script []Pruned
	steps  []PruneStep
	err    error
}

func (f *fakeBackend) CachePrune(_ context.Context, p PruneStep) (Pruned, error) {
	f.steps = append(f.steps, p)
	if f.err != nil {
		return Pruned{}, f.err
	}
	r := f.script[0]
	f.script = f.script[1:]
	return r, nil
}
func (f *fakeBackend) CacheEntries(context.Context, string) ([]Manifest, error) { return nil, nil }

func newFake(t *testing.T, script ...Pruned) (*Store, *fakeBackend, *time.Time) {
	t.Helper()
	now := time.Unix(1000, 0)
	fb := &fakeBackend{script: script}
	blobs, err := cas.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return New(fb, blobs, Options{Clock: func() time.Time { now = now.Add(time.Second); return now }, Getenv: func(string) string { return "" }}), fb, &now
}

func TestPruneLoopAccumulatesBatches(t *testing.T) {
	s, fb, _ := newFake(t, Pruned{Entries: 64, Bytes: 640, More: true}, Pruned{Entries: 64, Bytes: 640, More: true}, Pruned{Entries: 3, Bytes: 30})
	res, err := s.Prune(context.Background(), 0)
	if err != nil || res.Entries != 131 || res.Bytes != 1310 || res.More {
		t.Fatalf("%+v %v", res, err)
	}
	if len(fb.steps) != 3 || fb.steps[0].Limit != pruneBatch || fb.steps[0].MaxBytes != defaultMaxBytes || fb.steps[0].MaxAge != defaultMaxAge || fb.steps[0].Retention != UsageRetention || fb.steps[0].FreeBytes != 0 {
		t.Fatalf("steps %+v", fb.steps)
	}
}

func TestPruneStopsAtBudgetAndReportsMore(t *testing.T) {
	s, fb, _ := newFake(t, Pruned{Entries: 64, Bytes: 640, More: true}, Pruned{Entries: 64, Bytes: 640, More: true}, Pruned{Entries: 1})
	res, err := s.Prune(context.Background(), 2*time.Second) // the fake clock ticks one second per read
	if err != nil || !res.More || res.Entries != 128 || len(fb.steps) != 2 {
		t.Fatalf("%+v %v steps=%d", res, err, len(fb.steps))
	}
}

func TestPruneHonoursContextAndBackendErrors(t *testing.T) {
	s, _, _ := newFake(t, Pruned{Entries: 1, More: true}, Pruned{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.Prune(ctx, 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context: %v", err)
	}
	boom := errors.New("boom")
	s, fb, _ := newFake(t)
	fb.err = boom
	if _, err := s.Prune(context.Background(), 0); !errors.Is(err, boom) {
		t.Fatalf("backend error: %v", err)
	}
}

func TestPruneSkipsTheFloorStageWhenFreeEqualsTheFloor(t *testing.T) {
	s, fb, _ := newFake(t, Pruned{Entries: 2, Bytes: 20})
	s.minFree = 500
	s.opt.Free = func() (int64, int64, error) { return 500, 1 << 40, nil }
	res, err := s.Prune(context.Background(), 0)
	if err != nil || res.Entries != 2 || len(fb.steps) != 1 {
		t.Fatalf("%+v %v steps=%d", res, err, len(fb.steps))
	}
}

func TestPruneFloorStageReleasesTheShortfall(t *testing.T) {
	s, fb, _ := newFake(t, Pruned{}, Pruned{Entries: 3, Bytes: 31})
	s.minFree = 500
	free := int64(470)
	s.opt.Free = func() (int64, int64, error) { f := free; free = 500; return f, 1 << 40, nil }
	res, err := s.Prune(context.Background(), 0)
	if err != nil || res.Entries != 3 || res.Bytes != 31 || len(fb.steps) != 2 {
		t.Fatalf("%+v %v steps=%d", res, err, len(fb.steps))
	}
	if st := fb.steps[1]; st.FreeBytes != 30 || st.Limit != 0 {
		t.Fatalf("floor step %+v", st)
	}
}
