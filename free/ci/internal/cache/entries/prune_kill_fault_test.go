//go:build fault

package entries_test

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/blob/cas"
	"github.com/nself-org/plugins/free/ci/internal/cache/entries"
	"github.com/nself-org/plugins/free/ci/internal/store"
)

// killPhases are the markers the child prints inside Prune; "sweeping" is the
// moment the cas sweep starts and has no linger, so a kill there races the sweep.
var killPhases = []string{"before-prune-tx", "after-prune-tx", "before-refs", "sweeping"}

const (
	killIterations = 50
	killEntries    = 24
	killBlobs      = 8
)

// ahead is the clock of both processes: two hours on, so every blob is past cas's grace.
func ahead() time.Time { return time.Now().Add(2 * time.Hour) }

// phaseBackend prints a marker at each phase of Prune and lingers there, so the
// parent can kill the child at a known point inside the operation.
type phaseBackend struct {
	entries.Backend
	linger time.Duration
}

func (p phaseBackend) mark(name string) {
	fmt.Println("phase:" + name)
	time.Sleep(p.linger)
}

func (p phaseBackend) CachePrune(ctx context.Context, s entries.PruneStep) (entries.Pruned, error) {
	p.mark("before-prune-tx")
	r, err := p.Backend.CachePrune(ctx, s)
	p.mark("after-prune-tx")
	return r, err
}

func (p phaseBackend) CacheBlobRefs(ctx context.Context, each func([]byte, string) error) error {
	p.mark("before-refs")
	err := p.Backend.CacheBlobRefs(ctx, each)
	fmt.Println("phase:sweeping") // no linger: the cas sweep runs right after
	return err
}

func faultStack(dir string, env map[string]string, linger time.Duration) (*store.Store, *entries.Store, error) {
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{Clock: ahead})
	if err != nil {
		return nil, nil, err
	}
	blobs, err := cas.New(dir)
	if err != nil {
		return nil, nil, err
	}
	var be entries.Backend = st
	if linger > 0 {
		be = phaseBackend{Backend: st, linger: linger}
	}
	return st, entries.New(be, blobs, entries.Options{Clock: ahead, DisableFloor: true, Getenv: func(k string) string { return env[k] }}), nil
}

// TestPruneKillChild is the process the parent kills; it does nothing unless
// NSELF_FAULT_DIR is set.
func TestPruneKillChild(t *testing.T) {
	dir := os.Getenv("NSELF_FAULT_DIR")
	if dir == "" {
		return
	}
	st, es, err := faultStack(dir, map[string]string{"NSELF_CI_CACHE_MAX_BYTES": os.Getenv("NSELF_FAULT_MAX_BYTES")}, 40*time.Millisecond)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(3)
	}
	defer func() { _ = st.Close() }()
	fmt.Println("prune-start")
	if _, err = es.Prune(context.Background(), 0); err != nil {
		fmt.Println("error:", err)
		os.Exit(3)
	}
	os.Exit(0)
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o700)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		if _, err = io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatal(err)
	}
}

func cacheBlobFiles(dir string) (n int) {
	_ = filepath.WalkDir(filepath.Join(dir, "blobs"), func(p string, d os.DirEntry, err error) error {
		if err == nil && d.Type().IsRegular() && strings.Contains(p, string(filepath.Separator)+"sha256"+string(filepath.Separator)) {
			n++
		}
		return nil
	})
	return n
}

// TestPruneKill kills a prune with SIGKILL at 50 different moments and checks
// the store afterwards: every entry whole or absent, the next prune completes
// and no orphan cas blob remains.
func TestPruneKill(t *testing.T) {
	template := newEnv(t, envOpts{})
	var total int64
	for i := 0; i < killEntries; i++ {
		var parts []string
		for j := 0; j < killBlobs; j++ {
			parts = append(parts, fmt.Sprintf("entry %d blob %d %s", i, j, strings.Repeat("x", 100+i)))
		}
		total += template.save("k"+strconv.Itoa(i), trusted, "e"+strconv.Itoa(i), parts...).Bytes()
		template.advance(time.Second)
	}
	if err := template.st.Close(); err != nil {
		t.Fatal(err)
	}
	if got := cacheBlobFiles(template.home); got != killEntries*killBlobs {
		t.Fatalf("template has %d blob files", got)
	}
	killed, byPhase, tried := 0, map[string]int{}, map[string]int{}
	// every counted iteration is a confirmed SIGKILL of a child inside Prune; a missed
	// window (the child finished first) is still checked for consistency but retried
	for try := 0; killed < killIterations; try++ {
		if try >= killIterations*4 {
			t.Fatalf("only %d of %d kills landed in %d tries (%v)", killed, killIterations, try, byPhase)
		}
		i := killed
		dir := t.TempDir()
		copyTree(t, template.home, dir)
		cmd := exec.Command(os.Args[0], "-test.run=^TestPruneKillChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), "NSELF_FAULT_DIR="+dir, "NSELF_FAULT_MAX_BYTES="+strconv.FormatInt(total/2, 10))
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err = cmd.Start(); err != nil {
			t.Fatal(err)
		}
		// kill the child at a rotating phase of Prune, a few milliseconds after its marker
		phase := killPhases[i%len(killPhases)]
		tried[phase]++
		sc := bufio.NewScanner(out)
		for sc.Scan() && sc.Text() != "phase:"+phase {
		}
		step := 1500 * time.Microsecond // inside the 40 ms linger of the marked phases
		if phase == "sweeping" {
			step = 200 * time.Microsecond // the cas sweep has no linger: strike while it runs
		}
		time.Sleep(time.Duration(i/len(killPhases)) * step / time.Duration(1+try-killed)) // a missed window retries sooner
		_ = cmd.Process.Signal(syscall.SIGKILL)
		go func() { _, _ = io.Copy(io.Discard, out) }()
		var ee *exec.ExitError
		switch err = cmd.Wait(); {
		case err == nil:
		case errors.As(err, &ee) && ee.ExitCode() == -1:
			killed++
			byPhase[phase]++
		default:
			t.Fatalf("iteration %d: child failed: %v", i, err)
		}
		st, es, err := faultStack(dir, map[string]string{"NSELF_CI_CACHE_MAX_BYTES": strconv.FormatInt(total/2, 10)}, 0)
		if err != nil {
			t.Fatalf("iteration %d: reopen after kill: %v", i, err)
		}
		if rep, err := es.Verify(context.Background()); err != nil || rep.Dropped != 0 {
			t.Fatalf("iteration %d: entry left damaged by the kill: %+v %v", i, rep, err)
		}
		if _, err = es.Prune(context.Background(), 0); err != nil {
			t.Fatalf("iteration %d: next prune: %v", i, err)
		}
		list, err := st.CacheEntries(context.Background(), "")
		if err != nil {
			t.Fatal(err)
		}
		refs := 0
		for _, m := range list {
			refs += len(m.Blobs)
		}
		if files := cacheBlobFiles(dir); files != refs || refs == 0 || refs == killEntries*killBlobs {
			t.Fatalf("iteration %d: %d blob files for %d referenced blobs of %d entries", i, files, refs, len(list))
		}
		_ = st.Close()
	}
	// the three lingering phases are deterministic: every kill must land
	for _, ph := range killPhases[:3] {
		if byPhase[ph] != tried[ph] {
			t.Fatalf("phase %q: %d kills in %d tries (the child finished before the kill)", ph, byPhase[ph], tried[ph])
		}
	}
	t.Logf("%d confirmed SIGKILLs inside Prune by phase %v, recovery verified after each", killed, byPhase)
}
