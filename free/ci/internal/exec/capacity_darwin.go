//go:build darwin

package exec

// Purpose: the macOS half of the capacity probe (P7-CI-32): total memory, free
// memory and power state.
// Inputs:  the hw.memsize sysctl, vm_stat and pmset.
// Outputs: the memory and power fields of a Capacity.
// Constraints: a run probes once per admission and once per dispatcher tick
// (P7-CI-46 measured four child processes, about 17 ms, every time), so the
// total memory is read once with a sysctl (no fork), free memory is cached for
// freeMemoryTTL and the power state for powerTTL, and the two pmset calls and
// vm_stat run side by side when they are due. Neither value moves at the
// rate a run asks for it; the store's reservation ledger, not this snapshot,
// is what keeps two admissions from taking the same memory.

import (
	osexec "os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const (
	freeMemoryTTL = 250 * time.Millisecond
	powerTTL      = 30 * time.Second
)

// darwinPower is the parsed pmset state; battery is -1 when pmset names none.
type darwinPower struct {
	battery      int
	plugged, low bool
}

var darwinProbe struct {
	mu      sync.Mutex
	memMB   int
	memRead bool
	freeMB  int
	freeAt  time.Time
	power   darwinPower
	powerAt time.Time
}

func probeDarwin(c *Capacity) {
	p := &darwinProbe
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	if !p.memRead {
		if n, err := unix.SysctlUint64("hw.memsize"); err == nil {
			p.memMB = int(n / (1024 * 1024))
		}
		p.memRead = true
	}
	var wg sync.WaitGroup
	if p.freeAt.IsZero() || now.Sub(p.freeAt) >= freeMemoryTTL {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.freeMB = 0
			if out, err := osexec.Command("vm_stat").Output(); err == nil {
				p.freeMB = parseVMStat(string(out))
			}
		}()
		p.freeAt = now
	}
	if p.powerAt.IsZero() || now.Sub(p.powerAt) >= powerTTL {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.power = readPower()
		}()
		p.powerAt = now
	}
	wg.Wait()
	c.MemoryMB, c.FreeMemoryMB = p.memMB, p.freeMB
	c.BatteryPercent, c.PluggedIn, c.LowPower = p.power.battery, p.power.plugged, p.power.low
}

// parseVMStat is free + inactive + speculative pages, in MB.
func parseVMStat(out string) int {
	pageSize := int64(4096)
	var pages int64
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "page size of") {
			f := strings.Fields(line)
			for i, v := range f {
				if v == "of" && i+1 < len(f) {
					pageSize, _ = strconv.ParseInt(f[i+1], 10, 64)
				}
			}
		}
		if strings.HasPrefix(line, "Pages free:") || strings.HasPrefix(line, "Pages inactive:") || strings.HasPrefix(line, "Pages speculative:") {
			f := strings.Fields(line)
			if len(f) > 0 {
				n, _ := strconv.ParseInt(strings.TrimSuffix(f[len(f)-1], "."), 10, 64)
				pages += n
			}
		}
	}
	return int(pages * pageSize / (1024 * 1024))
}

// readPower runs the two pmset queries side by side.
func readPower() darwinPower {
	pw := darwinPower{battery: -1}
	var batt, custom []byte
	var battErr, customErr error
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); batt, battErr = osexec.Command("pmset", "-g", "batt").Output() }()
	go func() { defer wg.Done(); custom, customErr = osexec.Command("pmset", "-g", "custom").Output() }()
	wg.Wait()
	if battErr == nil {
		s := string(batt)
		pw.plugged = strings.Contains(s, "AC Power")
		for _, field := range strings.Fields(s) {
			if strings.HasSuffix(field, "%;") || strings.HasSuffix(field, "%") {
				pw.battery, _ = strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(field, ";"), "%"))
				break
			}
		}
	}
	if customErr == nil {
		pw.low = strings.Contains(string(custom), "lowpowermode 1")
	}
	return pw
}
