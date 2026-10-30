package run

// Purpose: run each cache tool probe (`go version` and the like) once per run.
// P7-CI-46 measured the probe at 40 to 70 ms on macOS (it runs through the
// exec funnel in a copy of the tree), paid again by every job of the run.
// Inputs:  the run id, the job (workdir, env), the project root and the argv.
// Outputs: the probe's output, shared by every job that asks the same question.
// Constraints: the key holds everything the answer depends on (run, root,
// workdir, the job's env, argv), so two jobs with a different toolchain pin
// still probe separately; a failed probe is not remembered; entries older than
// probeMemoTTL are dropped, so a long-lived coordinator does not grow.

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

const probeMemoTTL = 10 * time.Minute

type probeResult struct {
	done chan struct{}
	out  string
	err  error
	at   time.Time
}

var probeMemo struct {
	mu sync.Mutex
	m  map[string]*probeResult
}

func probeKey(runID, root string, job model.Job, argv []string) string {
	env := make([]string, 0, len(job.Env))
	for k, v := range job.Env {
		env = append(env, k+"="+v)
	}
	sort.Strings(env)
	return strings.Join([]string{runID, root, job.Workdir, strings.Join(env, "\x01"), strings.Join(argv, "\x01")}, "\x00")
}

// probeOnce answers argv for the job from the run's memo, probing on a miss.
// Concurrent callers with the same key wait for the one probe in flight.
func (h *cacheHook) probeOnce(ctx context.Context, runID string, job model.Job, root string, argv []string) (string, error) {
	key := probeKey(runID, root, job, argv)
	now := time.Now()
	probeMemo.mu.Lock()
	if probeMemo.m == nil {
		probeMemo.m = map[string]*probeResult{}
	}
	if r, ok := probeMemo.m[key]; ok {
		probeMemo.mu.Unlock()
		select {
		case <-r.done:
			if r.err == nil {
				return r.out, nil
			}
		case <-ctx.Done():
			return "", ctx.Err()
		}
		return h.probe(ctx, job, root, argv) // the shared probe failed: ask again
	}
	for k, r := range probeMemo.m {
		if now.Sub(r.at) > probeMemoTTL {
			delete(probeMemo.m, k)
		}
	}
	r := &probeResult{done: make(chan struct{}), at: now}
	probeMemo.m[key] = r
	probeMemo.mu.Unlock()
	r.out, r.err = h.probe(ctx, job, root, argv)
	if r.err != nil {
		probeMemo.mu.Lock()
		if probeMemo.m[key] == r {
			delete(probeMemo.m, key)
		}
		probeMemo.mu.Unlock()
	}
	close(r.done)
	return r.out, r.err
}
