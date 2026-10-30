package exec

// Purpose: read the kernel's own count of forks refused at a container's process limit (D-1580).
// Inputs:  the runtime binary and the container name of a planned attempt.
// Outputs: pidsReading, the highest `max` counter of the container cgroup's pids.events and what was read.
// Constraints: planned containers run with --rm, so the cgroup is gone when the job ends; the counter is
// sampled while the container lives and the last good reading is kept. pids.events `max` rises each time
// a fork or clone is refused at pids.max, whatever runtime ran the job, so it does not depend on the
// runtime's log. Only a host that exposes the cgroup (cgroup v2 under /sys/fs/cgroup) answers; elsewhere
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
	Max     uint64 // highest `max` counter read from pids.events
	Path    string // the cgroup directory that was read ("" when none was found)
	Limit   string // that cgroup's pids.max
	Reads   int    // successful reads of pids.events
	Problem string // why no cgroup was read, when none was
}

// Hit reports whether the kernel refused at least one fork at the container's limit.
func (r pidsReading) Hit() bool { return r.Max > 0 }

// String is the line printed for a silent failure, so a miss explains itself.
func (r pidsReading) String() string {
	if r.Path == "" {
		return "pids.events: no container cgroup read (" + r.Problem + ")"
	}
	return fmt.Sprintf("pids.events: max=%d after %d reads of %s (pids.max=%s)", r.Max, r.Reads, r.Path, r.Limit)
}

// pidsWatch samples pids.events of one container until stopped.
type pidsWatch struct {
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex
	r      pidsReading
}

// startPidsWatch begins sampling the cgroup of container name.
func startPidsWatch(binary, name string) *pidsWatch {
	ctx, cancel := context.WithCancel(context.WithoutCancel(context.Background()))
	w := &pidsWatch{cancel: cancel, r: pidsReading{Problem: "container not seen"}}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.run(ctx, binary, name)
	}()
	return w
}

// stop ends the sampling and returns what was read. A nil watch reads nothing.
func (w *pidsWatch) stop() pidsReading {
	if w == nil {
		return pidsReading{Problem: "not watched"}
	}
	w.cancel()
	w.wg.Wait()
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.r
}

func (w *pidsWatch) run(ctx context.Context, binary, name string) {
	var dir string
	tick := time.NewTicker(pidsPoll)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if dir == "" {
			d, problem := findContainerCgroup(ctx, binary, name)
			w.mu.Lock()
			w.r.Problem = problem
			w.mu.Unlock()
			if d == "" {
				continue
			}
			dir = d
			w.mu.Lock()
			w.r.Path, w.r.Limit, w.r.Problem = dir, readTrim(filepath.Join(dir, "pids.max")), ""
			w.mu.Unlock()
		}
		raw, err := os.ReadFile(filepath.Join(dir, "pids.events"))
		if err != nil {
			continue // the cgroup is removed with the container: keep the last good reading
		}
		n := parsePidsMax(string(raw))
		w.mu.Lock()
		w.r.Reads++
		if n > w.r.Max {
			w.r.Max = n
		}
		w.mu.Unlock()
	}
}

// parsePidsMax returns the `max` counter of a pids.events body ("max 12\n").
func parsePidsMax(body string) uint64 {
	for _, line := range strings.Split(body, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == "max" {
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

// findContainerCgroup resolves the cgroup directory of a running container, or says why not.
func findContainerCgroup(ctx context.Context, binary, name string) (string, string) {
	out, err := runtimeQuery(ctx, binary, "inspect", "--format", "{{.Id}} {{.State.Pid}}", name)
	if err != nil {
		return "", "container not inspectable yet"
	}
	f := strings.Fields(string(out))
	if len(f) != 2 || f[1] == "0" {
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
		if _, err := os.Stat(filepath.Join(c, "pids.events")); err != nil {
			continue
		}
		if lim := readTrim(filepath.Join(c, "pids.max")); lim != "max" && lim != "?" {
			return c, ""
		}
	}
	return "", "no cgroup v2 pids.events with a numeric pids.max among " + strings.Join(cands, ", ")
}
