package sample

// Purpose: the Linux readers. Memory comes from MemAvailable, load from /proc/loadavg
// and CPU idle from two /proc/stat reads a short pause apart (one read only gives the
// average since boot, which says nothing about now).
// Constraints: only /proc, through the injected readers.

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// idleWindow is the gap between the two /proc/stat reads.
const idleWindow = 250 * time.Millisecond

func (s Sampler) sampleLinux(ctx context.Context, live *Live) {
	if b, err := s.readFile("/proc/meminfo"); err != nil {
		live.note("mem_free_mb", err)
	} else if kb, ok := meminfoAvailableKB(b); ok {
		live.MemFreeMB = i64(kb / 1024)
	} else {
		live.note("mem_free_mb", fmt.Errorf("MemAvailable missing from /proc/meminfo"))
	}
	if b, err := s.readFile("/proc/loadavg"); err != nil {
		live.note("load_per_core", err)
	} else if one, ok := firstFloat(string(b)); ok {
		live.LoadPerCore = f64(one / s.cores())
	} else {
		live.note("load_per_core", fmt.Errorf("unreadable /proc/loadavg"))
	}
	idle, err := s.linuxIdle(ctx)
	live.CPUIdlePct = idle
	live.note("cpu_idle_pct", err)
}

func meminfoAvailableKB(b []byte) (int64, bool) {
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "MemAvailable:" {
			n, err := strconv.ParseInt(f[1], 10, 64)
			return n, err == nil && n >= 0
		}
	}
	return 0, false
}

// firstFloat parses the first number in s, skipping the braces `sysctl vm.loadavg` wraps it in.
func firstFloat(s string) (float64, bool) {
	for _, field := range strings.Fields(s) {
		if field = strings.Trim(field, "{},"); field == "" {
			continue
		}
		v, err := strconv.ParseFloat(field, 64)
		return v, err == nil && v >= 0
	}
	return 0, false
}

// cpuTimes returns idle (idle + iowait) and total jiffies from the first /proc/stat line.
func cpuTimes(b []byte) (idle, total uint64, ok bool) {
	line, _, _ := bytes.Cut(b, []byte("\n"))
	f := strings.Fields(string(line))
	if len(f) < 5 || f[0] != "cpu" {
		return 0, 0, false
	}
	// user nice system idle iowait irq softirq steal; guest time is already inside user.
	for i, v := range f[1:] {
		if i >= 8 {
			break
		}
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return 0, 0, false
		}
		total += n
		if i == 3 || i == 4 {
			idle += n
		}
	}
	return idle, total, true
}

func (s Sampler) linuxIdle(ctx context.Context) (*float64, error) {
	read := func() (uint64, uint64, error) {
		b, err := s.readFile("/proc/stat")
		if err != nil {
			return 0, 0, err
		}
		idle, total, ok := cpuTimes(b)
		if !ok {
			return 0, 0, fmt.Errorf("unreadable /proc/stat")
		}
		return idle, total, nil
	}
	idle0, total0, err := read()
	if err != nil {
		return nil, err
	}
	pause := s.Pause
	if pause == nil {
		pause = sleep
	}
	if err = pause(ctx, idleWindow); err != nil {
		return nil, err
	}
	idle1, total1, err := read()
	if err != nil {
		return nil, err
	}
	if total1 <= total0 || idle1 < idle0 {
		return nil, fmt.Errorf("/proc/stat did not advance between samples")
	}
	pct := 100 * float64(idle1-idle0) / float64(total1-total0)
	return f64(min(pct, 100)), nil
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// linuxProcesses lists the short names (comm) of every process.
func (s Sampler) linuxProcesses() ([]string, error) {
	entries, err := s.readDir("/proc")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		b, err := s.readFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue // the process exited between the listing and the read
		}
		if name := strings.TrimSpace(string(b)); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
