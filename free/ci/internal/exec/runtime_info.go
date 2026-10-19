package exec

// Purpose: one `info` question per runtime per window (D-1008). The orphan
// sweep, hygiene and the isolation probe all ask whether the daemon answers; each
// `docker info` also runs the compose and buildx plugin metadata probes, about
// 60 ms of wall on the job's start path, three times per run.
// Inputs:  the runtime binary and the environment the CLI runs under.
// Outputs: the JSON `info` output (stdout), shared by every caller of the window.
// Constraints: a plain `info` and `info --format {{json .}}` both get the JSON
// answer (no caller reads the plain text); concurrent callers share one spawn;
// a cancelled question is never cached; any other verb is not cached.

import (
	"context"
	"os"
	"sync"
	"time"
)

// runtimeInfoTTL is how long an answer stands: a run's start path asks within a
// second, and hygiene keeps its own 15 s view of the daemon.
const runtimeInfoTTL = 15 * time.Second

type infoAnswer struct {
	done chan struct{}
	out  []byte
	err  error
	at   time.Time // zero while the question is in flight
}

var (
	runtimeInfoMu sync.Mutex
	runtimeInfo   = map[string]*infoAnswer{}
)

// ResetRuntimeInfoCache forgets every cached `info` answer (tests, and a caller
// that just started or stopped a daemon).
func ResetRuntimeInfoCache() {
	runtimeInfoMu.Lock()
	runtimeInfo = map[string]*infoAnswer{}
	runtimeInfoMu.Unlock()
}

// isInfoQuery reports whether args ask for the runtime's info in one of the two
// shapes this package serves from the shared answer.
func isInfoQuery(args []string) bool {
	switch len(args) {
	case 1:
		return args[0] == "info"
	case 3:
		return args[0] == "info" && args[1] == "--format" && args[2] == "{{json .}}"
	}
	return false
}

// cachedInfo returns the shared `info` answer of binary, asking at most once per
// window. The key holds the environment the CLI is started with.
func cachedInfo(ctx context.Context, binary string) ([]byte, error) {
	key := binary
	for _, k := range []string{"PATH", "HOME", "XDG_RUNTIME_DIR"} {
		key += "\x00" + os.Getenv(k)
	}
	for {
		runtimeInfoMu.Lock()
		a, ok := runtimeInfo[key]
		if ok && !a.at.IsZero() && time.Since(a.at) >= runtimeInfoTTL {
			ok = false
		}
		if !ok {
			a = &infoAnswer{done: make(chan struct{})}
			runtimeInfo[key] = a
			runtimeInfoMu.Unlock()
			a.out, a.err = runtimeQuery(ctx, binary, "info", "--format", "{{json .}}")
			runtimeInfoMu.Lock()
			if a.err != nil && ctx.Err() != nil { // the claimant gave up: nobody inherits its cancellation
				if runtimeInfo[key] == a {
					delete(runtimeInfo, key)
				}
			} else {
				a.at = time.Now()
			}
			runtimeInfoMu.Unlock()
			close(a.done)
			return a.out, a.err
		}
		runtimeInfoMu.Unlock()
		select {
		case <-a.done:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		runtimeInfoMu.Lock()
		kept := runtimeInfo[key] == a
		runtimeInfoMu.Unlock()
		if kept || a.err == nil {
			return a.out, a.err
		} // the claimant was cancelled and dropped its answer: ask again
	}
}

// WarmRuntimeInfo starts the `info` question in the background, so a run that
// asks the daemon later (the orphan sweep on its first job) finds the answer
// already in flight instead of waiting a whole spawn on its start path.
func WarmRuntimeInfo() {
	go func() { _, _ = cachedInfo(context.Background(), "docker") }()
}
