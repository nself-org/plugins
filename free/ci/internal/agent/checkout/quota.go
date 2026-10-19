package checkout

// Purpose: bound what an unpacked bundle may cost the node (D-1052).
// Inputs:  the bundle file, the staging source directory and the receiver's limits.
// Outputs: a refusal (ErrQuota) before git runs, or a cancelled unpack.
// Constraints: the packed size is capped by MaxBundle and the sha256, but a compressible
// bundle inflates far beyond it. Three layers hold the line: the pack header's object count
// is read before any git process starts; every git process runs under RLIMIT_FSIZE (one
// inflated file cannot pass the quota); and a watcher measures the staging directory while
// git runs and cancels it the moment the total passes the quota.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

// Default limits of a Receiver; a field set on the Receiver replaces them.
const (
	DefaultMaxStageBytes int64 = 16 << 30
	DefaultMaxObjects    int64 = 10_000_000
)

// ErrQuota is a bundle that would cost the node more than its staging limits allow.
var ErrQuota = errors.New("checkout: bundle exceeds the node's staging quota")

// quotaPoll is how often the watcher measures the staging directory.
const quotaPoll = 25 * time.Millisecond

func (r *Receiver) limits() (bytes, objects int64) {
	bytes, objects = r.MaxStageBytes, r.MaxObjects
	if bytes <= 0 {
		bytes = DefaultMaxStageBytes
	}
	if objects <= 0 {
		objects = DefaultMaxObjects
	}
	return bytes, objects
}

// packObjects reads the object count from the pack header that follows the bundle's
// header lines (v2 and v3 both end them with an empty line).
func packObjects(path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	br := bufio.NewReader(io.LimitReader(f, 1<<20))
	for first := true; ; first = false {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return 0, fmt.Errorf("checkout: bundle header is malformed")
		}
		if first && !bytes.HasPrefix(line, []byte("# v")) {
			return 0, fmt.Errorf("checkout: not a git bundle")
		}
		if len(line) == 1 {
			break
		}
	}
	var hdr [12]byte
	if _, err := io.ReadFull(br, hdr[:]); err != nil || string(hdr[:4]) != "PACK" {
		return 0, fmt.Errorf("checkout: bundle has no pack header")
	}
	return int64(binary.BigEndian.Uint32(hdr[8:])), nil
}

// dirBytes is the apparent size of every file under dir (0 when it does not exist).
func dirBytes(dir string) int64 {
	var n int64
	_ = filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			n += info.Size()
		}
		return nil
	})
	return n
}

// watchQuota cancels the unpack when dir grows past limit. stop ends the watcher and
// reports whether the quota was passed at any point, the final state included.
func watchQuota(dir string, limit int64, cancel context.CancelFunc) (stop func() bool) {
	done, finished := make(chan struct{}), make(chan bool, 1)
	go func() {
		tick := time.NewTicker(quotaPoll)
		defer tick.Stop()
		for {
			select {
			case <-done:
				finished <- dirBytes(dir) > limit
				return
			case <-tick.C:
				if dirBytes(dir) > limit {
					cancel()
					finished <- true
					return
				}
			}
		}
	}()
	return func() bool {
		close(done)
		return <-finished
	}
}

// withFileLimit runs every argv under `ulimit -f` (RLIMIT_FSIZE) so that no single file git
// writes can pass limit. It trails the limit by two blocks, so a file stopped by the limit is
// always over the quota and reads as one. Windows has no sh and keeps the watcher alone.
func withFileLimit(run Runner, limit int64) Runner {
	if runtime.GOOS == "windows" {
		return run
	}
	blocks := strconv.FormatInt(limit/512+2, 10) // POSIX ulimit -f counts 512-byte blocks
	return func(ctx context.Context, argv ...string) (string, int, error) {
		wrapped := append([]string{"sh", "-c", `ulimit -f "$1" || exit 125; shift; exec "$@"`, "ci-git", blocks}, argv...)
		return run(ctx, wrapped...)
	}
}

// killedBySignal is a git process the file-size limit stopped (SIGXFSZ, exit -1 from the
// executor) while the caller's own context is still live; git deletes the partial file it was
// writing, so the directory no longer shows the overrun.
func killedBySignal(err error, parent context.Context) bool {
	var ge *GitError
	return parent.Err() == nil && errors.As(err, &ge) && ge.Exit == -1
}
