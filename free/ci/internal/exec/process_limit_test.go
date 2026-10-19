package exec

import (
	"strings"
	"testing"
)

// The default memory demand (MemoryMB with no declared limit) must never become an rlimit:
// RLIMIT_AS at 256 MB kills go and node at start on Linux (D-1009).
func TestLimitedCommandDefaultDemandAddsNoMemoryLimit(t *testing.T) {
	got := limitedCommandOn(JobSpec{MemoryMB: 256, Command: []string{"go", "version"}}, "linux")
	if strings.Join(got, " ") != "go version" {
		t.Fatalf("default demand wrapped the command: %q", got)
	}
}

func TestLimitedCommandDeclaredLimitUsesDataNotAddressSpace(t *testing.T) {
	got := strings.Join(limitedCommandOn(JobSpec{MemoryMB: 4096, MemoryLimitMB: 4096, Command: []string{"go", "version"}}, "linux"), " ")
	if !strings.Contains(got, "ulimit -d 4194304") || strings.Contains(got, "ulimit -v") {
		t.Fatalf("want RLIMIT_DATA only, got %q", got)
	}
	if other := limitedCommandOn(JobSpec{MemoryLimitMB: 4096, Command: []string{"x"}}, "darwin"); len(other) != 1 {
		t.Fatalf("darwin has no memory rlimit: %q", other)
	}
}
