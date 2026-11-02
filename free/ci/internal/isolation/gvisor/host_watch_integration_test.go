//go:build integration

package gvisor

// Purpose: the host's own readings of a running fork bomb (D-1580): its container cgroup's pids.max, the
// cgroup's process count while the job waits and while the bomb runs, and host spawns made under that pressure.
// Inputs:  the container name of an attempt whose script waits for gateFile before it starts the bomb.
// Outputs: hostReadings, taken until stop.
// Constraints: every reading comes from the host (docker inspect, /proc, /sys/fs/cgroup, fork+exec of
// /bin/true), never from the job's output, so the job cannot shape it. The bomb starts only when the host
// opens the gate, after it found the cgroup and read the idle count, so discovery never races --rm and
// setup is never mistaken for the bomb. Every probe is bounded by the watch's context and probeTimeout.

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

const (
	// gateFile is created inside the container by the host to release the bomb.
	gateFile = "/tmp/nself-ci-release"
	// pressureMargin is how many processes above the idle count make a reading "under fork pressure".
	pressureMargin = 16
	// probeTimeout bounds every single host probe (docker inspect, docker exec, /bin/true).
	probeTimeout = 5 * time.Second
)

type hostReadings struct {
	cgroup     string // the container's cgroup directory, once found
	limit      string // its pids.max
	idle       int    // pids.current while the job waited at the gate
	released   bool   // the host opened the gate
	peak       int    // highest pids.current read after the gate opened
	reads      int    // successful pids.current reads after the gate opened
	spawns     int    // host /bin/true runs after the gate opened
	overlapped int    // host spawns bracketed by two readings of at least idle+pressureMargin
	spawnErr   string // the first failed host spawn
	problem    string // why the watch stopped short
}

func (r hostReadings) String() string {
	return fmt.Sprintf("cgroup %q pids.max=%s idle=%d released=%v peak=%d over %d reads, %d host spawns, %d under fork pressure, spawn error %q, problem %q",
		r.cgroup, r.limit, r.idle, r.released, r.peak, r.reads, r.spawns, r.overlapped, r.spawnErr, r.problem)
}

type hostWatch struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	r      hostReadings
}

// watchHost watches container name until stop or until parent ends.
func watchHost(parent context.Context, name string) *hostWatch {
	ctx, cancel := context.WithCancel(parent)
	w := &hostWatch{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(w.done)
		w.run(ctx, name)
	}()
	return w
}

// stop ends the watch. It returns within probeTimeout plus the probe's WaitDelay: every probe honours the context.
func (w *hostWatch) stop() hostReadings {
	w.cancel()
	<-w.done
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.r
}

func (w *hostWatch) set(f func(*hostReadings)) {
	w.mu.Lock()
	f(&w.r)
	w.mu.Unlock()
}

func (w *hostWatch) run(ctx context.Context, name string) {
	dir := ""
	for dir == "" { // the job waits at the gate, so the container stays up until it is found
		if dir = containerCgroup(ctx, name); dir != "" {
			break
		}
		select {
		case <-ctx.Done():
			w.set(func(r *hostReadings) { r.problem = "container cgroup never found" })
			return
		case <-time.After(20 * time.Millisecond):
		}
	}
	idle, err := pidsCurrent(dir)
	if err != nil {
		w.set(func(r *hostReadings) { r.cgroup, r.problem = dir, "idle pids.current: "+err.Error() })
		return
	}
	w.set(func(r *hostReadings) {
		r.cgroup, r.limit, r.idle = dir, readField(filepath.Join(dir, "pids.max")), idle
	})
	if err := probe(ctx, "docker", "exec", name, "touch", gateFile); err != nil {
		w.set(func(r *hostReadings) { r.problem = "open the gate: " + err.Error() })
		return
	}
	w.set(func(r *hostReadings) { r.released = true })
	floor := idle + pressureMargin
	for ctx.Err() == nil {
		before, err := pidsCurrent(dir)
		if err != nil {
			return // the cgroup goes with the container: the bomb is over
		}
		spawnErr := probe(ctx, "/bin/true")
		after, afterErr := pidsCurrent(dir)
		w.set(func(r *hostReadings) {
			r.reads++
			r.peak = max(r.peak, before)
			if afterErr == nil {
				r.reads++
				r.peak = max(r.peak, after)
			}
			if ctx.Err() != nil { // a probe cut short by stop says nothing about the host
				return
			}
			r.spawns++
			if spawnErr != nil && r.spawnErr == "" {
				r.spawnErr = spawnErr.Error()
			}
			if spawnErr == nil && afterErr == nil && before >= floor && after >= floor {
				r.overlapped++
			}
		})
	}
}

// probe runs one host command bounded by ctx and probeTimeout.
func probe(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = time.Second
	return cmd.Run()
}

func pidsCurrent(dir string) (int, error) {
	return strconv.Atoi(readField(filepath.Join(dir, "pids.current")))
}

// containerCgroup finds the running container's cgroup v2 directory: the one its init process lives in,
// else docker's systemd scope. Only a directory with a numeric pids.max (the container's own limit) counts.
func containerCgroup(ctx context.Context, name string) string {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Id}} {{.State.Pid}}", name)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
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
