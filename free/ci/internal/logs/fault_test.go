//go:build fault && !windows

package logs

// P7-CI-39 acceptance: kill -9 of the writer's process mid-stream, 100 times,
// then resume. The stored log must have strictly increasing seq starting at 1
// with no duplicate and no gap, hold every line the writer acknowledged, and
// after the producer replays from the start hold exactly lines 1..total.
// The writer runs in a child process (this test binary re-executed) that
// SIGKILLs itself at the n-th crash point of the writer (after a write, after
// an fsync, between the steps of a rotation), so every recovery path is hit.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

const (
	faultIterations = 100
	faultTotal      = 120
	childEnv        = "LOGS_FAULT_CHILD_DIR"
	killEnv         = "LOGS_FAULT_KILL_AT"
	tearEnv         = "LOGS_FAULT_TEAR"
)

func TestMain(m *testing.M) {
	if dir := os.Getenv(childEnv); dir != "" {
		os.Exit(faultChild(dir))
	}
	os.Exit(m.Run())
}

func faultText(seq int64) string {
	return fmt.Sprintf("line-%04d-%s", seq, strings.Repeat("p", int(seq%23)))
}

// faultChild writes seq 1..faultTotal (replaying from 1 each time, so lines
// already stored are dropped) and prints "ack <seq>" after each acknowledged
// line. SegmentBytes is tiny so rotations happen inside the kill window.
func faultChild(dir string) int {
	if n, _ := strconv.Atoi(os.Getenv(killEnv)); n > 0 {
		count := 0
		faultPoint = func(step string) {
			if count++; count == n {
				if step == "written" && os.Getenv(tearEnv) != "" { // a write the kernel only half did
					if fi, err := os.Stat(filepath.Join(dir, activeName)); err == nil && fi.Size() > 9 {
						_ = os.Truncate(filepath.Join(dir, activeName), fi.Size()-9)
					}
				}
				_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
				select {}
			}
		}
	}
	w, err := Open(dir, Options{SegmentBytes: 900})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		return 3
	}
	for seq := int64(1); seq <= faultTotal; seq++ {
		stored := w.LastSeq() >= seq
		ack, err := w.Append(mk(seq, faultText(seq)))
		if err != nil {
			fmt.Fprintln(os.Stderr, "append:", err)
			return 4
		}
		if !stored {
			fmt.Printf("ack %d\n", ack)
		}
	}
	_ = w.Close()
	fmt.Println("done")
	return 0
}

// runChild runs the writer child; killAt > 0 makes it SIGKILL itself at its
// killAt-th crash point (0 lets it finish). It returns the highest sequence
// the child acknowledged on its stdout.
func runChild(t *testing.T, dir string, killAt int, tear bool) int64 {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), childEnv+"="+dir, killEnv+"="+strconv.Itoa(killAt))
	if tear {
		cmd.Env = append(cmd.Env, tearEnv+"=1")
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var acked int64
	for sc := bufio.NewScanner(out); sc.Scan(); {
		if f := strings.Fields(sc.Text()); len(f) == 2 && f[0] == "ack" {
			acked, _ = strconv.ParseInt(f[1], 10, 64)
		}
	}
	_ = cmd.Wait()
	return acked
}

// rawLines parses every stored file in order, without the reader's de-duplication:
// the segments by name order, then the active file. It also checks that each
// segment's name states the sequence range and raw size of its content.
func rawLines(t *testing.T, dir string) []Line {
	t.Helper()
	var out []Line
	parse := func(b []byte) []Line {
		var ls []Line
		for _, raw := range bytes.SplitAfter(b, []byte("\n")) {
			if len(raw) == 0 {
				continue
			}
			if raw[len(raw)-1] != '\n' {
				continue // a torn tail: not a stored line
			}
			var l Line
			if err := json.Unmarshal(raw, &l); err != nil {
				t.Fatalf("unparsable stored line %q: %v", raw, err)
			}
			ls = append(ls, l)
		}
		return ls
	}
	segs, err := segments(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range segs {
		f, _ := os.Open(s.Path)
		zr, err := gzip.NewReader(f)
		if err != nil {
			t.Fatalf("segment %s: %v", s.Path, err)
		}
		b, _ := io.ReadAll(zr)
		_ = f.Close()
		ls := parse(b)
		if len(ls) == 0 || ls[0].Seq != s.First || ls[len(ls)-1].Seq != s.Last || int64(len(b)) != s.Bytes {
			t.Fatalf("segment %s does not match its name (holds %d lines, %d bytes)", filepath.Base(s.Path), len(ls), len(b))
		}
		out = append(out, ls...)
	}
	if b, err := os.ReadFile(filepath.Join(dir, activeName)); err == nil {
		out = append(out, parse(b)...)
	}
	return out
}

func checkStored(t *testing.T, dir string, acked int64, label string) int64 {
	t.Helper()
	var last int64
	err := Read(dir, 0, func(l Line) error {
		if l.Seq != last+1 {
			return fmt.Errorf("%s: seq %d follows %d", label, l.Seq, last)
		}
		if l.Text != faultText(l.Seq) {
			return fmt.Errorf("%s: line %d holds %q", label, l.Seq, l.Text)
		}
		last = l.Seq
		return nil
	})
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	// the reader above tolerates a crash leftover (an archived active file); resuming
	// the writer must repair it, so the files themselves are checked after Open
	w, err := Open(dir, Options{SegmentBytes: 900})
	if err != nil {
		t.Fatalf("%s: reopen: %v", label, err)
	}
	_ = w.Close()
	prev := int64(0)
	for _, l := range rawLines(t, dir) {
		if l.Seq != prev+1 {
			t.Fatalf("%s: the files hold seq %d after %d (a duplicate or a gap on disk)", label, l.Seq, prev)
		}
		prev = l.Seq
	}
	if prev != last {
		t.Fatalf("%s: files end at %d, the reader at %d", label, prev, last)
	}
	if last < acked {
		t.Fatalf("%s: acknowledged seq %d but the log ends at %d", label, acked, last)
	}
	return last
}

func TestLogResume(t *testing.T) {
	rng := rand.New(rand.NewSource(20261018))
	root := t.TempDir()
	for i := 0; i < faultIterations; i++ {
		dir := fmt.Sprintf("%s/it%03d", root, i)
		// about 3 crash points per line, plus rotations: 1..250 lands anywhere in the first lines
		a1 := runChild(t, dir, 1+rng.Intn(250), i%2 == 0)
		checkStored(t, dir, a1, fmt.Sprintf("it%d first kill", i))
		a2 := runChild(t, dir, 1+rng.Intn(150), i%3 == 0)
		checkStored(t, dir, max(a1, a2), fmt.Sprintf("it%d second kill", i))
		runChild(t, dir, 0, false) // the producer replays from the start and finishes
		if last := checkStored(t, dir, faultTotal, fmt.Sprintf("it%d resumed", i)); last != faultTotal {
			t.Fatalf("it%d: resumed log ends at %d, want %d", i, last, faultTotal)
		}
	}
}
