//go:build integration

package gvisor

// Purpose: the host's own readings of a running fork bomb (D-1580): its container cgroup's pids.max and
// peak pids.current, and whether the host could start processes while the bomb ran.
// Inputs:  the container name of an attempt.
// Outputs: hostReadings, taken until stop.
// Constraints: every reading comes from the host (docker inspect, /proc, /sys/fs/cgroup, fork+exec of
// /bin/true), never from the job's output, so the job cannot shape it.

import (
	"context"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type hostReadings struct {
	cgroup        string // the container's cgroup directory, once found
	limit         string // its pids.max
	peak          int    // highest pids.current read
	reads         int    // successful pids.current reads
	spawnedDuring int    // /bin/true runs that finished while the cgroup existed
	spawnErr      string // the first failed host spawn
}

func (r hostReadings) String() string {
	return fmt.Sprintf("cgroup %q pids.max=%s peak pids.current=%d over %d reads, %d host spawns during the bomb, spawn error %q",
		r.cgroup, r.limit, r.peak, r.reads, r.spawnedDuring, r.spawnErr)
}

type hostWatch struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	r      hostReadings
}

func watchHost(name string) *hostWatch {
	ctx, cancel := context.WithCancel(context.Background())
	w := &hostWatch{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		w.run(ctx, name)
	}()
	return w
}

func (w *hostWatch) stop() hostReadings {
	w.cancel()
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.r
}

func (w *hostWatch) run(ctx context.Context, name string) {
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	var dir string
	lastSpawn := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if dir == "" {
			dir = containerCgroup(ctx, name)
			if dir == "" {
				continue
			}
			w.mu.Lock()
			w.r.cgroup, w.r.limit = dir, readField(filepath.Join(dir, "pids.max"))
			w.mu.Unlock()
		}
		cur, err := strconv.Atoi(readField(filepath.Join(dir, "pids.current")))
		if err != nil {
			continue // the cgroup goes with the container: keep what was read
		}
		w.mu.Lock()
		w.r.reads++
		if cur > w.r.peak {
			w.r.peak = cur
		}
		w.mu.Unlock()
		if time.Since(lastSpawn) < 100*time.Millisecond {
			continue
		}
		lastSpawn = time.Now()
		spawnErr := osexec.Command("/bin/true").Run()
		_, alive := os.Stat(filepath.Join(dir, "pids.current"))
		w.mu.Lock()
		if spawnErr != nil && w.r.spawnErr == "" {
			w.r.spawnErr = spawnErr.Error()
		}
		if spawnErr == nil && alive == nil { // the bomb's cgroup still existed when the spawn finished
			w.r.spawnedDuring++
		}
		w.mu.Unlock()
	}
}

// containerCgroup finds the running container's cgroup v2 directory: the one its init process lives in,
// else docker's systemd scope. Only a directory with a numeric pids.max (the container's own limit) counts.
func containerCgroup(ctx context.Context, name string) string {
	out, err := osexec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Pid}}", name).Output()
	if err != nil {
		return ""
	}
	f := strings.Fields(string(out))
	if len(f) != 2 || f[1] == "0" {
		return ""
	}
	var cands []string
	if b, err := os.ReadFile(filepath.Join("/proc", f[1], "cgroup")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if rel, ok := strings.CutPrefix(line, "0::"); ok {
				cands = append(cands, filepath.Join("/sys/fs/cgroup", rel))
			}
		}
	}
	cands = append(cands, filepath.Join("/sys/fs/cgroup/system.slice", "docker-"+f[0]+".scope"), filepath.Join("/sys/fs/cgroup/docker", f[0]))
	for _, d := range cands {
		if _, err := strconv.Atoi(readField(filepath.Join(d, "pids.max"))); err == nil {
			return d
		}
	}
	return ""
}

func readField(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
