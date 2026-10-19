package entries

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
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
	return New(fb, blobs, Options{Clock: func() time.Time { now = now.Add(time.Second); return now }, Getenv: func(string) string { return "" }, DisableFloor: true}), fb, &now
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

func TestSpaceTreatsAMissingStatfsAsNoCheck(t *testing.T) {
	s, _, _ := newFake(t)
	s.opt.Free = func() (int64, int64, error) { return 0, 0, errNoStatfs }
	if _, _, ok, err := s.space(); ok || err != nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	s.opt.Free = func() (int64, int64, error) { return 0, 0, errors.New("disk on fire") }
	if _, _, _, err := s.space(); err == nil {
		t.Fatal("a real probe failure must surface")
	}
}

type nopCloser struct{ io.Reader }

func (nopCloser) Close() error { return nil }

func TestIntactRetriesRacedOpensAndClassifiesTheRest(t *testing.T) {
	s, _, _ := newFake(t)
	realm := cas.CacheRealm(string(trustedNS.Canonical()))
	digest, size, err := s.blobs.Put(context.Background(), realm, strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{Namespace: trustedNS.Canonical(), Blobs: []Blob{{Path: "a", SHA256: digest, Size: size}}}
	gone := m
	gone.Blobs = []Blob{{Path: "a", SHA256: strings.Repeat("0", 64), Size: 1}}
	if got, err := s.intact(gone); got != "missing" || err != nil {
		t.Fatalf("missing blob: %q %v", got, err)
	}
	wrong := m
	wrong.Blobs = []Blob{{Path: "a", SHA256: digest, Size: size + 1}}
	if got, err := s.intact(wrong); got != "size_mismatch" || err != nil {
		t.Fatalf("size lie: %q %v", got, err)
	}
	for _, c := range []struct {
		name   string
		errs   []error // what successive Opens return; the last repeats
		reason string
		calls  int
		fail   bool
	}{
		{"healthy", []error{nil}, "", 1, false},
		{"raced once", []error{cas.ErrInvalid, nil}, "", 2, false},
		{"raced twice", []error{cas.ErrInvalid, cas.ErrInvalid, nil}, "", 3, false},
		{"raced three times", []error{cas.ErrInvalid}, "unstable", 3, false},
		{"digest mismatch", []error{cas.ErrDigestMismatch}, "corrupt", 1, false},
		{"size mismatch", []error{cas.ErrSizeMismatch}, "corrupt", 1, false},
		{"vanished", []error{os.ErrNotExist}, "missing", 1, false},
		{"disk error", []error{errors.New("i/o")}, "", 1, true},
	} {
		calls := 0
		s.open = func(cas.Realm, string) (io.ReadCloser, error) {
			e := c.errs[min(calls, len(c.errs)-1)]
			calls++
			if e != nil {
				return nil, e
			}
			return nopCloser{strings.NewReader("x")}, nil
		}
		got, err := s.intact(m)
		if got != c.reason || (err != nil) != c.fail || calls != c.calls {
			t.Errorf("%s: reason %q err %v after %d opens, want %q fail=%v after %d", c.name, got, err, calls, c.reason, c.fail, c.calls)
		}
	}
}
