package sample

// Purpose: the macOS readers, each one a fixed argv run through the exec funnel:
// `vm_stat` (free + inactive + speculative pages, the same set the CI-32 probe counts),
// `sysctl -n vm.loadavg`, `top -l 1 -n 0` for the CPU idle percentage and `ps -axco comm=`.
// Constraints: absolute binary paths for the probes themselves (the funnel launcher resolves
// its own `sh` through PATH, which is exec's concern).

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const darwinTimeout = 5 * time.Second

var (
	pageSizeRe = regexp.MustCompile(`page size of (\d+) bytes`)
	idleRe     = regexp.MustCompile(`([0-9]+(?:\.[0-9]+)?)% idle`)
)

func (s Sampler) sampleDarwin(ctx context.Context, live *Live) {
	if out, err := s.run(ctx, "mem_free_mb", live, "/usr/bin/vm_stat"); err == nil {
		if mb, ok := vmStatFreeMB(out); ok {
			live.MemFreeMB = i64(mb)
		} else {
			live.note("mem_free_mb", fmt.Errorf("unreadable vm_stat output"))
		}
	}
	if out, err := s.run(ctx, "load_per_core", live, "/usr/sbin/sysctl", "-n", "vm.loadavg"); err == nil {
		if one, ok := firstFloat(out); ok {
			live.LoadPerCore = f64(one / s.cores())
		} else {
			live.note("load_per_core", fmt.Errorf("unreadable vm.loadavg"))
		}
	}
	if out, err := s.run(ctx, "cpu_idle_pct", live, "/usr/bin/top", "-l", "1", "-n", "0"); err == nil {
		if m := idleRe.FindStringSubmatch(out); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			live.CPUIdlePct = f64(min(v, 100))
		} else {
			live.note("cpu_idle_pct", fmt.Errorf("no CPU idle figure in top output"))
		}
	}
}

// run executes one fixed command and records a failure against the value it feeds.
func (s Sampler) run(ctx context.Context, name string, live *Live, argv ...string) (string, error) {
	res, err := s.runner().Run(ctx, argv, darwinTimeout)
	if err == nil && res.ExitCode != 0 {
		err = fmt.Errorf("%s exited %d", filepath.Base(argv[0]), res.ExitCode)
	}
	if err != nil {
		live.note(name, err)
		return "", err
	}
	return res.Stdout, nil
}

func vmStatFreeMB(out string) (int64, bool) {
	page := int64(4096)
	if m := pageSizeRe.FindStringSubmatch(out); m != nil {
		page, _ = strconv.ParseInt(m[1], 10, 64)
	}
	var pages int64
	seen := false
	for _, line := range strings.Split(out, "\n") {
		for _, key := range []string{"Pages free:", "Pages inactive:", "Pages speculative:"} {
			if strings.HasPrefix(line, key) {
				f := strings.Fields(line)
				n, err := strconv.ParseInt(strings.TrimSuffix(f[len(f)-1], "."), 10, 64)
				if err != nil {
					return 0, false
				}
				pages += n
				seen = true
			}
		}
	}
	return pages * page / (1 << 20), seen && page > 0
}

// darwinProcesses lists the base names of every process.
func (s Sampler) darwinProcesses(ctx context.Context) ([]string, error) {
	// -c prints the executable name only (16 characters at most), a third of the size of
	// the full paths; an incomplete list would let a yield_to target hide in the cut tail.
	res, err := s.runner().Run(ctx, []string{"/bin/ps", "-axco", "comm="}, darwinTimeout)
	if err != nil {
		return nil, err
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("ps exited %d", res.ExitCode)
	}
	if res.Truncated {
		return nil, fmt.Errorf("process list longer than %d bytes", MaxOutput)
	}
	var names []string
	for _, line := range strings.Split(res.Stdout, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			names = append(names, line) // -c prints the bare executable name, which can contain "/"
		}
	}
	return names, nil
}

// Processes lists the running process names; an unsupported platform is an error.
func (s Sampler) Processes(ctx context.Context) ([]string, error) {
	switch s.goos() {
	case "linux":
		return s.linuxProcesses()
	case "darwin":
		return s.darwinProcesses(ctx)
	}
	return nil, fmt.Errorf("process names are not readable on %s", s.goos())
}
