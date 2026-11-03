package exec

// Purpose: read the kernel's own count of forks refused at a container's process limit (D-1580).
// Inputs:  the runtime binary and the container name of a planned attempt.
// Outputs: pidsReading, the highest `max` counter of the container cgroup's pids.events, the `max` and
// `oom_kill` counters of its memory.events, and what was read. Every planned container is watched: these
// counters are the host's evidence of a limit (resourceReason), the job's output never is.
// Constraints: the counter is sampled while the container lives and once more before explicit removal;
// the last good reading is kept. pids.events `max` rises each time
// a host fork or clone is refused at pids.max, so it does not depend on the runtime's log. Under gVisor a
// guest fork the Sentry refuses by itself never reaches the host limit and leaves the counter at 0 (D-1580),
// so a miss is no evidence either way. Only a host that exposes the cgroup (cgroup v2 under /sys/fs/cgroup) answers; elsewhere
// nothing is claimed and the reading says why.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// cgroupRoot is the cgroup v2 mount; procRoot holds /proc. Variables so tests can point them at a fixture.
var (
	cgroupRoot = "/sys/fs/cgroup"
	procRoot   = "/proc"
)

// pidsPoll is the sampling period: the window between the first refused fork and the container's removal is short.
const pidsPoll = 3 * time.Millisecond

// pidsReading is what the sampler learned about one container.
type pidsReading struct {
	Max       uint64 // highest `max` counter read from pids.events
	MemMax    uint64 // highest `max` counter read from memory.events (usage reached memory.max)
	OOMKill   uint64 // highest `oom_kill` counter read from memory.events
	Path      string // the cgroup directory that was read ("" when none was found)
	Limit     string // that cgroup's pids.max
	Reads     int    // successful reads of pids.events
	MemReads  int    // successful reads of memory.events
	PidsError string // latest pids.events read error, retained after later success
	MemError  string // latest memory.events read error, retained after later success
	Problem   string // why no cgroup was read, when none was
}

// Hit reports whether the kernel refused at least one fork at the container's limit.
func (r pidsReading) Hit() bool { return r.Max > 0 }

// String is the line printed for a silent failure, so a miss explains itself.
func (r pidsReading) String() string {
	if r.Path == "" {
		return "pids.events: no container cgroup read (" + r.Problem + ")"
	}
	return fmt.Sprintf("pids.events: max=%d after %d reads of %s (pids.max=%s, error=%q), memory.events max=%d oom_kill=%d after %d reads (error=%q)", r.Max, r.Reads, r.Path, r.Limit, r.PidsError, r.MemMax, r.OOMKill, r.MemReads, r.MemError)
}

// pidsWatch samples pids.events of one container until stopped.
type pidsWatch struct {
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mu       sync.Mutex
	stopOnce sync.Once
	r        pidsReading
	binary   string
	name     string
}

// startPidsWatch begins sampling the cgroup of container name.
func startPidsWatch(binary, name string) *pidsWatch {
	ctx, cancel := context.WithCancel(context.WithoutCancel(context.Background()))
	w := &pidsWatch{cancel: cancel, binary: binary, name: name, r: pidsReading{Problem: "container not seen"}}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(ctx)
	}()
	return w
}

// stop ends the sampling and returns what was read. A nil watch reads nothing.
func (w *pidsWatch) stop() pidsReading {
	if w == nil {
		return pidsReading{Problem: "not watched"}
	}
	w.stopOnce.Do(func() {
		w.cancel()
		w.wg.Wait()
		w.sample(context.Background()) // retain final host counters before the caller removes the container
	})
	return w.snapshot()
}

// snapshot acknowledges completed samples without racing the watcher.
func (w *pidsWatch) snapshot() pidsReading {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.r
}

func (w *pidsWatch) run(ctx context.Context) {
	tick := time.NewTicker(pidsPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		w.sample(ctx)
	}
}

// sample reads each host counter independently; a failed read never becomes a zero sample.
func (w *pidsWatch) sample(ctx context.Context) {
	dir := w.snapshot().Path
	if dir == "" {
		d, problem := findContainerCgroup(ctx, w.binary, w.name)
		w.mu.Lock()
		w.r.Problem = problem
		if d != "" {
			w.r.Path, w.r.Limit = d, readTrim(filepath.Join(d, "pids.max"))
		}
		w.mu.Unlock()
		dir = d
	}
	if dir == "" {
		return
	}
	raw, pidsErr := os.ReadFile(filepath.Join(dir, "pids.events"))
	mem, memErr := os.ReadFile(filepath.Join(dir, "memory.events"))
	w.mu.Lock()
	defer w.mu.Unlock()
	if pidsErr != nil {
		w.r.PidsError = pidsErr.Error()
	} else {
		w.r.Reads++
		w.r.Max = max(w.r.Max, parsePidsMax(string(raw)))
	}
	if memErr != nil {
		w.r.MemError = memErr.Error()
	} else {
		w.r.MemReads++
		w.r.MemMax = max(w.r.MemMax, parseCounter(string(mem), "max"))
		w.r.OOMKill = max(w.r.OOMKill, parseCounter(string(mem), "oom_kill"))
	}
}

// parsePidsMax returns the `max` counter of a pids.events body ("max 12\n").
func parsePidsMax(body string) uint64 { return parseCounter(body, "max") }

// parseCounter returns the counter key of a cgroup *.events body ("key N" lines), 0 when absent.
func parseCounter(body, key string) uint64 {
	for _, line := range strings.Split(body, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == key {
			n, err := strconv.ParseUint(f[1], 10, 64)
			if err == nil {
				return n
			}
		}
	}
	return 0
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(b))
}

// findContainerCgroup resolves a running or exited container's cgroup directory, or says why not.
func findContainerCgroup(ctx context.Context, binary, name string) (string, string) {
	out, err := runtimeQuery(ctx, binary, "inspect", "--format", "{{.Id}} {{.State.Pid}}", name)
	if err != nil {
		return "", "container not inspectable yet"
	}
	f := strings.Fields(string(out))
	if len(f) != 2 {
		return "", "container not running yet"
	}
	return pickCgroup(cgroupRoot, procRoot, f[0], f[1])
}

// pickCgroup chooses the container's cgroup: the one its init process lives in, then the usual docker
// layouts. The right one has a numeric pids.max (the container limit); a parent slice shows `max`.
func pickCgroup(root, proc, id, pid string) (string, string) {
	var cands []string
	if b, err := os.ReadFile(filepath.Join(proc, pid, "cgroup")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if rest, ok := strings.CutPrefix(line, "0::"); ok && strings.HasPrefix(rest, "/") {
				cands = append(cands, filepath.Join(root, rest))
			}
		}
	}
	cands = append(cands, filepath.Join(root, "system.slice", "docker-"+id+".scope"), filepath.Join(root, "docker", id))
	for _, c := range cands {
		if _, err := strconv.ParseUint(readTrim(filepath.Join(c, "pids.max")), 10, 64); err == nil {
			return c, ""
		}
	}
	return "", "no cgroup v2 directory with a numeric pids.max among " + strings.Join(cands, ", ")
}
