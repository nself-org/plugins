package logs

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// A line written while someone follows reaches them within a second, across
// rotations of the active file.
func TestFollowDeliversEachLineWithinASecondAcrossRotation(t *testing.T) {
	s, dir := newTestSink(t, Options{SegmentBytes: 400})
	var ended atomic.Bool
	type got struct {
		seq int64
		at  time.Time
	}
	ch := make(chan got, 1000)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	errc := make(chan error, 1)
	go func() {
		errc <- Follow(ctx, dir, 0, ended.Load, func(l Line) error { ch <- got{l.Seq, time.Now()}; return nil })
	}()
	sent := map[int64]time.Time{}
	for i := 1; i <= 60; i++ {
		sent[int64(i)] = time.Now()
		_, _ = s.Write([]byte("line number " + string(rune('A'+i%26)) + "\n"))
		time.Sleep(15 * time.Millisecond)
	}
	_ = s.Close()
	ended.Store(true)
	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	close(ch)
	var want int64 = 1
	for g := range ch {
		if g.seq != want {
			t.Fatalf("got seq %d, want %d (a line was lost or repeated)", g.seq, want)
		}
		if d := g.at.Sub(sent[g.seq]); d > time.Second {
			t.Fatalf("line %d took %v", g.seq, d)
		}
		want++
	}
	if want != 61 {
		t.Fatalf("followed %d lines of 60", want-1)
	}
	if segs, _ := segments(dir); len(segs) == 0 {
		t.Fatal("the test never rotated")
	}
}

func TestFollowWaitsForALateDirectoryAndStopsWhenDone(t *testing.T) {
	dir := t.TempDir() + "/late/att"
	var done atomic.Bool
	var n atomic.Int32
	errc := make(chan error, 1)
	go func() {
		errc <- Follow(context.Background(), dir, 0, done.Load, func(Line) error { n.Add(1); return nil })
	}()
	time.Sleep(250 * time.Millisecond)
	w, _ := Open(dir, Options{})
	_, _ = w.Append(mk(1, "hello"))
	_ = w.Close()
	time.Sleep(300 * time.Millisecond)
	done.Store(true)
	if err := <-errc; err != nil || n.Load() != 1 {
		t.Fatalf("lines %d, err %v", n.Load(), err)
	}
}

func TestReadOfAMissingAttemptIsE633(t *testing.T) {
	err := Read(t.TempDir()+"/nope", 0, func(Line) error { return nil })
	if e, ok := AsError(err); !ok || e.Code != CodeNone {
		t.Fatalf("want E633, got %v", err)
	}
}

// A writer that rotates constantly while a follower polls: no line is lost or
// repeated (the follower opens the active file before listing segments).
func TestFollowLosesNothingUnderConstantRotation(t *testing.T) {
	old := PollEvery
	PollEvery = time.Millisecond
	defer func() { PollEvery = old }()
	dir := t.TempDir() + "/att"
	w, err := Open(dir, Options{SegmentBytes: 250})
	if err != nil {
		t.Fatal(err)
	}
	var ended atomic.Bool
	var want int64 = 1
	errc := make(chan error, 1)
	go func() {
		errc <- Follow(context.Background(), dir, 0, ended.Load, func(l Line) error {
			if l.Seq != want {
				t.Errorf("got seq %d, want %d", l.Seq, want)
				return context.Canceled
			}
			want++
			return nil
		})
	}()
	const total = 400
	for i := int64(1); i <= total; i++ {
		if _, err := w.Append(mk(i, "rotating line of text")); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	ended.Store(true)
	if err := <-errc; err != nil && err != context.Canceled {
		t.Fatal(err)
	}
	if want != total+1 {
		t.Fatalf("the follower saw %d of %d lines", want-1, total)
	}
}

// The replacement active file can reuse the inode of the one it replaced and be
// longer than the follower's offset into the old one (seen on Linux): the
// follower must restart at the top of the new file, not seek into the middle of a line.
func TestFollowSurvivesAReplacedActiveFileWithTheSameInodeAndLargerSize(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, activeName)
	write := func(first, last int64) {
		var b []byte
		for i := first; i <= last; i++ {
			b = append(b, encodeLine(mk(i, "line"))...)
		}
		if err := os.WriteFile(active, b, 0o600); err != nil { // truncates in place: same inode
			t.Fatal(err)
		}
	}
	write(1, 3)
	var got []int64
	fl := follower{dir: dir, fn: func(l Line) error { got = append(got, l.Seq); return nil }}
	if err := fl.poll(); err != nil {
		t.Fatal(err)
	}
	// rotation: lines 1-3 become a segment, the active file now holds 4-12 (larger than the old offset)
	if err := gzipFile(active, filepath.Join(dir, segmentName(1, 3, mustSize(t, active)))); err != nil {
		t.Fatal(err)
	}
	write(4, 12)
	if err := fl.poll(); err != nil {
		t.Fatalf("poll after the replacement: %v", err)
	}
	if len(got) != 12 || got[11] != 12 {
		t.Fatalf("saw %v", got)
	}
}
