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

const (
	killIterations = 50
	killEntries    = 24
	killBlobs      = 8
)

// ahead is the clock of both processes: two hours on, so every blob is past cas's grace.
func ahead() time.Time { return time.Now().Add(2 * time.Hour) }

func faultStack(dir string, env map[string]string) (*store.Store, *entries.Store, error) {
	st, err := store.Open(filepath.Join(dir, "state.db"), store.Options{Clock: ahead})
	if err != nil {
		return nil, nil, err
	}
	blobs, err := cas.New(dir)
	if err != nil {
		return nil, nil, err
	}
	return st, entries.New(st, blobs, entries.Options{Clock: ahead, Getenv: func(k string) string { return env[k] }}), nil
}

// TestPruneKillChild is the process the parent kills; it does nothing unless
// NSELF_FAULT_DIR is set.
func TestPruneKillChild(t *testing.T) {
	dir := os.Getenv("NSELF_FAULT_DIR")
	if dir == "" {
		return
	}
	st, es, err := faultStack(dir, map[string]string{"NSELF_CI_CACHE_MAX_BYTES": os.Getenv("NSELF_FAULT_MAX_BYTES")})
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
	killed := 0
	for i := 0; i < killIterations; i++ {
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
		sc := bufio.NewScanner(out)
		for sc.Scan() && sc.Text() != "prune-start" {
		}
		time.Sleep(time.Duration(i) * 200 * time.Microsecond)
		_ = cmd.Process.Signal(syscall.SIGKILL)
		go func() { _, _ = io.Copy(io.Discard, out) }()
		var ee *exec.ExitError
		switch err = cmd.Wait(); {
		case err == nil:
		case errors.As(err, &ee) && ee.ExitCode() == -1:
			killed++
		default:
			t.Fatalf("iteration %d: child failed: %v", i, err)
		}
		st, es, err := faultStack(dir, map[string]string{"NSELF_CI_CACHE_MAX_BYTES": strconv.FormatInt(total/2, 10)})
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
	if killed < 10 {
		t.Fatalf("only %d of %d children were killed mid-prune: the fault window was missed", killed, killIterations)
	}
	t.Logf("killed mid-prune in %d of %d iterations", killed, killIterations)
}
