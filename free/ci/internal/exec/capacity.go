package exec

import (
	"bufio"
	"bytes"
	"os"
	osexec "os/exec"
	"runtime"
	"strconv"
	"strings"
)

// Capacity is a host snapshot used for local admission and evidence.
type Capacity struct {
	CPUs                                 int
	MemoryMB, FreeMemoryMB               int
	BatteryPercent                       int // -1 means unknown.
	PluggedIn, LowPower, Interactive, CI bool
}

// Probe reads local quotas, memory and power from the current host.
func Probe() Capacity {
	c := Capacity{CPUs: runtime.NumCPU(), BatteryPercent: -1, Interactive: isTerminal(), CI: strings.EqualFold(os.Getenv("CI"), "true")}
	if c.CPUs < 1 {
		c.CPUs = 1
	}
	if runtime.GOOS == "linux" {
		probeLinux(&c)
	}
	if runtime.GOOS == "darwin" {
		probeDarwin(&c)
	}
	if c.MemoryMB < 1 {
		c.MemoryMB = 1024
	}
	if c.FreeMemoryMB < 1 {
		c.FreeMemoryMB = c.MemoryMB
	}
	return c
}

func probeLinux(c *Capacity) {
	if b, err := os.ReadFile("/sys/fs/cgroup/cpu.max"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) == 2 && fields[0] != "max" {
			quota, e1 := strconv.Atoi(fields[0])
			period, e2 := strconv.Atoi(fields[1])
			if e1 == nil && e2 == nil && period > 0 {
				n := (quota + period - 1) / period
				if n < 1 {
					n = 1
				}
				if n < c.CPUs {
					c.CPUs = n
				}
			}
		}
	}
	if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && n > 0 {
			c.MemoryMB = int(n / (1024 * 1024))
		}
	}
	if c.MemoryMB == 0 {
		if b, err := os.ReadFile("/proc/meminfo"); err == nil {
			s := bufio.NewScanner(bytes.NewReader(b))
			for s.Scan() {
				f := strings.Fields(s.Text())
				if len(f) >= 2 && f[0] == "MemTotal:" {
					n, _ := strconv.Atoi(f[1])
					c.MemoryMB = n / 1024
					break
				}
			}
		}
	}
	if b, err := os.ReadFile("/proc/meminfo"); err == nil {
		s := bufio.NewScanner(bytes.NewReader(b))
		for s.Scan() {
			f := strings.Fields(s.Text())
			if len(f) >= 2 && f[0] == "MemAvailable:" {
				n, _ := strconv.Atoi(f[1])
				c.FreeMemoryMB = n / 1024
				break
			}
		}
	}
	if limit, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		if max, e1 := strconv.ParseInt(strings.TrimSpace(string(limit)), 10, 64); e1 == nil && max > 0 {
			if used, err := os.ReadFile("/sys/fs/cgroup/memory.current"); err == nil {
				n, e2 := strconv.ParseInt(strings.TrimSpace(string(used)), 10, 64)
				if e2 == nil {
					free := int((max - n) / (1024 * 1024))
					if c.FreeMemoryMB == 0 || free < c.FreeMemoryMB {
						c.FreeMemoryMB = free
					}
				}
			}
		}
	}
	entries, _ := os.ReadDir("/sys/class/power_supply")
	for _, entry := range entries {
		base := "/sys/class/power_supply/" + entry.Name() + "/"
		kind, _ := os.ReadFile(base + "type")
		switch strings.TrimSpace(string(kind)) {
		case "Battery":
			if b, err := os.ReadFile(base + "capacity"); err == nil {
				c.BatteryPercent, _ = strconv.Atoi(strings.TrimSpace(string(b)))
			}
			if b, err := os.ReadFile(base + "status"); err == nil {
				status := strings.TrimSpace(string(b))
				if status == "Charging" || status == "Full" {
					c.PluggedIn = true
				}
			}
		case "Mains", "AC":
			if b, err := os.ReadFile(base + "online"); err == nil && strings.TrimSpace(string(b)) == "1" {
				c.PluggedIn = true
			}
		}
	}
}

func probeDarwin(c *Capacity) {
	if out, err := osexec.Command("sysctl", "-n", "hw.memsize").Output(); err == nil {
		n, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		c.MemoryMB = int(n / (1024 * 1024))
	}
	if out, err := osexec.Command("vm_stat").Output(); err == nil {
		pageSize := int64(4096)
		var pages int64
		for _, line := range strings.Split(string(out), "\n") {
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
		c.FreeMemoryMB = int(pages * pageSize / (1024 * 1024))
	}
	if out, err := osexec.Command("pmset", "-g", "batt").Output(); err == nil {
		s := string(out)
		c.PluggedIn = strings.Contains(s, "AC Power")
		for _, field := range strings.Fields(s) {
			if strings.HasSuffix(field, "%;") || strings.HasSuffix(field, "%") {
				n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimSuffix(field, ";"), "%"))
				c.BatteryPercent = n
				break
			}
		}
	}
	if out, err := osexec.Command("pmset", "-g", "custom").Output(); err == nil {
		c.LowPower = strings.Contains(string(out), "lowpowermode 1")
	}
}
