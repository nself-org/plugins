//go:build linux

package exec

import (
	"os"
	"testing"
)

func TestProbeCgroupQuota(t *testing.T) {
	c := Probe()
	if c.CPUs < 1 || c.MemoryMB < 1 || c.FreeMemoryMB < 1 {
		t.Fatalf("invalid capacity: %+v", c)
	}
	if os.Getenv("CI32_EXPECT_QUOTA") == "1" {
		if c.CPUs != 2 {
			t.Fatalf("2-CPU cgroup reported %+v", c)
		}
		if c.MemoryMB < 900 || c.MemoryMB > 1100 {
			t.Fatalf("1-GiB cgroup reported %+v", c)
		}
	}
}
